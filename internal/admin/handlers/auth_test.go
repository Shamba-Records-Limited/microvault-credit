package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/handlers"
)

// guardedApp mirrors the route ordering in cmd/admin: public sign-in routes,
// then the redirect, then everything else.
func guardedApp() *fiber.App {
	app := fiber.New()
	app.Get("/login", func(c *fiber.Ctx) error { return c.SendString("sign in") })

	app.Use(func(c *fiber.Ctx) error {
		if c.Cookies(handlers.SessionCookie) == "" {
			return c.Redirect("/login", fiber.StatusSeeOther)
		}
		return c.Next()
	})

	app.Get("/", func(c *fiber.Ctx) error { return c.SendString("dashboard") })
	app.Get("/loan-products", func(c *fiber.Ctx) error { return c.SendString("products") })
	app.Post("/loan-products", func(c *fiber.Ctx) error { return c.SendString("created") })
	return app
}

func TestGuardRedirectsAnonymousRequests(t *testing.T) {
	app := guardedApp()

	for _, target := range []struct{ method, path string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/loan-products"},
		{http.MethodPost, "/loan-products"},
	} {
		res, err := app.Test(httptest.NewRequest(target.method, target.path, nil))
		if err != nil {
			t.Fatalf("%s %s: %v", target.method, target.path, err)
		}
		if res.StatusCode != fiber.StatusSeeOther {
			t.Errorf("%s %s: status = %d, want %d", target.method, target.path, res.StatusCode, fiber.StatusSeeOther)
		}
		if loc := res.Header.Get("Location"); loc != "/login" {
			t.Errorf("%s %s: Location = %q, want /login", target.method, target.path, loc)
		}
	}
}

func TestSignInPageStaysPublic(t *testing.T) {
	res, err := guardedApp().Test(httptest.NewRequest(http.MethodGet, "/login", nil))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}

func TestGuardAdmitsRequestsCarryingTheCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: handlers.SessionCookie, Value: "some-token"})

	res, err := guardedApp().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
}
