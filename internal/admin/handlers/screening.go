package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
	corerepo "github.com/Shamba-Records-Limited/microvault/pkg/repository"
	compliancesvc "github.com/Shamba-Records-Limited/microvault/pkg/services/compliance"
)

// vaultInfo is the two on-chain reads the screening queue's governance
// panel needs — narrow enough that a fake can stand in for it in tests
// without a real RPC client.
type vaultInfo interface {
	ComplianceRole(ctx context.Context) (string, error)
	AllowlistEnforced(ctx context.Context) (bool, error)
}

// screeningQueueStatuses is everything the queue shows: review, pending,
// and expired — the source design doc §14's full list. The rescreening
// sweep (pkg/services/compliance.RescreenSweep) is what makes "expired"
// reachable at all; before it existed nothing ever set that status, so
// this queue only listened for review/pending.
var screeningQueueStatuses = []string{"review", "pending", "expired"}

// Screening serves the queue (everything needing a compliance decision)
// and the per-address detail/audit screen.
type Screening struct {
	repo       corerepo.CounterpartyRepository
	compliance *compliancesvc.Service
	vault      vaultInfo
}

func NewScreening(repo corerepo.CounterpartyRepository, compliance *compliancesvc.Service, vault vaultInfo) *Screening {
	return &Screening{repo: repo, compliance: compliance, vault: vault}
}

func (h *Screening) List(c *fiber.Ctx) error {
	page := views.ScreeningQueuePage{
		Page:     queryInt(c, "page", 1),
		PageSize: defaultPageSize,
		Flash:    c.Query("flash"),
	}
	offset := (page.Page - 1) * page.PageSize

	// Best-effort: an RPC hiccup here shouldn't stop a compliance officer
	// from working the queue. VaultInfoError, not a false AllowlistEnforced,
	// is what a read failure produces — false reads as "confirmed off",
	// which a stale/unreadable RPC call must never claim.
	role, roleErr := h.vault.ComplianceRole(c.UserContext())
	enforced, enforcedErr := h.vault.AllowlistEnforced(c.UserContext())
	if roleErr != nil || enforcedErr != nil {
		page.VaultInfoError = "Could not read on-chain compliance state."
	} else {
		page.ComplianceRole = role
		page.AllowlistEnforced = enforced
	}

	addrs, err := h.repo.ListAddressesByStatus(c.UserContext(), screeningQueueStatuses, page.PageSize, offset)
	if err != nil {
		page.Error = "Could not load the screening queue: " + err.Error()
		return render(c, views.ScreeningQueue(page))
	}
	count, err := h.repo.CountAddressesByStatus(c.UserContext(), screeningQueueStatuses)
	if err != nil {
		page.Error = "Could not load the screening queue: " + err.Error()
		return render(c, views.ScreeningQueue(page))
	}

	page.Addresses = addrs
	page.TotalCount = count
	return render(c, views.ScreeningQueue(page))
}

// Approve and Reject deliberately never fold err.Error() (or id, an
// attacker-influenced route param) into the redirect URL — see
// handlers/counterparties.go's redisplayDetail doc comment on why. These
// two stay a plain redirect with a static flash rather than a full
// redisplay, since reconstructing the queue's paginated state from a
// POST-only request isn't worth it for a write that only fails if the row
// vanished concurrently.
func (h *Screening) Approve(c *fiber.Ctx) error {
	id := c.Params("id")
	actor := middleware.GetAdminClaims(c).AdminPublicKey
	if err := h.repo.ApproveAddress(c.UserContext(), id, actor, c.FormValue("reason")); err != nil {
		return c.Redirect("/screening?flash=Approval+failed", fiber.StatusSeeOther)
	}
	return c.Redirect("/screening?flash=Address+approved", fiber.StatusSeeOther)
}

func (h *Screening) Reject(c *fiber.Ctx) error {
	id := c.Params("id")
	actor := middleware.GetAdminClaims(c).AdminPublicKey
	if err := h.repo.RejectAddress(c.UserContext(), id, actor, c.FormValue("reason")); err != nil {
		return c.Redirect("/screening?flash=Rejection+failed", fiber.StatusSeeOther)
	}
	return c.Redirect("/screening?flash=Address+rejected", fiber.StatusSeeOther)
}

// Rescreen is the admin's first hx-post handler. A rescreen is a
// synchronous Elliptic call that can take seconds, so it returns just the
// updated row's HTML (hx-target/hx-swap on the client replaces only that
// <tr>) instead of forcing a full-page reload to block on it — see the
// source design doc §14.
func (h *Screening) Rescreen(c *fiber.Ctx) error {
	id := c.Params("id")
	// A failed rescreen still renders the row below rather than an error
	// page — the row itself is the only thing hx-swap is watching, and its
	// current (unchanged) state is a more honest partial than a broken one.
	_ = h.compliance.ScreenAndRecord(c.UserContext(), id)

	addr, err := h.repo.GetAddressByID(c.UserContext(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("address not found")
	}
	return render(c, views.ScreeningRow(addr))
}

func (h *Screening) Show(c *fiber.Ctx) error {
	id := c.Params("id")
	addr, err := h.repo.GetAddressByID(c.UserContext(), id)
	if err != nil {
		return render(c, views.ScreeningDetail(views.ScreeningDetailPage{Error: "Could not load address: " + err.Error()}))
	}

	screenings, err := h.repo.ListScreeningsByAddress(c.UserContext(), id)
	if err != nil {
		return render(c, views.ScreeningDetail(views.ScreeningDetailPage{
			Address: addr,
			Error:   "Could not load screening history: " + err.Error(),
		}))
	}

	return render(c, views.ScreeningDetail(views.ScreeningDetailPage{
		Address:    addr,
		Screenings: screenings,
		Flash:      c.Query("flash"),
	}))
}
