package handlers

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services"
	loanproduct "github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan_product"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
)

const defaultPageSize = 25

// LoanProducts lists and creates loan products.
type LoanProducts struct {
	products loanproduct.Service
}

func NewLoanProducts(products loanproduct.Service) *LoanProducts {
	return &LoanProducts{products: products}
}

func (h *LoanProducts) List(c *fiber.Ctx) error {
	page := views.LoanProductsPage{
		Page:     queryInt(c, "page", 1),
		PageSize: defaultPageSize,
		Flash:    c.Query("flash"),
		ShowForm: c.Query("new") != "",
		Form:     views.DefaultLoanProductForm(),
	}

	result, err := h.products.GetAll(c.UserContext(), services.Pagination{
		Page:     page.Page,
		PageSize: page.PageSize,
	})
	if err != nil {
		middleware.NoteError(c, err)
		page.Error = "Could not load loan products: " + err.Error()
	} else {
		page.Products = result.Data
	}

	return render(c, views.LoanProducts(page))
}

func (h *LoanProducts) Create(c *fiber.Ctx) error {
	form := formFromRequest(c)

	req, err := buildCreateRequest(form)
	if err != nil {
		middleware.NoteError(c, err)
		return h.redisplay(c, form, err.Error())
	}

	if _, err := h.products.Create(c.UserContext(), req); err != nil {
		middleware.NoteError(c, err)
		return h.redisplay(c, form, err.Error())
	}

	return c.Redirect("/loan-products?flash=Loan+product+created", fiber.StatusSeeOther)
}

func (h *LoanProducts) redisplay(c *fiber.Ctx, form views.LoanProductForm, message string) error {
	page := views.LoanProductsPage{
		Page:     1,
		PageSize: defaultPageSize,
		Error:    message,
		ShowForm: true,
		Form:     form,
	}

	if result, err := h.products.GetAll(c.UserContext(), services.Pagination{
		Page:     page.Page,
		PageSize: page.PageSize,
	}); err == nil {
		page.Products = result.Data
	}

	c.Status(fiber.StatusUnprocessableEntity)
	return render(c, views.LoanProducts(page))
}

func formFromRequest(c *fiber.Ctx) views.LoanProductForm {
	return views.LoanProductForm{
		Name:                   strings.TrimSpace(c.FormValue("name")),
		Description:            strings.TrimSpace(c.FormValue("description")),
		InterestRateBps:        c.FormValue("interest_rate_bps"),
		InterestType:           c.FormValue("interest_type"),
		OriginationFeeBps:      c.FormValue("origination_fee_bps"),
		MinAmount:              c.FormValue("min_amount"),
		MaxAmount:              c.FormValue("max_amount"),
		Currency:               strings.TrimSpace(c.FormValue("currency")),
		MinDurationDays:        c.FormValue("min_duration_days"),
		MaxDurationDays:        c.FormValue("max_duration_days"),
		RepaymentSchedules:     c.FormValue("repayment_schedules"),
		MaxCreditMultiplierBps: c.FormValue("max_credit_multiplier_bps"),
		PriorityOrder:          c.FormValue("priority_order"),
		IsActive:               c.FormValue("is_active") == "true",
	}
}

func buildCreateRequest(f views.LoanProductForm) (loanproduct.CreateLoanProductRequest, error) {
	var req loanproduct.CreateLoanProductRequest

	interestRate, err := parseInt32(f.InterestRateBps, "interest rate")
	if err != nil {
		return req, err
	}
	minAmount, err := parseInt64(f.MinAmount, "minimum amount")
	if err != nil {
		return req, err
	}
	maxAmount, err := parseInt64(f.MaxAmount, "maximum amount")
	if err != nil {
		return req, err
	}
	minDuration, err := parseInt(f.MinDurationDays, "minimum duration")
	if err != nil {
		return req, err
	}
	maxDuration, err := parseInt(f.MaxDurationDays, "maximum duration")
	if err != nil {
		return req, err
	}
	multiplier, err := parseInt32(f.MaxCreditMultiplierBps, "credit multiplier")
	if err != nil {
		return req, err
	}

	schedules := splitSchedules(f.RepaymentSchedules)
	if len(schedules) == 0 {
		return req, fiber.NewError(fiber.StatusBadRequest, "at least one repayment schedule is required")
	}

	req = loanproduct.CreateLoanProductRequest{
		Name:                      f.Name,
		Description:               optionalString(f.Description),
		InterestRateBps:           interestRate,
		InterestType:              f.InterestType,
		OriginationFeeBps:         optionalInt32(f.OriginationFeeBps),
		MinAmount:                 minAmount,
		MaxAmount:                 maxAmount,
		Currency:                  f.Currency,
		MinDurationDays:           minDuration,
		MaxDurationDays:           maxDuration,
		AllowedRepaymentSchedules: schedules,
		MaxCreditMultiplierBps:    multiplier,
		PriorityOrder:             optionalIntValue(f.PriorityOrder),
		IsActive:                  f.IsActive,
	}

	return req, nil
}

func splitSchedules(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optionalInt32(s string) *int32 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return nil
	}
	value := int32(parsed)
	return &value
}

func optionalIntValue(s string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return parsed
}

func parseInt32(s, field string) (int32, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(s), 10, 32)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, field+" must be a whole number")
	}
	return int32(parsed), nil
}

func parseInt64(s, field string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, field+" must be a whole number")
	}
	return parsed, nil
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func parseInt(s, field string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, field+" must be a whole number")
	}
	return parsed, nil
}

func queryInt(c *fiber.Ctx, key string, fallback int) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
