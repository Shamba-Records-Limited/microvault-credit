package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/Shamba-Records-Limited/microvault-credit/cmd/credit/docs"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/adapters"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	"github.com/Shamba-Records-Limited/microvault/pkg/account"
	"github.com/Shamba-Records-Limited/microvault/pkg/auth"
	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/controllers"
	"github.com/Shamba-Records-Limited/microvault/pkg/health"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/sms"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/sms/providers/africastalking"
	"github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd"
	ussdadapters "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/adapters"
	atussd "github.com/Shamba-Records-Limited/microvault/pkg/mobile/ussd/providers/africastalking"
	mvnotifications "github.com/Shamba-Records-Limited/microvault/pkg/notifications"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/yellowcard"
	"github.com/Shamba-Records-Limited/microvault/pkg/pin"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"
	"github.com/Shamba-Records-Limited/microvault/pkg/user"
	"github.com/Shamba-Records-Limited/microvault/pkg/validation"
	"github.com/Shamba-Records-Limited/microvault/pkg/webhook"
	"github.com/Shamba-Records-Limited/microvault/platform/cache"
	"github.com/Shamba-Records-Limited/microvault/platform/database"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

// @title microvault Credit API
// @version 1.0
// @description Credit management and loan processing service for microvault.
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email smugane@shambarecords.com
// @license.name AGPL-3.0
// @license.url https://www.gnu.org/licenses/agpl-3.0.en.html
// @host localhost:8081
// @BasePath /
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// ---- 1. Configuration ----
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// ---- 2. Database ----
	db, err := database.GetConnection("credit", &cfg.Postgres)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Println("Database connected successfully")

	// ---- 3. Redis ----
	redisClient, err := cache.GetConnection("credit", &cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Redis connected successfully")

	// ---- 3b. Auth services ----
	challengeStore, err := auth.NewRedisStore(redisClient, "microvault:auth")
	if err != nil {
		log.Fatalf("Failed to initialize challenge store: %v", err)
	}
	challengeService, err := auth.NewChallengeService(&cfg.Auth, &cfg.Stellar, challengeStore)
	if err != nil {
		log.Fatalf("Failed to initialize challenge service: %v", err)
	}
	jwtService := auth.NewJWTService(&cfg.Auth)
	validationService := validation.NewValidatorService()
	authController := controllers.NewAuthController(challengeService, jwtService, cfg.Stellar.AdminPublicKey, validationService)

	// ---- 4. Repositories ----
	repos, err := repository.NewRepositories(db)
	if err != nil {
		log.Fatalf("Failed to initialize repositories: %v", err)
	}

	// ---- 4b. Core repositories (shared DB — users, accounts, transactions) ----
	coreRepos, err := corerepository.NewRepositories(db)
	if err != nil {
		log.Fatalf("Failed to initialize core repositories: %v", err)
	}

	// ---- 5. Loan service ----
	loanSvc := loan.NewService(repos.Loan)

	// ---- 6. Stellar RPC client + service ----
	rpcClient := cfg.Stellar.NewRpcClient()
	stellarSvc := stellar.NewService(
		rpcClient,
		cfg.Stellar.NetworkPassphrase,
		cfg.Stellar.TreasurySecretKey,
		cfg.Stellar.AdminSecretKey,
		cfg.Stellar.ContractID,
		cfg.Stellar.USDCIssuer,
	)

	// ---- 7. SMS + notification service ----
	smsService := sms.NewSMSService()
	atSMSAdapter := africastalking.NewAfricasTalkingSMSAdapter(
		cfg.Mobile.AfricasTalking.Username,
		cfg.Mobile.AfricasTalking.APIKey,
		cfg.Mobile.AfricasTalking.BaseURL,
	)
	smsService.RegisterProvider("africastalking", atSMSAdapter)

	// Resolve SMS provider and create notifier + loan notifier.
	atProvider, _ := smsService.GetProvider("africastalking")
	notifier := mvnotifications.NewSMSNotifier(atProvider, cfg.Mobile.AfricasTalking.ResolveSenderID())
	loanNotifier := mvnotifications.NewSMSLoanNotifier(notifier, nil)

	// ---- 8. YellowCard adapter ----
	ycAdapter := yellowcard.NewYellowcardAdapter(
		cfg.Payments.YellowCard.PublicKey,
		cfg.Payments.YellowCard.SecretKey,
		cfg.Payments.YellowCard.BaseURL,
	)

	// ---- 9. Treasury transfer bridge ----
	treasuryTransfer := ussdadapters.NewStellarTreasuryTransfer(stellarSvc)

	// ---- 10. Off-ramp adapter ----
	offRampSvc := ussdadapters.NewYellowCardOffRampAdapter(ussdadapters.YellowCardOffRampConfig{
		Adapter:      ycAdapter,
		Treasury:     treasuryTransfer,
		BusinessID:   cfg.Payments.YellowCard.BusinessID,
		BusinessName: cfg.Payments.YellowCard.BusinessName,
	})

	// ---- 10b. User + Account + Transaction services ----
	userSvc := user.NewService(coreRepos.User)
	accountSvc := account.NewService(coreRepos.Account, coreRepos.User)
	txnSvc := transaction.NewService(coreRepos.Transaction)

	// ---- 10c. UserServiceAdapter ----
	userAdapter, err := ussdadapters.NewUserServiceAdapter(
		userSvc,
		accountSvc,
		stellarSvc,
		db,
		ussdadapters.WalletConfig{
			TreasuryPrivateKey: cfg.Stellar.TreasurySecretKey,
			USDCIssuer:         cfg.Stellar.USDCIssuer,
			EnableMultiSig:     cfg.Stellar.EnableMultiSig,
			LowThreshold:       cfg.Stellar.MultiSigLowThreshold,
			MediumThreshold:    cfg.Stellar.MultiSigMediumThreshold,
			HighThreshold:      cfg.Stellar.MultiSigHighThreshold,
		},
	)
	if err != nil {
		log.Fatalf("Failed to create user service adapter: %v", err)
	}

	// ---- 11. LoanServiceAdapter (USSD LoanService) ----
	// 5000 USDC = 50_000_000_000 stroops auto-approve limit
	loanAdapter := adapters.NewLoanServiceAdapter(
		loanSvc,
		stellarSvc,
		offRampSvc,
		loanNotifier,
		txnSvc,
		logger,
		50_000_000_000,
	)

	// ---- 12. DisbursementStatusAdapter (webhook callbacks) ----
	disbursementAdapter := adapters.NewDisbursementStatusAdapter(
		repos.Loan,
		loanNotifier,
		txnSvc,
		logger,
	)

	// ---- 12b. Rate service adapter ----
	rateSvc := adapters.NewRateServiceAdapter(offRampSvc)

	// ---- 12c. Account notifier + PIN service ----
	accountNotifier := mvnotifications.NewSMSAccountNotifier(notifier, nil)
	pinRepo := pin.NewSecurityQuestionRepository(db)
	pinService := pin.NewService(coreRepos.User, pinRepo, accountNotifier, cfg.Auth.PINLockoutDuration)
	log.Println("PIN service initialized")

	// ---- 13. USSD stack ----
	sessionManager := ussd.NewSessionManager(redisClient, 0) // default 5min TTL
	menuRegistry := ussd.NewMenuRegistry()

	// Register standard loan menus
	standardPreset := &ussd.StandardLoanMenuPreset{}
	standardPreset.Initialize(menuRegistry)

	ussdHandler := ussd.NewUSSDHandler(sessionManager, menuRegistry, userAdapter, loanAdapter, rateSvc, pinService, accountNotifier)
	ussdService := ussd.NewUSSDService(ussdHandler)

	// Register Africa's Talking USSD provider
	atUSSDProvider := atussd.NewAfricasTalkingUSSDAdapter(
		cfg.Mobile.AfricasTalking.Username,
		cfg.Mobile.AfricasTalking.APIKey,
	)
	ussdService.RegisterProvider("africastalking", atUSSDProvider)

	// ---- 13b. USSD controller ----
	ussdCtrl := controllers.NewUSSDController(ussdService)

	// ---- 14. Webhook service + controller ----
	webhookSvc := webhook.NewService(disbursementAdapter, nil, disbursementAdapter)
	webhookCtrl := controllers.NewWebhookController(webhookSvc, cfg.Payments.YellowCard.WebhookSecret)

	// ---- 15. Refund poller ----
	pollerCtx, pollerCancel := context.WithCancel(context.Background())
	refundPoller := webhook.NewRefundPoller(
		ycAdapter,
		offRampSvc,
		disbursementAdapter,
		disbursementAdapter,
		nil, // AlertService — ops alerts via logging for now
		disbursementAdapter,
		webhook.DefaultRefundPollerConfig(),
	)
	go refundPoller.Start(pollerCtx)

	// ---- 16. Fiber app + middleware + routes ----
	app := fiber.New()

	healthCheck := health.NewCheckerWithoutStellar("credit", "credit")
	middleware.FiberMiddleware(app, healthCheck)

	// Swagger
	app.Get("/swagger/*", swagger.New(swagger.Config{
		DeepLinking:  false,
		DocExpansion: "list",
		OAuth: &swagger.OAuthConfig{
			AppName:  "OAuth Provider",
			ClientId: "21bb4edc-05a7-4afc-86f1-2e151e4ba6e2",
		},
		OAuth2RedirectUrl: "http://localhost:8081/swagger/oauth2-redirect.html",
	}))

	// Redoc at root
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("./cmd/credit/docs/redoc-static.html")
	})

	// Core API routes (from microvault)
	api := app.Group("/api/v1")
	api.Get("/auth/challenge", middleware.FormatResponse(), authController.GetChallenge)
	api.Post("/auth/verify", middleware.FormatResponse(), authController.VerifyChallenge)

	// USSD callback — supports multiple providers via URL param
	api.Post("/mobile/ussd/:provider", ussdCtrl.HandleCallback)

	// SMS delivery report callback — supports multiple providers via URL param
	smsCallbackHandler := sms.NewDeliveryReportHandler()
	smsCallbackCtrl := controllers.NewSMSCallbackController(smsCallbackHandler)
	api.Post("/mobile/sms/:provider/delivery", smsCallbackCtrl.HandleDeliveryReport)

	// Webhook routes
	api.Post("/webhooks/yellowcard", webhookCtrl.HandleYellowCardWebhook)

	// ---- 17. Start server ----
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Starting credit server on %s", cfg.Server.CreditAddr())
		if err := app.Listen(cfg.Server.CreditAddr()); err != nil {
			log.Printf("Server Listen error: %v", err)
		}
	}()

	// ---- 18. Graceful shutdown ----
	sig := <-sigChan
	log.Printf("Received signal %s. Shutting down gracefully...", sig)

	pollerCancel()

	if err := app.Shutdown(); err != nil {
		log.Printf("Fiber shutdown error: %v", err)
	}
	log.Println("Fiber server shut down.")

	if err := database.CloseAll(); err != nil {
		log.Printf("Database shutdown error: %v", err)
	} else {
		log.Println("Database connections closed successfully.")
	}

	if err := cache.CloseAll(); err != nil {
		log.Printf("Cache shutdown error: %v", err)
	} else {
		log.Println("Cache connections closed successfully.")
	}

	log.Println("Application shutdown complete.")
}
