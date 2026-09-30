package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	loanlimitconfig "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_limit_config"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
)

// Limits lists and creates per-risk-tier loan limits.
type Limits struct {
	configs loanlimitconfig.Service
}

func NewLimits(configs loanlimitconfig.Service) *Limits {
	return &Limits{configs: configs}
}

func (h *Limits) List(c *fiber.Ctx) error {
	page := views.LimitsPage{
		Page:     queryInt(c, "page", 1),
		PageSize: defaultPageSize,
		Flash:    c.Query("flash"),
		ShowForm: c.Query("new") != "",
		Form:     views.DefaultLimitForm(),
	}

	if configs, err := h.load(c, page.Page); err != nil {
		middleware.NoteError(c, err)
		page.Error = "Could not load risk-tier limits: " + err.Error()
	} else {
		page.Configs = configs
	}

	return render(c, views.Limits(page))
}

func (h *Limits) Create(c *fiber.Ctx) error {
	form := views.LimitForm{
		RiskTier:            strings.TrimSpace(c.FormValue("risk_tier")),
		MinLoanAmount:       c.FormValue("min_loan_amount"),
		MaxLoanAmount:       c.FormValue("max_loan_amount"),
		IncomeMultiplierBps: c.FormValue("income_multiplier_bps"),
		MaxConcurrentLoans:  c.FormValue("max_concurrent_loans"),
		MaxLoanDurationDays: c.FormValue("max_loan_duration_days"),
		InterestRateBps:     c.FormValue("interest_rate_bps"),
		LateFeeBps:          c.FormValue("late_fee_bps"),
		DefaultPenaltyBps:   c.FormValue("default_penalty_bps"),
		IsActive:            c.FormValue("is_active") == "true",
	}

	req, err := buildLimitRequest(form)
	if err != nil {
		middleware.NoteError(c, err)
		return h.redisplay(c, form, err.Error())
	}

	if _, err := h.configs.Create(c.UserContext(), req); err != nil {
		middleware.NoteError(c, err)
		return h.redisplay(c, form, err.Error())
	}

	return c.Redirect("/limits?flash=Risk+tier+created", fiber.StatusSeeOther)
}

func (h *Limits) load(c *fiber.Ctx, page int) ([]loanlimitconfig.LoanLimitConfigResponse, error) {
	result, err := h.configs.GetAll(c.UserContext(), services.Pagination{Page: page, PageSize: defaultPageSize})
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (h *Limits) redisplay(c *fiber.Ctx, form views.LimitForm, message string) error {
	page := views.LimitsPage{
		Page:     1,
		PageSize: defaultPageSize,
		Error:    message,
		ShowForm: true,
		Form:     form,
	}
	if configs, err := h.load(c, 1); err == nil {
		page.Configs = configs
	}

	c.Status(fiber.StatusUnprocessableEntity)
	return render(c, views.Limits(page))
}

func buildLimitRequest(f views.LimitForm) (loanlimitconfig.CreateLoanLimitConfigRequest, error) {
	var req loanlimitconfig.CreateLoanLimitConfigRequest

	if f.RiskTier == "" {
		return req, fiber.NewError(fiber.StatusBadRequest, "risk tier is required")
	}

	minAmount, err := parseInt64(f.MinLoanAmount, "minimum loan amount")
	if err != nil {
		return req, err
	}
	maxAmount, err := parseInt64(f.MaxLoanAmount, "maximum loan amount")
	if err != nil {
		return req, err
	}
	multiplier, err := parseInt32(f.IncomeMultiplierBps, "income multiplier")
	if err != nil {
		return req, err
	}
	concurrent, err := parseInt(f.MaxConcurrentLoans, "max concurrent loans")
	if err != nil {
		return req, err
	}
	duration, err := parseInt(f.MaxLoanDurationDays, "max loan duration")
	if err != nil {
		return req, err
	}
	rate, err := parseInt32(f.InterestRateBps, "interest rate")
	if err != nil {
		return req, err
	}

	// CreatedBy is left unset: admin identity is a Stellar public key and the
	// column is a user UUID. See the audit-column decision in the plan doc.
	req = loanlimitconfig.CreateLoanLimitConfigRequest{
		RiskTier:            f.RiskTier,
		MinLoanAmount:       minAmount,
		MaxLoanAmount:       maxAmount,
		IncomeMultiplierBps: multiplier,
		MaxConcurrentLoans:  concurrent,
		MaxLoanDurationDays: duration,
		InterestRateBps:     rate,
		LateFeeBps:          optionalInt32(f.LateFeeBps),
		DefaultPenaltyBps:   optionalInt32(f.DefaultPenaltyBps),
		IsActive:            f.IsActive,
	}

	return req, nil
}
