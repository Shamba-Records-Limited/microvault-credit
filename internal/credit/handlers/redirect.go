package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

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

// Handle resolves :code and redirects. Unknown codes 404.
func (h *InteractiveRedirectHandler) Handle(c *fiber.Ctx) error {
	code := c.Params("code")
	if code == "" {
		return c.SendStatus(fiber.StatusNotFound)
	}

	resp, err := h.loans.GetByRampShortCode(c.UserContext(), code)
	if err != nil {
		if !errors.Is(err, loan.ErrLoanNotFound) {
			// Unexpected — e.g. a missing ramp_short_code column (migration not
			// applied). Surface it rather than masking as a plain 404.
			h.logger.ErrorContext(c.UserContext(), "short-code lookup failed", "code", code, "error", err)
		}
		return c.SendStatus(fiber.StatusNotFound)
	}
	if resp == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}

	if matches(resp.RampMoreInfoShortCode, code) {
		if resp.RampMoreInfoURL == nil || *resp.RampMoreInfoURL == "" {
			h.logger.WarnContext(c.UserContext(), "more-info code resolved but no URL", "code", code)
			return c.SendStatus(fiber.StatusNotFound)
		}
		return c.Redirect(*resp.RampMoreInfoURL, fiber.StatusFound)
	}

	if resp.RampInteractiveURL == nil || *resp.RampInteractiveURL == "" {
		h.logger.WarnContext(c.UserContext(), "short-code resolved but no interactive URL", "code", code)
		return c.SendStatus(fiber.StatusNotFound)
	}
	if isTerminalDisbursement(resp.DisbursementStatus) {
		return c.SendStatus(fiber.StatusGone)
	}
	// The status gate alone leaves a code live indefinitely on a loan that never
	// reaches a terminal state. Nil expiry means a row predating migration
	// 000024, which keeps the status-only behaviour.
	if resp.RampShortCodeExpiresAt != nil && time.Now().After(*resp.RampShortCodeExpiresAt) {
		return c.SendStatus(fiber.StatusGone)
	}

	return c.Redirect(*resp.RampInteractiveURL, fiber.StatusFound)
}

func matches(stored *string, code string) bool {
	return stored != nil && *stored == code
}

// isTerminalDisbursement reports whether the withdrawal has finished, however
// it finished. DisbursementStatus is derived from the loan rather than stored;
// see models.Loan.DeriveDisbursementStatus.
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
