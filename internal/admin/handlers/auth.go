package handlers

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	"github.com/Shamba-Records-Limited/microvault/pkg/auth"
	"github.com/Shamba-Records-Limited/microvault/pkg/config"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
)

// SessionCookie is the cookie name middleware.AuthMiddleware reads.
const SessionCookie = "admin_token"

// Auth serves the sign-in page and the challenge/verify exchange behind it.
type Auth struct {
	challenges auth.ChallengeService
	jwt        *auth.JWTService
	stellar    *config.StellarConfig
	secure     bool
}

func NewAuth(challenges auth.ChallengeService, jwt *auth.JWTService, stellar *config.StellarConfig, secure bool) *Auth {
	return &Auth{challenges: challenges, jwt: jwt, stellar: stellar, secure: secure}
}

func (h *Auth) ShowLogin(c *fiber.Ctx) error {
	if c.Cookies(SessionCookie) != "" {
		return c.Redirect("/", fiber.StatusSeeOther)
	}
	return render(c, views.Login(h.stellar.NetworkPassphrase, c.Query("error")))
}

func (h *Auth) Challenge(c *fiber.Ctx) error {
	challenge, err := h.challenges.GenerateChallenge(c.UserContext())
	if err != nil {
		middleware.NoteError(c, err)
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate challenge")
	}
	return c.JSON(fiber.Map{"id": challenge.ID, "transaction": challenge.Transaction})
}

type verifyRequest struct {
	ChallengeID       string `json:"challenge_id"`
	SignedTransaction string `json:"signed_transaction"`
}

func (h *Auth) Verify(c *fiber.Ctx) error {
	var body verifyRequest
	if err := c.BodyParser(&body); err != nil {
		middleware.NoteError(c, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.ChallengeID == "" || body.SignedTransaction == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing challenge or signature"})
	}

	if err := h.challenges.VerifySignedChallenge(c.UserContext(), body.ChallengeID, body.SignedTransaction); err != nil {
		middleware.NoteError(c, err)
		status, message := verifyError(err)
		return c.Status(status).JSON(fiber.Map{"error": message})
	}

	token, expiresAt, err := h.jwt.GenerateToken(h.stellar.AdminPublicKey)
	if err != nil {
		middleware.NoteError(c, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to issue session"})
	}
	h.setSession(c, token, expiresAt)

	return c.JSON(fiber.Map{"ok": true})
}

func (h *Auth) Logout(c *fiber.Ctx) error {
	h.setSession(c, "", time.Unix(0, 0))
	return c.Redirect("/login", fiber.StatusSeeOther)
}

func (h *Auth) setSession(c *fiber.Ctx, token string, expiresAt time.Time) {
	c.Cookie(&fiber.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HTTPOnly: true,
		Secure:   h.secure,
		SameSite: "Strict",
	})
}

func verifyError(err error) (int, string) {
	switch {
	case errors.Is(err, auth.ErrChallengeNotFound):
		return fiber.StatusNotFound, "challenge not found"
	case errors.Is(err, auth.ErrChallengeExpired):
		return fiber.StatusUnauthorized, "challenge expired"
	case errors.Is(err, auth.ErrInvalidTransaction):
		return fiber.StatusBadRequest, "invalid transaction"
	case errors.Is(err, auth.ErrTransactionMismatch):
		return fiber.StatusUnauthorized, "transaction does not match challenge"
	case errors.Is(err, auth.ErrInvalidSignature):
		return fiber.StatusUnauthorized, "not signed by the admin key"
	default:
		return fiber.StatusInternalServerError, "verification failed"
	}
}
