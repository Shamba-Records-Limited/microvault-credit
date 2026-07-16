package handlers

import (
	"context"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/services/loan"
)

// ShortCodeResolver resolves a /r/{code} short-link to its loan. loan.Service
// satisfies it.
type ShortCodeResolver interface {
	GetByRampShortCode(ctx context.Context, code string) (*loan.LoanResponse, error)
}

// InteractiveRedirectHandler serves the /r/{code} SMS short-link, resolving it
// to a loan's MoneyGram interactive URL and 302-redirecting the user there.
type InteractiveRedirectHandler struct {
	loans  ShortCodeResolver
	logger *slog.Logger
}

// NewInteractiveRedirectHandler wires the handler to the loan service.
func NewInteractiveRedirectHandler(loans ShortCodeResolver, logger *slog.Logger) *InteractiveRedirectHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &InteractiveRedirectHandler{
		loans:  loans,
		logger: logger.With("component", "interactive_redirect"),
	}
}

// Handle resolves :code to the loan's interactive URL and redirects. Unknown
// codes 404; codes on a settled loan 410 so a leaked link can't reopen a
// completed session.
func (h *InteractiveRedirectHandler) Handle(c *fiber.Ctx) error {
	code := c.Params("code")
	if code == "" {
		return c.SendStatus(fiber.StatusNotFound)
	}

	resp, err := h.loans.GetByRampShortCode(c.UserContext(), code)
	if err != nil || resp == nil || resp.RampInteractiveURL == nil || *resp.RampInteractiveURL == "" {
		return c.SendStatus(fiber.StatusNotFound)
	}

	if isTerminalDisbursement(resp.DisbursementStatus) {
		return c.SendStatus(fiber.StatusGone)
	}

	return c.Redirect(*resp.RampInteractiveURL, fiber.StatusFound)
}

func isTerminalDisbursement(status *string) bool {
	if status == nil {
		return false
	}
	switch *status {
	case models.DisbursementStatusCompleted, models.DisbursementStatusFailed:
		return true
	}
	return strings.Contains(*status, "refund")
}
