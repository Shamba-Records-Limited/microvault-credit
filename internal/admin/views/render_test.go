package views_test

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/metrics"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	globallendinglimit "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/global_lending_limit"
	loanlimitconfig "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_limit_config"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
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
			Outstanding: []metrics.AssetTotal{{Asset: "USDC", Amount: 123456700000}},
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
		amount int64
		asset  string
		want   string
	}{
		{0, "KES", "KES 0.00"},
		{5, "KES", "KES 0.05"},
		{50000, "KES", "KES 500.00"},
		{300000, "KES", "KES 3,000.00"},
		{123456789, "USDC", "USDC 12.34"},
		{123456700000, "USDC", "USDC 12,345.67"},
		{15000000, "USDC", "USDC 1.50"},
		{123456789, "KES", "KES 1,234,567.89"},
		{-50000, "KES", "-KES 500.00"},
	}
	for _, tc := range cases {
		if got := views.Money(tc.amount, tc.asset); got != tc.want {
			t.Errorf("Money(%d, %q) = %q, want %q", tc.amount, tc.asset, got, tc.want)
		}
	}
}

func TestLimitsRendersTableAndCreateAffordance(t *testing.T) {
	lateFee := int32(150)
	html := render(t, views.Limits(views.LimitsPage{
		Configs: []loanlimitconfig.LoanLimitConfigResponse{{
			RiskTier:            "standard",
			MinLoanAmount:       50000,
			MaxLoanAmount:       300000,
			IncomeMultiplierBps: 10000,
			MaxConcurrentLoans:  1,
			MaxLoanDurationDays: 30,
			InterestRateBps:     500,
			LateFeeBps:          &lateFee,
			IsActive:            true,
		}},
	}))

	mustContain(t, html, "standard", "50,000", "300,000", "5.00%", "1.50%", "30 days", "Active", `href="/limits?new=1"`)
}

func TestLimitsEmptyStateStillOffersCreate(t *testing.T) {
	html := render(t, views.Limits(views.LimitsPage{}))
	mustContain(t, html, "No risk-tier limits configured yet.", `href="/limits?new=1"`)
}

func TestLimitsFormPostsToLimits(t *testing.T) {
	html := render(t, views.Limits(views.LimitsPage{ShowForm: true, Form: views.DefaultLimitForm()}))
	mustContain(t, html, `action="/limits"`, `name="risk_tier"`, `name="max_concurrent_loans"`, "Create risk tier")
}

func TestConfigRendersTableAndCreateAffordance(t *testing.T) {
	category := "limits"
	description := "Ceiling on total outstanding principal"
	html := render(t, views.Config(views.ConfigPage{
		Limits: []globallendinglimit.GlobalLendingLimitResponse{{
			ConfigKey:   "max_total_exposure",
			ConfigValue: "1000000",
			ValueType:   "integer",
			Category:    &category,
			Description: &description,
			IsActive:    true,
		}},
	}))

	mustContain(t, html, "max_total_exposure", "1000000", "integer", "limits", description, "Active", `href="/config?new=1"`)
}

func TestConfigEmptyStateStillOffersCreate(t *testing.T) {
	html := render(t, views.Config(views.ConfigPage{}))
	mustContain(t, html, "No global lending limits configured yet.", `href="/config?new=1"`)
}

func TestConfigFormOffersEveryValueType(t *testing.T) {
	html := render(t, views.Config(views.ConfigPage{ShowForm: true, Form: views.DefaultConfigForm()}))
	mustContain(t, html, `action="/config"`, `name="config_key"`, "Create setting")
	for _, vt := range views.ConfigValueTypes {
		mustContain(t, html, `<option value="`+vt+`"`)
	}
	for _, cat := range views.ConfigCategories {
		mustContain(t, html, `<option value="`+cat+`"`)
	}
}

func TestLogoInlinesSVGWithCurrentColor(t *testing.T) {
	html := render(t, views.Logo("size-6"))
	mustContain(t, html, "<svg", `class="size-6"`, `fill="currentColor"`, `aria-hidden="true"`)
	if strings.Contains(html, `fill="black"`) {
		t.Error("logo still carries hardcoded black fills; it will vanish in dark mode")
	}
}

func TestShellRendersBrandingAndFavicon(t *testing.T) {
	html := render(t, views.Placeholder("Loans", "/loans", "not yet"))
	mustContain(t, html,
		`href="/static/favicon.ico"`,
		`href="/static/img/apple-touch-icon.png"`,
		`aria-label="Microvault Portal home"`,
		`fill="currentColor"`,
	)
}

func TestLoginRendersLogo(t *testing.T) {
	html := render(t, views.Login("passphrase", ""))
	mustContain(t, html, `class="size-12"`, `href="/static/favicon.ico"`)
}
