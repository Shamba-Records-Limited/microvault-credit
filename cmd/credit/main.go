package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/Shamba-Records-Limited/microvault-credit/cmd/credit/docs"
	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/health"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
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
	// ---- Initialize Configuration ----
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// ---- Initialize Database ----
	_, err = database.GetConnection("credit", &cfg.Postgres)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Println("Database connected successfully")

	// ---- Initialize Cache ----
	_, err = cache.GetConnection("credit", &cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Redis connected successfully")

	// ---- Initialize Application ----
	// Create a new fiber app
	app := fiber.New()

	// Initialize health check middleware (without Stellar for credit service)
	healthCheck := health.NewCheckerWithoutStellar("credit", "credit")

	// Initialize middleware & pass health checker middleware
	middleware.FiberMiddleware(app, healthCheck)

	// Define swagger routes
	app.Get("/swagger/*", swagger.New(swagger.Config{
		DeepLinking:  false,
		DocExpansion: "list",
		OAuth: &swagger.OAuthConfig{
			AppName:  "OAuth Provider",
			ClientId: "21bb4edc-05a7-4afc-86f1-2e151e4ba6e2",
		},
		OAuth2RedirectUrl: "http://localhost:8081/swagger/oauth2-redirect.html",
	}))

	// Serve redoc.html at the root route
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("./cmd/credit/docs/redoc-static.html")
	})

	// Create a channel to listen for OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run the server in a separate goroutine
	go func() {
		log.Printf("Starting credit server on %s", cfg.Server.CreditAddr())
		if err := app.Listen(cfg.Server.CreditAddr()); err != nil {
			log.Printf("Server Listen error: %v", err)
		}
	}()

	// Block main goroutine until a signal is received
	sig := <-sigChan
	log.Printf("Received signal %s. Shutting down gracefully...", sig)

	// Tell Fiber to shut down
	if err := app.Shutdown(); err != nil {
		log.Printf("Fiber shutdown error: %v", err)
	}
	log.Println("Fiber server shut down.")

	// Close database connections
	if err := database.CloseAll(); err != nil {
		log.Printf("Database shutdown error: %v", err)
	} else {
		log.Println("Database connections closed successfully.")
	}

	// Close cache connections
	if err := cache.CloseAll(); err != nil {
		log.Printf("Cache shutdown error: %v", err)
	} else {
		log.Println("Cache connections closed successfully.")
	}

	log.Println("Application shutdown complete.")
}
