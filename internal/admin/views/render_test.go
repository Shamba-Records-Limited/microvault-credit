package views_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/metrics"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
	"github.com/a-h/templ"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := c.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

func mustContain(t *testing.T, html string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML missing %q", want)
		}
	}
}

func TestLoginRendersSigningAffordance(t *testing.T) {
	html := render(t, views.Login("Test SDF Network ; September 2015", ""))
	mustContain(t, html,
		`id="signin"`,
		`data-network="Test SDF Network ; September 2015"`,
		"/static/login.js",
	)
	if strings.Contains(html, "Sign out") {
		t.Error("sign-in page should not render the authenticated shell")
	}
}

func TestLoginShowsFlash(t *testing.T) {
	html := render(t, views.Login("passphrase", "not signed by the admin key"))
	mustContain(t, html, "not signed by the admin key")
}

func TestShellMarksCurrentNavItem(t *testing.T) {
	html := render(t, views.Placeholder("Loans", "/loans", "not yet"))
	mustContain(t, html, `aria-current="page"`, "Loans", "not yet")

	// Every nav destination should be reachable from the shell.
	for _, item := range views.Nav {
		mustContain(t, html, `href="`+item.Href+`"`)
	}
}

func TestDashboardRendersMetrics(t *testing.T) {
	html := render(t, views.Dashboard(views.DashboardPage{
		WindowDays: 30,
		Snapshot: &metrics.Snapshot{
			Outstanding: []metrics.AssetTotal{{Asset: "USDC", Amount: 1234567}},
			ActiveLoans: 42,
			DefaultRate: 3.5,
			Alerts: []metrics.Alert{
				{Label: "Off-ramp failed", Count: 2, Note: "Borrower never received funds"},
				{Label: "Refund shortfall", Count: 0, Note: "Anchor returned less"},
			},
		},
	}))

	mustContain(t, html,
		"USDC 12,345.67",
		"42",
		"3.5%",
		"Off-ramp failed",
		"Refund shortfall",
		"Disbursed (30d)",
	)
}

func TestDashboardRendersErrorWithoutSnapshot(t *testing.T) {
	html := render(t, views.Dashboard(views.DashboardPage{Error: "database unreachable"}))
	mustContain(t, html, "database unreachable")
}

func TestLoanProductsTable(t *testing.T) {
	fee := int32(250)
	description := "Instant mobile money loan"
	html := render(t, views.LoanProducts(views.LoanProductsPage{
		Products: []loanproduct.LoanProductResponse{{
			Name:                      "shamba_instant_loan",
			Description:               &description,
			InterestRateBps:           500,
			InterestType:              "simple",
			OriginationFeeBps:         &fee,
			MinAmount:                 50000,
			MaxAmount:                 300000,
			Currency:                  "KES",
			MinDurationDays:           30,
			MaxDurationDays:           30,
			AllowedRepaymentSchedules: []string{"lump_sum"},
			IsActive:                  true,
		}},
	}))

	mustContain(t, html,
		"shamba_instant_loan",
		"5.00%",
		"2.50%",
		"KES 500.00",
		"KES 3,000.00",
		"30 days",
		"lump_sum",
		"Active",
	)
}

func TestLoanProductsEmptyState(t *testing.T) {
	html := render(t, views.LoanProducts(views.LoanProductsPage{}))
	mustContain(t, html, "No loan products yet.")
}

func TestLoanProductFormRedisplaysSubmittedValues(t *testing.T) {
	html := render(t, views.LoanProducts(views.LoanProductsPage{
		ShowForm: true,
		Error:    "maximum amount must be greater than minimum",
		Form: views.LoanProductForm{
			Name:            "bad_product",
			InterestRateBps: "900",
			Currency:        "KES",
			InterestType:    "compound",
		},
	}))

	mustContain(t, html,
		"maximum amount must be greater than minimum",
		`value="bad_product"`,
		`value="900"`,
		`<option value="compound" selected`,
	)
}

func TestMoneyFormatting(t *testing.T) {
	cases := []struct {
		cents    int64
		currency string
		want     string
	}{
		{0, "KES", "KES 0.00"},
		{5, "KES", "KES 0.05"},
		{50000, "KES", "KES 500.00"},
		{300000, "KES", "KES 3,000.00"},
		{123456789, "USDC", "USDC 1,234,567.89"},
		{-50000, "KES", "-KES 500.00"},
	}
	for _, tc := range cases {
		if got := views.Money(tc.cents, tc.currency); got != tc.want {
			t.Errorf("Money(%d, %q) = %q, want %q", tc.cents, tc.currency, got, tc.want)
		}
	}
}
