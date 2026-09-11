package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
	coremodels "github.com/Shamba-Records-Limited/microvault/pkg/models"
	corerepo "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	compliancesvc "github.com/Shamba-Records-Limited/microvault/pkg/services/compliance"
)

// Counterparties lists, creates and approves KYB-onboarding counterparties.
type Counterparties struct {
	repo       corerepo.CounterpartyRepository
	compliance *compliancesvc.Service
}

func NewCounterparties(repo corerepo.CounterpartyRepository, compliance *compliancesvc.Service) *Counterparties {
	return &Counterparties{repo: repo, compliance: compliance}
}

func (h *Counterparties) List(c *fiber.Ctx) error {
	page := views.CounterpartiesPage{
		KYBStatus: c.Query("kyb_status"),
		Page:      queryInt(c, "page", 1),
		PageSize:  defaultPageSize,
		Flash:     c.Query("flash"),
		ShowForm:  c.Query("new") != "",
	}
	offset := (page.Page - 1) * page.PageSize

	cps, err := h.repo.List(c.UserContext(), page.KYBStatus, page.PageSize, offset)
	if err != nil {
		page.Error = "Could not load counterparties: " + err.Error()
		return render(c, views.Counterparties(page))
	}
	count, err := h.repo.Count(c.UserContext(), page.KYBStatus)
	if err != nil {
		page.Error = "Could not load counterparties: " + err.Error()
		return render(c, views.Counterparties(page))
	}

	page.Counterparties = cps
	page.TotalCount = count
	return render(c, views.Counterparties(page))
}

func (h *Counterparties) Create(c *fiber.Ctx) error {
	form := views.CounterpartyForm{
		LegalName:          strings.TrimSpace(c.FormValue("legal_name")),
		RegistrationNumber: strings.TrimSpace(c.FormValue("registration_number")),
		Jurisdiction:       strings.TrimSpace(c.FormValue("jurisdiction")),
	}
	if form.LegalName == "" {
		return h.redisplayCreate(c, form, "Legal name is required")
	}

	// Pre-generate the ID so EllipticCustomerReference (derived from it, per
	// the source design doc §4) can be set on the same insert — the column
	// is NOT NULL and unique, so writing it empty and backfilling later
	// would collide the moment a second counterparty was created before the
	// first one's backfill ran. See Counterparty.BeforeCreate's doc comment.
	id, err := uuid.NewV7()
	if err != nil {
		return h.redisplayCreate(c, form, "Could not generate an ID: "+err.Error())
	}
	cp := &coremodels.Counterparty{
		ID:                        id.String(),
		LegalName:                 form.LegalName,
		RegistrationNumber:        form.RegistrationNumber,
		Jurisdiction:              form.Jurisdiction,
		KYBStatus:                 coremodels.CounterpartyKYBPending,
		EllipticCustomerReference: "kyb-" + id.String(),
	}
	if err := h.repo.Create(c.UserContext(), cp); err != nil {
		return h.redisplayCreate(c, form, err.Error())
	}

	return c.Redirect("/counterparties?flash=Counterparty+created", fiber.StatusSeeOther)
}

func (h *Counterparties) redisplayCreate(c *fiber.Ctx, form views.CounterpartyForm, message string) error {
	page := views.CounterpartiesPage{
		Page:     1,
		PageSize: defaultPageSize,
		Error:    message,
		ShowForm: true,
		Form:     form,
	}
	if cps, err := h.repo.List(c.UserContext(), "", defaultPageSize, 0); err == nil {
		page.Counterparties = cps
	}
	c.Status(fiber.StatusUnprocessableEntity)
	return render(c, views.Counterparties(page))
}

func (h *Counterparties) Show(c *fiber.Ctx) error {
	cp, err := h.repo.GetByID(c.UserContext(), c.Params("id"))
	if err != nil {
		return render(c, views.CounterpartyDetail(views.CounterpartyDetailPage{Error: "Could not load counterparty: " + err.Error()}))
	}
	return render(c, views.CounterpartyDetail(views.CounterpartyDetailPage{
		Counterparty: cp,
		Flash:        c.Query("flash"),
	}))
}

// ApproveKYB approves the counterparty and screens every address already
// on file — see pkg/services/compliance.Service.ApproveKYB. The actor is
// the authenticated admin's Stellar public key, read from the same claims
// the auth middleware already attaches to every guarded request — the
// first write handler in this admin to actually use it (see the source
// design doc §14's audit-column note).
func (h *Counterparties) ApproveKYB(c *fiber.Ctx) error {
	id := c.Params("id")
	actor := middleware.GetAdminClaims(c).AdminPublicKey
	if err := h.compliance.ApproveKYB(c.UserContext(), id, actor); err != nil {
		return h.redisplayDetail(c, id, "Approval failed: "+err.Error())
	}
	return c.Redirect("/counterparties/"+id+"?flash=KYB+approved", fiber.StatusSeeOther)
}

func (h *Counterparties) RejectKYB(c *fiber.Ctx) error {
	id := c.Params("id")
	actor := middleware.GetAdminClaims(c).AdminPublicKey
	if err := h.repo.RejectKYB(c.UserContext(), id, actor); err != nil {
		return h.redisplayDetail(c, id, "Rejection failed: "+err.Error())
	}
	return c.Redirect("/counterparties/"+id+"?flash=KYB+rejected", fiber.StatusSeeOther)
}

// SubmitAddress adds an address to a counterparty — screened immediately
// if KYB is already approved, per pkg/services/compliance.Service.
func (h *Counterparties) SubmitAddress(c *fiber.Ctx) error {
	id := c.Params("id")
	address := strings.TrimSpace(c.FormValue("address"))
	if _, err := h.compliance.SubmitAddress(c.UserContext(), id, address); err != nil {
		return h.redisplayDetail(c, id, "Could not submit address: "+err.Error())
	}
	return c.Redirect("/counterparties/"+id+"?flash=Address+submitted", fiber.StatusSeeOther)
}

// redisplayDetail re-renders the counterparty detail page with an inline
// error, instead of redirecting with the error folded into a URL —
// unescaped dynamic content (error text, or id itself, which is an
// attacker-influenced route param) in a Location header is fragile at
// best. This matches the one existing write-action precedent in this
// admin (handlers/limits.go's Create/redisplay), which never puts dynamic
// content in a redirect URL either.
func (h *Counterparties) redisplayDetail(c *fiber.Ctx, id, message string) error {
	page := views.CounterpartyDetailPage{Error: message}
	if cp, err := h.repo.GetByID(c.UserContext(), id); err == nil {
		page.Counterparty = cp
	}
	c.Status(fiber.StatusUnprocessableEntity)
	return render(c, views.CounterpartyDetail(page))
}
