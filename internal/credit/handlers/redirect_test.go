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

// The support link's whole purpose is to work after the withdrawal settles —
// when the borrower is at an agent with a problem. Routing it through the same
// terminal check as the interactive URL would 410 it exactly then.
func TestRedirect_MoreInfoLinkSurvivesSettlement(t *testing.T) {
	settled := models.DisbursementStatusCompleted
	app := newTestApp(stubResolver{resp: &loan.LoanResponse{
		RampInteractiveURL:    strptr("https://stellar.moneygram.com/sep24?token=xyz"),
		RampShortCode:         strptr("interact"),
		RampMoreInfoURL:       strptr("https://moneygram.com/support/72540163"),
		RampMoreInfoShortCode: strptr("support1"),
		DisbursementStatus:    &settled,
	}})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/r/support1", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("got %d, want 302 — the support link must outlive the withdrawal", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "https://moneygram.com/support/72540163" {
		t.Fatalf("redirected to %q, want the support URL", got)
	}
}

// The interactive webview must stop working once settled, so a leaked SMS
// cannot reopen a completed session.
func TestRedirect_InteractiveLinkDiesOnSettlement(t *testing.T) {
	settled := models.DisbursementStatusCompleted
	app := newTestApp(stubResolver{resp: &loan.LoanResponse{
		RampInteractiveURL:    strptr("https://stellar.moneygram.com/sep24?token=xyz"),
		RampShortCode:         strptr("interact"),
		RampMoreInfoURL:       strptr("https://moneygram.com/support/72540163"),
		RampMoreInfoShortCode: strptr("support1"),
		DisbursementStatus:    &settled,
	}})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/r/interact", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusGone {
		t.Fatalf("got %d, want 410", resp.StatusCode)
	}
}

// A loan whose more-info URL has not arrived yet must 404 rather than redirect
// to an empty location.
func TestRedirect_MoreInfoCodeWithoutURL(t *testing.T) {
	app := newTestApp(stubResolver{resp: &loan.LoanResponse{
		RampMoreInfoShortCode: strptr("support1"),
	}})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/r/support1", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
}
