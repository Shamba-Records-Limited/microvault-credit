// Command admin serves the Microvault admin dashboard.
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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/handlers"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/metrics"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/static"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	globallendinglimit "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/global_lending_limit"
	loanlimitconfig "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_limit_config"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
	"github.com/Shamba-Records-Limited/microvault/pkg/auth"
	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
	corerepository "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	"github.com/Shamba-Records-Limited/microvault/platform/cache"
	"github.com/Shamba-Records-Limited/microvault/platform/database"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	fiberlog "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

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
	app.Use(recover.New())
	app.Use(fiberlog.New(fiberlog.Config{
		Format: "${time} ${status} ${latency} ${method} ${path}\n",
	}))

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
		logger.Error("shutdown failed", "error", err)
	}
	logger.Info("admin dashboard stopped")
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
