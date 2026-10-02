// Package metrics runs the dashboard's aggregate queries.
//
// These scan the full loans, users, and transactions tables on every dashboard
// load. That is acceptable at current volume and will need caching or a rollup
// table before it is not.
package metrics

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/Shamba-Records-Limited/microvault-credit/internal/credit/app/models"
)

// AssetTotal is a summed amount in one asset's minor units.
type AssetTotal struct {
	Asset  string
	Amount int64
}

// Snapshot is everything the dashboard renders in one pass.
type Snapshot struct {
	Outstanding     []AssetTotal
	ActiveLoans     int64
	DisbursedRecent []AssetTotal
	AverageLoan     int64
	DefaultRate     float64
	Alerts          []Alert
}

// Alert is one operational-health count. Zero counts are still rendered.
type Alert struct {
	Label string
	Count int64
	Note  string
}

// Service reads dashboard aggregates.
type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Snapshot collects every dashboard figure. A failure on any single query is
// returned rather than partially rendered, so the dashboard never shows a
// silently wrong number.
func (s *Service) Snapshot(ctx context.Context, window time.Duration) (*Snapshot, error) {
	snap := &Snapshot{}
	db := s.db.WithContext(ctx)

	if err := db.Model(&models.Loan{}).
		Select("principal_asset AS asset, COALESCE(SUM(principal_amount), 0)::bigint AS amount").
		Where("status = ?", models.LoanStatusDisbursed).
		Group("principal_asset").
		Scan(&snap.Outstanding).Error; err != nil {
		return nil, err
	}

	if err := db.Model(&models.Loan{}).
		Where("status = ?", models.LoanStatusDisbursed).
		Count(&snap.ActiveLoans).Error; err != nil {
		return nil, err
	}

	if err := db.Model(&models.Loan{}).
		Select("principal_asset AS asset, COALESCE(SUM(principal_amount), 0)::bigint AS amount").
		Where("disbursed_at > ?", time.Now().Add(-window)).
		Group("principal_asset").
		Scan(&snap.DisbursedRecent).Error; err != nil {
		return nil, err
	}

	if err := db.Model(&models.Loan{}).
		Select("COALESCE(AVG(principal_amount), 0)::bigint").
		Where("disbursed_at IS NOT NULL").
		Scan(&snap.AverageLoan).Error; err != nil {
		return nil, err
	}

	var settled, defaulted int64
	if err := db.Model(&models.Loan{}).
		Where("status IN ?", []string{models.LoanStatusRepaid, models.LoanStatusDefaulted}).
		Count(&settled).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&models.Loan{}).
		Where("status = ?", models.LoanStatusDefaulted).
		Count(&defaulted).Error; err != nil {
		return nil, err
	}
	if settled > 0 {
		snap.DefaultRate = float64(defaulted) / float64(settled) * 100
	}

	alerts, err := s.alerts(ctx)
	if err != nil {
		return nil, err
	}
	snap.Alerts = alerts

	return snap, nil
}

func (s *Service) alerts(ctx context.Context) ([]Alert, error) {
	db := s.db.WithContext(ctx)

	specs := []struct {
		label string
		note  string
		apply func(*gorm.DB) *gorm.DB
	}{
		{
			label: "Stuck mid-disbursement",
			note:  "Vault borrow succeeded, settlement unfinished",
			apply: func(q *gorm.DB) *gorm.DB {
				return q.Model(&models.Loan{}).Where("status = ?", models.LoanStatusDisbursing)
			},
		},
		{
			label: "Off-ramp failed",
			note:  "Borrower never received funds; not a default",
			apply: func(q *gorm.DB) *gorm.DB {
				return q.Model(&models.Loan{}).Where("status = ?", models.LoanStatusOffRampFailed)
			},
		},
		{
			label: "Vault repayment failed",
			note:  "USDC held in treasury; the reconciler is retrying",
			apply: func(q *gorm.DB) *gorm.DB {
				return q.Model(&models.Loan{}).
					Where("vault_repay_status = ? AND vault_repay_attempted_at IS NOT NULL", models.VaultRepayStatusFailed)
			},
		},
		{
			label: "Vault repay unconfirmed",
			note:  "Submitted; the reconciler settles it from the ledger once it expires",
			apply: func(q *gorm.DB) *gorm.DB {
				return q.Model(&models.Loan{}).
					Where("vault_repay_status IN ? AND vault_repay_pending_tx_hash IS NOT NULL",
						[]string{models.VaultRepayStatusPending, models.VaultRepayStatusUnknown})
			},
		},
		{
			label: "Vault repay needs an operator",
			note:  "Outcome unknown with no recorded transaction, or failed before retries were tracked",
			apply: func(q *gorm.DB) *gorm.DB {
				return q.Model(&models.Loan{}).Where(
					"(vault_repay_status = ? AND vault_repay_pending_tx_hash IS NULL) OR "+
						"(vault_repay_status = ? AND vault_repay_attempted_at IS NULL)",
					models.VaultRepayStatusUnknown, models.VaultRepayStatusFailed)
			},
		},
		{
			label: "Refund shortfall",
			note:  "Anchor returned less than was sent",
			apply: func(q *gorm.DB) *gorm.DB {
				return q.Model(&models.Loan{}).Where("ramp_refund_shortfall > 0")
			},
		},
	}

	alerts := make([]Alert, 0, len(specs))
	for _, spec := range specs {
		var count int64
		if err := spec.apply(db).Count(&count).Error; err != nil {
			return nil, err
		}
		alerts = append(alerts, Alert{Label: spec.label, Count: count, Note: spec.note})
	}

	return alerts, nil
}
