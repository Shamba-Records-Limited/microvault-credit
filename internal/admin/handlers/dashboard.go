package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/metrics"
	"github.com/Shamba-Records-Limited/microvault-credit/internal/admin/views"
	"github.com/Shamba-Records-Limited/microvault/pkg/middleware"
)

const dashboardWindow = 30 * 24 * time.Hour

// Dashboard renders the KPI row and the operational-health panel.
type Dashboard struct {
	metrics *metrics.Service
}

func NewDashboard(m *metrics.Service) *Dashboard {
	return &Dashboard{metrics: m}
}

func (h *Dashboard) Show(c *fiber.Ctx) error {
	page := views.DashboardPage{WindowDays: int(dashboardWindow.Hours() / 24)}

	snapshot, err := h.metrics.Snapshot(c.UserContext(), dashboardWindow)
	if err != nil {
		middleware.NoteError(c, err)
		page.Error = "Could not load metrics: " + err.Error()
	} else {
		page.Snapshot = snapshot
	}

	return render(c, views.Dashboard(page))
}
