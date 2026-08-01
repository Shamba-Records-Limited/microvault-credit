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
	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/repository"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
	"github.com/Shamba-Records-Limited/microvault/pkg/auth"
	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
	"github.com/Shamba-Records-Limited/microvault/platform/cache"
	"github.com/Shamba-Records-Limited/microvault/platform/database"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
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

	secureCookies := cfg.Server.ServerEnvironment != "development"

	authHandler := handlers.NewAuth(challengeService, jwtService, &cfg.Stellar, secureCookies)
	dashboardHandler := handlers.NewDashboard(metrics.NewService(db))
	loanProductHandler := handlers.NewLoanProducts(loanproduct.NewService(loanProductRepo))
	guard := middleware.NewAuthMiddleware(jwtService, &cfg.Stellar)

	app := fiber.New(fiber.Config{
		AppName:               "microvault-admin",
		DisableStartupMessage: true,
	})
	app.Use(recover.New())

	assets, err := fs.Sub(static.FS, ".")
	if err != nil {
		log.Fatalf("Failed to mount static assets: %v", err)
	}
	app.Use("/static", filesystem.New(filesystem.Config{
		Root:   http.FS(assets),
		MaxAge: 3600,
	}))

	app.Get("/login", authHandler.ShowLogin)
	app.Post("/login/challenge", authHandler.Challenge)
	app.Post("/login/verify", authHandler.Verify)

	app.Use(redirectUnauthenticated, guard.RequireAuth())

	app.Post("/logout", authHandler.Logout)
	app.Get("/", dashboardHandler.Show)
	app.Get("/loan-products", loanProductHandler.List)
	app.Post("/loan-products", loanProductHandler.Create)

	for _, pending := range []struct{ path, title, note string }{
		{"/loans", "Loans", "The loans table needs filtered list and count queries first."},
		{"/transactions", "Transactions", "The transactions table needs filtered list and count queries first."},
		{"/users", "Users", "The users table needs filtered list and count queries first."},
		{"/limits", "Limits", "Risk-tier loan limits are not wired up yet."},
		{"/config", "Config", "Global lending limits are not wired up yet."},
	} {
		app.Get(pending.path, placeholder(pending.title, pending.path, pending.note))
	}

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

func placeholder(title, current, note string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return views.Placeholder(title, current, note).Render(c.UserContext(), c.Response().BodyWriter())
	}
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
