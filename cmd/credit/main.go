package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/Shamba-Records-Limited/microvault-credit/cmd/credit/docs"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/adapters"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/handlers"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
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
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/moneygram"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/offramp"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/stellaranchor"
	"github.com/Shamba-Records-Limited/microvault/pkg/payment/yellowcard"
	"github.com/Shamba-Records-Limited/microvault/pkg/pin"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	"github.com/Shamba-Records-Limited/microvault/pkg/services/mgpoller"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	stellarrpc "github.com/Shamba-Records-Limited/microvault/pkg/stellar/rpc"
	"github.com/Shamba-Records-Limited/microvault/pkg/transaction"
	"github.com/Shamba-Records-Limited/microvault/pkg/urlshortener"
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
		cfg.Mobile.AfricasTalking.HTTPTimeout,
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
	treasuryTransfer := ussdadapters.NewStellarTreasuryTransfer(stellarSvc, logger)

	// ---- 10. Off-ramp adapters ----
	// Both YellowCard (mobile money) and MoneyGram (cash pickup) are
	// platform defaults — no env-gate. Each provider is built and
	// registered once; the registry drives per-request dispatch inside
	// LoanServiceAdapter, the YC adapter is passed directly to the YC-
	// specific refund poller and rate service, and the FXOrchestrator
	// cascades MG to YC for rate quoting inside the loan adapter.

	// 10a. YellowCard off-ramp adapter
	//
	// YELLOWCARD_TEST_DESTINATION_PHONE_OVERRIDE: test-only knob. When set,
	// every YC payment uses this number instead of the real recipient — used
	// for hitting YC sandbox simulation phones (e.g. +2341111111111 for a
	// guaranteed-success momo transaction). Hard-disabled in production:
	// even if the env var is set, the override is dropped when
	// SERVER_ENVIRONMENT=production.
	ycTestPhoneOverride := os.Getenv("YELLOWCARD_TEST_DESTINATION_PHONE_OVERRIDE")
	if ycTestPhoneOverride != "" && os.Getenv("SERVER_ENVIRONMENT") == "production" {
		logger.Error("YELLOWCARD_TEST_DESTINATION_PHONE_OVERRIDE is set but SERVER_ENVIRONMENT=production — override ignored")
		ycTestPhoneOverride = ""
	}
	ycTestAddressOverride := os.Getenv("YELLOWCARD_TEST_DESTINATION_ADDRESS_OVERRIDE")
	if ycTestAddressOverride != "" && os.Getenv("SERVER_ENVIRONMENT") == "production" {
		logger.Error("YELLOWCARD_TEST_DESTINATION_ADDRESS_OVERRIDE is set but SERVER_ENVIRONMENT=production — override ignored")
		ycTestAddressOverride = ""
	}
	ycOffRamp := ussdadapters.NewYellowCardOffRampAdapter(ussdadapters.YellowCardOffRampConfig{
		Adapter:                        ycAdapter,
		Treasury:                       treasuryTransfer,
		BusinessID:                     cfg.Payments.YellowCard.BusinessID,
		BusinessName:                   cfg.Payments.YellowCard.BusinessName,
		Logger:                         logger,
		TestDestinationPhoneOverride:   ycTestPhoneOverride,
		TestDestinationAddressOverride: ycTestAddressOverride,
	})
	offRampRegistry := offramp.NewRegistry()
	if err := offRampRegistry.Register(ycOffRamp); err != nil {
		log.Fatalf("Failed to register YellowCard off-ramp: %v", err)
	}
	if err := offRampRegistry.Alias(offramp.PayoutMethodMobileMoney, offramp.ProviderYellowCard); err != nil {
		log.Fatalf("Failed to alias mobile_money → yellowcard: %v", err)
	}

	// ---- 10a. MoneyGram cash-pickup adapter ----
	// MG is the platform's default cash-pickup anchor: we always fetch the
	// anchor's TOML, validate it against the pinned signing key + network
	// passphrase, construct the SDK client, register the adapter, and alias
	// cash_pickup → moneygram. Boot fails loudly if any of that breaks.
	if err := cfg.Payments.MoneyGram.Validate(); err != nil {
		log.Fatalf("MoneyGram config invalid: %v", err)
	}
	tomlCtx, tomlCancel := context.WithTimeout(context.Background(), 15*time.Second)
	mgTOML, err := stellaranchor.FetchTOML(tomlCtx, nil, cfg.Payments.MoneyGram.HomeDomain)
	tomlCancel()
	if err != nil {
		log.Fatalf("MoneyGram TOML fetch failed for %s: %v", cfg.Payments.MoneyGram.HomeDomain, err)
	}
	if err := mgTOML.Validate(stellaranchor.ValidateOptions{
		ExpectedNetworkPassphrase: cfg.Payments.MoneyGram.NetworkPassphrase,
		ExpectedSigningKey:        cfg.Payments.MoneyGram.ServerSigningKey,
		ExpectedUSDCIssuer:        cfg.Payments.MoneyGram.USDCIssuer,
	}); err != nil {
		log.Fatalf("MoneyGram TOML validation failed: %v", err)
	}

	// TransferServerURL override from env wins over the TOML; otherwise
	// fall back to whatever the anchor publishes.
	transferServerURL := cfg.Payments.MoneyGram.TransferServerURL
	if transferServerURL == "" {
		transferServerURL = mgTOML.TransferServerSEP24
	}

	mgCfg := moneygram.Config{
		HomeDomain:        cfg.Payments.MoneyGram.HomeDomain,
		WebAuthEndpoint:   mgTOML.WebAuthEndpoint,
		TransferServerURL: transferServerURL,
		ServerSigningKey:  cfg.Payments.MoneyGram.ServerSigningKey,
		NetworkPassphrase: cfg.Payments.MoneyGram.NetworkPassphrase,
		USDCIssuer:        cfg.Payments.MoneyGram.USDCIssuer,
		TreasurySecret:    cfg.Payments.MoneyGram.AuthSecret,
		Logger:            logger,
	}
	if cfg.Payments.MoneyGram.HasRESTCredentials() {
		mgCfg.REST = moneygram.RESTConfig{
			BaseURL:       cfg.Payments.MoneyGram.FXRateURL,
			OAuthTokenURL: cfg.Payments.MoneyGram.OAuthURL,
			ClientID:      cfg.Payments.MoneyGram.ClientID,
			ClientSecret:  cfg.Payments.MoneyGram.ClientSecret,
			Scope:         "fx_rate",
		}
	}
	mgClient, err := moneygram.New(mgCfg)
	if err != nil {
		log.Fatalf("MoneyGram client construction failed: %v", err)
	}

	// Funds wallet: SEP-24 account + USDC send source. Distinct from the auth
	// wallet in prod; both default to TREASURY_SECRET_KEY.
	mgFundsAddr, err := cfg.Payments.MoneyGram.FundsAddress()
	if err != nil {
		log.Fatalf("MoneyGram funds address derivation failed: %v", err)
	}
	mgFundsSvc := stellar.NewService(
		rpcClient,
		cfg.Stellar.NetworkPassphrase,
		cfg.Payments.MoneyGram.FundsSecret,
		cfg.Stellar.AdminSecretKey,
		cfg.Stellar.ContractID,
		cfg.Stellar.USDCIssuer,
	)
	mgFundsTransfer := ussdadapters.NewStellarTreasuryTransfer(mgFundsSvc, logger)
	mgAuthAddr, _ := cfg.Payments.MoneyGram.AuthAddress()
	logger.Info("moneygram wallets resolved", "auth_address", mgAuthAddr, "funds_address", mgFundsAddr)

	mgAdapter, err := ussdadapters.NewMoneyGramOffRampAdapter(ussdadapters.MoneyGramOffRampConfig{
		Client:      mgClient,
		FundsPubkey: mgFundsAddr,
		Logger:      logger,
	})
	if err != nil {
		log.Fatalf("MoneyGram off-ramp adapter construction failed: %v", err)
	}
	if err := offRampRegistry.Register(mgAdapter); err != nil {
		log.Fatalf("Failed to register MoneyGram off-ramp: %v", err)
	}
	if err := offRampRegistry.Alias(offramp.PayoutMethodCashPickup, offramp.ProviderMoneyGram); err != nil {
		log.Fatalf("Failed to alias cash_pickup → moneygram: %v", err)
	}
	log.Printf("MoneyGram cash-pickup registered (home: %s, REST: %t)",
		cfg.Payments.MoneyGram.HomeDomain, cfg.Payments.MoneyGram.HasRESTCredentials())

	// 10d. FX orchestrator: MG primary, YC fallback, stale cache last resort.
	ycFallback := moneygram.FallbackRateFunc(func(ctx context.Context, currency string) (float64, error) {
		rates, err := ycAdapter.GetRates(ctx, currency)
		if err != nil {
			return 0, err
		}
		// Match on Code rather than trusting position. The ?currency= filter is
		// honoured today, so rates[0] is right — but quoting a loan at another
		// market's rate is not a failure worth risking on that assumption.
		for _, r := range rates {
			if strings.EqualFold(r.Code, currency) {
				if r.Sell <= 0 {
					return 0, fmt.Errorf("yellowcard: no sell rate for %s", currency)
				}
				return r.Sell, nil
			}
		}
		return 0, fmt.Errorf("yellowcard: no rates for %s", currency)
	})
	fxOrch, err := mgClient.NewFXOrchestrator(ycFallback, moneygram.FXOrchestratorConfig{})
	if err != nil {
		log.Fatalf("MoneyGram FX orchestrator init failed: %v", err)
	}
	logger.Info("moneygram FX orchestrator wired",
		"primary_active", mgClient.HasFXRate(),
		"fallback_active", true,
	)

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
		logger,
		nil, // AlertService — chain-status failures log-only for now
	)
	if err != nil {
		log.Fatalf("Failed to create user service adapter: %v", err)
	}

	// ---- 10d. Loan product service ----
	loanProductSvc := loanproduct.NewService(repos.LoanProduct)

	// ---- 11. LoanServiceAdapter (USSD LoanService) ----
	ctx := context.Background()
	loanAdapter, err := adapters.NewLoanServiceAdapter(
		ctx, loanSvc, loanProductSvc, stellarSvc, offRampRegistry, loanNotifier, txnSvc,
		adapters.FXConfig{BufferPct: adapters.DefaultFXBufferPct}, logger,
	)
	if err != nil {
		log.Fatalf("Failed to create loan service adapter: %v", err)
	}
	if fxOrch != nil {
		loanAdapter.SetFXOrchestrator(fxOrch)
	}
	loanAdapter.SetPublicBaseURL(cfg.Server.PublicBaseURL)
	loanAdapter.SetAccountEnsurer(userAdapter)
	if cfg.Shortener.Enabled() {
		loanAdapter.SetShortener(urlshortener.NewDub(cfg.Shortener.APIKey, cfg.Shortener.ImagePreviewURL))
		log.Printf("WARNING: dub.co link shortener enabled — cash-pickup SMS links are sent to dub.co")
	}

	// ---- 12. DisbursementStatusAdapter (webhook callbacks) ----
	disbursementAdapter := adapters.NewDisbursementStatusAdapter(
		repos.Loan,
		loanNotifier,
		txnSvc,
		stellarSvc,
		logger,
	)
	disbursementAdapter.SetPublicBaseURL(cfg.Server.PublicBaseURL)

	// ---- 12b. Rate service adapter ----
	// YC is the canonical Quoter for the USSD pre-loan rate display. MG's
	// FX cascade is consumed inside the loan adapter, not here.
	rateSvc := adapters.NewRateServiceAdapter(ycOffRamp)

	// ---- 12c. Account notifier + PIN service ----
	accountNotifier := mvnotifications.NewSMSAccountNotifier(notifier, nil)
	pinRepo := pin.NewSecurityQuestionRepository(db)
	pinService := pin.NewService(coreRepos.User, pinRepo, accountNotifier, cfg.Auth.PINLockoutDuration)
	log.Println("PIN service initialized")

	// Resolve SMS language from the recipient's stored preference so every
	// notification (including background poller/job sends) is localized.
	langResolver := func(ctx context.Context, phone string) string {
		u, err := userSvc.GetByMobileNumber(ctx, phone)
		if err != nil || u == nil {
			return ""
		}
		return u.PreferredLanguage
	}
	loanNotifier.SetLanguageResolver(langResolver)
	accountNotifier.SetLanguageResolver(langResolver)

	// ---- 13. USSD stack ----
	sessionManager := ussd.NewSessionManager(redisClient, cfg.Mobile.SessionTimeout)
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
	webhookSvc := webhook.NewService(disbursementAdapter, nil, disbursementAdapter, ycAdapter)
	webhookCtrl := controllers.NewWebhookController(webhookSvc, cfg.Payments.YellowCard.WebhookSecret)

	// ---- 15. Pollers ----
	pollerCtx, pollerCancel := context.WithCancel(context.Background())
	refundPoller := webhook.NewRefundPoller(
		ycAdapter,
		ycOffRamp,
		disbursementAdapter,
		disbursementAdapter,
		nil, // AlertService — ops alerts via logging for now
		disbursementAdapter,
		webhook.DefaultRefundPollerConfig(),
	)
	go refundPoller.Start(pollerCtx)

	// MoneyGram SEP-24 lifecycle poller.
	// Drives pending_user_transfer_start to SendUSDC, pending_user_transfer_complete
	// to backfill amount_out + cash-pickup reference, terminal to finalise.
	mgPollerAdapter, err := adapters.NewMoneyGramPollerAdapter(repos.Loan, loanSvc, txnSvc, logger)
	if err != nil {
		log.Fatalf("MoneyGram poller adapter construction failed: %v", err)
	}
	mgP, err := mgpoller.NewPoller(
		mgClient,
		mgPollerAdapter,                   // LoanFetcher + LoanRecorder
		mgPollerAdapter,                   // LoanRecorder (same impl)
		disbursementAdapter,               // DisbursementUpdater (reused from YC flow)
		mgFundsTransfer,                   // funds-wallet sender for USDC to MG anchor
		stellarrpc.NewVerifier(rpcClient), // confirms MG's refunds landed on-ledger
		nil,                               // AlertService — log-only for now
		mgPollerConfig(cfg),
		logger,
	)
	if err != nil {
		log.Fatalf("MoneyGram poller construction failed: %v", err)
	}
	go mgP.Start(pollerCtx)
	log.Println("MoneyGram poller started")

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

	// Cash-pickup SMS short-link → MoneyGram interactive URL redirect.
	redirectHandler := handlers.NewInteractiveRedirectHandler(loanSvc, logger)
	app.Get("/r/:code", redirectHandler.Handle)

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

// mgPollerConfig overlays the MoneyGram poller settings from the environment
// onto the package defaults. Unset values stay zero and NewPoller keeps its
// own default, so config never duplicates the defaults.
func mgPollerConfig(cfg *config.Config) mgpoller.PollerConfig {
	c := mgpoller.DefaultConfig()
	mg := cfg.Payments.MoneyGram
	if mg.PollInterval > 0 {
		c.PollInterval = mg.PollInterval
	}
	if mg.PollMaxBatch > 0 {
		c.MaxBatch = mg.PollMaxBatch
	}
	if mg.RefundSettleMaxAttempts > 0 {
		c.RefundSettleMaxAttempts = mg.RefundSettleMaxAttempts
	}
	// MoneyGram refunds return to the SEP-24 funds wallet, which is a different
	// account from the SEP-10 auth wallet outside development. Left unset, the
	// poller would watch the auth account and no refund would ever settle.
	if addr, err := mg.FundsAddress(); err == nil {
		c.RefundDestination = addr
	} else {
		log.Printf("MoneyGram funds address unresolved, refund destination falls back to the SEP-10 account: %v", err)
	}
	c.RefundAssetIssuer = cfg.Stellar.USDCIssuer
	return c
}
