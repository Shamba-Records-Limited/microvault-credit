// Command admin serves the Microvault portal dashboard.
//
// It binds to ADMIN_HOST (default 127.0.0.1) rather than all interfaces: the
// dashboard is an internal tool and is expected to sit behind a VPN or an
// authenticating proxy, not on the public internet.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Shamba-Records-Limited/microvault/pkg/telemetry"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/handlers"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/metrics"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/static"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	globallendinglimit "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/global_lending_limit"
	loanlimitconfig "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_limit_config"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
	"github.com/Shamba-Records-Limited/microvault/pkg/auth"
	"github.com/Shamba-Records-Limited/microvault/pkg/compliance/elliptic"
	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/logging"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	compliancesvc "github.com/Shamba-Records-Limited/microvault/pkg/services/compliance"
	"github.com/Shamba-Records-Limited/microvault/pkg/stellar"
	"github.com/Shamba-Records-Limited/microvault/platform/cache"
	"github.com/Shamba-Records-Limited/microvault/platform/database"
)

func main() {
	logger := logging.Setup()

	shutdownTracing, err := telemetry.Setup(context.Background())
	if err != nil {
		log.Fatalf("Failed to set up tracing: %v", err)
	}

	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	db, err := database.GetConnection("admin", &cfg.Postgres)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	redisClient, err := cache.GetConnection("admin", &cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	challengeStore, err := auth.NewRedisStore(redisClient, "microvault:admin-auth")
	if err != nil {
		log.Fatalf("Failed to initialize challenge store: %v", err)
	}
	challengeService, err := auth.NewChallengeService(&cfg.Auth, &cfg.Stellar, challengeStore)
	if err != nil {
		log.Fatalf("Failed to initialize challenge service: %v", err)
	}
	jwtService := auth.NewJWTService(&cfg.Auth)

	loanProductRepo, err := repository.NewLoanProductRepository(db)
	if err != nil {
		log.Fatalf("Failed to initialize loan product repository: %v", err)
	}
	limitConfigRepo, err := repository.NewLoanLimitConfigRepository(db)
	if err != nil {
		log.Fatalf("Failed to initialize loan limit config repository: %v", err)
	}
	globalLimitRepo, err := repository.NewGlobalLendingLimitRepository(db)
	if err != nil {
		log.Fatalf("Failed to initialize global lending limit repository: %v", err)
	}
	loanRepo, err := repository.NewLoanRepository(db)
	if err != nil {
		log.Fatalf("Failed to initialize loan repository: %v", err)
	}
	userRepo, err := corerepository.NewUserRepository(db)
	if err != nil {
		log.Fatalf("Failed to initialize user repository: %v", err)
	}
	txRepo, err := corerepository.NewTransactionRepository(db)
	if err != nil {
		log.Fatalf("Failed to initialize transaction repository: %v", err)
	}
	counterpartyRepo, err := corerepository.NewCounterpartyRepository(db)
	if err != nil {
		log.Fatalf("Failed to initialize counterparty repository: %v", err)
	}

	ellipticClient := elliptic.NewClient(elliptic.Config{
		APIKey:     cfg.Compliance.EllipticAPIKey,
		APISecret:  cfg.Compliance.EllipticAPISecret,
		BaseURL:    cfg.Compliance.EllipticBaseURL,
		Thresholds: elliptic.Thresholds{}, // runtime-configurable per the source design doc §17 Q8 — not wired to an admin control yet
	})
	complianceService := compliancesvc.NewService(compliancesvc.Deps{
		Repo:              counterpartyRepo,
		Screener:          ellipticClient,
		ScreeningValidity: cfg.Compliance.ScreeningValidity,
		Logger:            logger,
	})

	// Read-only: the admin never signs with these keys, only uses the admin
	// key as a simulation source account for view calls.
	stellarSvc := stellar.NewService(
		cfg.Stellar.NewRpcClient(),
		cfg.Stellar.NetworkPassphrase,
		cfg.Stellar.TreasurySecretKey,
		cfg.Stellar.AdminSecretKey,
		cfg.Stellar.ContractID,
		cfg.Stellar.USDCIssuer,
	)

	isDev := cfg.Server.ServerEnvironment == "development"
	secureCookies := !isDev

	authHandler := handlers.NewAuth(challengeService, jwtService, &cfg.Stellar, secureCookies)
	dashboardHandler := handlers.NewDashboard(metrics.NewService(db))
	loanProductHandler := handlers.NewLoanProducts(loanproduct.NewService(loanProductRepo))
	limitsHandler := handlers.NewLimits(loanlimitconfig.NewService(limitConfigRepo))
	configHandler := handlers.NewConfig(globallendinglimit.NewService(globalLimitRepo))
	guard := middleware.NewAuthMiddleware(jwtService, &cfg.Stellar)

	app := fiber.New(fiber.Config{
		AppName:               "microvault-admin",
		DisableStartupMessage: true,
	})
	app.Use(middleware.RequestID(), middleware.AccessLog(logger, "/static/"), recover.New())

	assets, err := fs.Sub(static.FS, ".")
	if err != nil {
		log.Fatalf("Failed to mount static assets: %v", err)
	}
	// Assets are embedded, so a rebuilt binary always carries fresh bytes. In
	// development the browser must not hold the previous ones.
	staticMaxAge := 3600
	if isDev {
		staticMaxAge = 0
		app.Use("/static", func(c *fiber.Ctx) error {
			c.Set(fiber.HeaderCacheControl, "no-store, must-revalidate")
			return c.Next()
		})
	}
	app.Use("/static", filesystem.New(filesystem.Config{
		Root:   http.FS(assets),
		MaxAge: staticMaxAge,
	}))

	app.Get("/login", authHandler.ShowLogin)
	app.Post("/login/challenge", authHandler.Challenge)
	app.Post("/login/verify", authHandler.Verify)

	app.Use(redirectUnauthenticated, guard.RequireAuth())

	app.Post("/logout", authHandler.Logout)
	loansHandler := handlers.NewLoans(loanRepo)
	usersHandler := handlers.NewUsers(userRepo)
	transactionsHandler := handlers.NewTransactions(txRepo)
	counterpartiesHandler := handlers.NewCounterparties(counterpartyRepo, complianceService)
	screeningHandler := handlers.NewScreening(counterpartyRepo, complianceService, stellarSvc)

	app.Get("/", dashboardHandler.Show)
	app.Get("/loan-products", loanProductHandler.List)
	app.Post("/loan-products", loanProductHandler.Create)
	app.Get("/loans", loansHandler.List)
	app.Get("/users", usersHandler.List)
	app.Get("/transactions", transactionsHandler.List)

	app.Get("/limits", limitsHandler.List)
	app.Post("/limits", limitsHandler.Create)
	app.Get("/config", configHandler.List)
	app.Post("/config", configHandler.Create)

	app.Get("/counterparties", counterpartiesHandler.List)
	app.Post("/counterparties", counterpartiesHandler.Create)
	app.Get("/counterparties/:id", counterpartiesHandler.Show)
	app.Post("/counterparties/:id/approve", counterpartiesHandler.ApproveKYB)
	app.Post("/counterparties/:id/reject", counterpartiesHandler.RejectKYB)
	app.Post("/counterparties/:id/addresses", counterpartiesHandler.SubmitAddress)

	app.Get("/screening", screeningHandler.List)
	app.Get("/screening/:id", screeningHandler.Show)
	app.Post("/screening/:id/approve", screeningHandler.Approve)
	app.Post("/screening/:id/reject", screeningHandler.Reject)
	app.Post("/screening/:id/rescreen", screeningHandler.Rescreen)

	addr := listenAddr()
	go func() {
		logger.Info("admin dashboard listening", "addr", addr)
		if err := app.Listen(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(ctx); err != nil {
		logger.ErrorContext(ctx, "shutdown failed", "error", err)
	}
	if err := shutdownTracing(ctx); err != nil {
		logger.ErrorContext(ctx, "tracing shutdown failed", "error", err)
	}
	logger.InfoContext(ctx, "admin dashboard stopped")
}

// redirectUnauthenticated sends browsers to the sign-in page instead of letting
// RequireAuth return a bare 401 they cannot act on.
func redirectUnauthenticated(c *fiber.Ctx) error {
	if c.Cookies(handlers.SessionCookie) == "" {
		return c.Redirect("/login", fiber.StatusSeeOther)
	}
	return c.Next()
}

func listenAddr() string {
	host := os.Getenv("ADMIN_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8090"
	}
	return host + ":" + port
}
