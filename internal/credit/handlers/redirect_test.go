package handlers

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

type stubResolver struct {
	resp *loan.LoanResponse
	err  error
}

func (s stubResolver) GetByRampShortCode(context.Context, string) (*loan.LoanResponse, error) {
	return s.resp, s.err
}

func strptr(s string) *string { return &s }

func newTestApp(r stubResolver) *fiber.App {
	app := fiber.New()
	h := NewInteractiveRedirectHandler(r, nil)
	app.Get("/r/:code", h.Handle)
	return app
}

func TestRedirect_ValidCodeRedirects(t *testing.T) {
	pending := models.DisbursementStatusProcessing
	app := newTestApp(stubResolver{resp: &loan.LoanResponse{
		RampInteractiveURL: strptr("https://stellar.moneygram.com/sep24?token=xyz"),
		DisbursementStatus: &pending,
	}})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/r/abc123", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("got %d, want 302", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://stellar.moneygram.com/sep24?token=xyz" {
		t.Fatalf("unexpected Location: %q", loc)
	}
}

func TestRedirect_UnknownCode404(t *testing.T) {
	app := newTestApp(stubResolver{err: loan.ErrLoanNotFound})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/r/missing", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}

func TestRedirect_SettledLoan410(t *testing.T) {
	done := models.DisbursementStatusCompleted
	app := newTestApp(stubResolver{resp: &loan.LoanResponse{
		RampInteractiveURL: strptr("https://stellar.moneygram.com/sep24?token=xyz"),
		DisbursementStatus: &done,
	}})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/r/spent", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusGone {
		t.Fatalf("got %d, want 410", resp.StatusCode)
	}
}
