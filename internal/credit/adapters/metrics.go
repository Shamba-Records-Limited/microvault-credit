package adapters

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/Shamba-Records-Limited/microvault/pkg/telemetry"
)

// Repayment rails, the bounded rail label on microvault.loan.repayments.
const (
	railMoneyGram    = "moneygram"
	railMpesaSTK     = "mpesa_stk"
	railMpesaPaybill = "mpesa_paybill"
	railAirtel       = "airtel"
)

var (
	disbursements, _ = telemetry.Meter().Int64Counter("microvault.loan.disbursements",
		metric.WithUnit("{loan}"),
		metric.WithDescription("Loans reaching a terminal disbursement status, by off-ramp provider and outcome (completed or failed)."))

	repayments, _ = telemetry.Meter().Int64Counter("microvault.loan.repayments",
		metric.WithUnit("{repayment}"),
		metric.WithDescription("Repayments reaching a milestone, by rail and outcome (funds_received, settled, expired or failed)."))
)

func recordDisbursement(ctx context.Context, provider, outcome string) {
	if provider == "" {
		provider = "unknown"
	}
	disbursements.Add(ctx, 1, metric.WithAttributes(attribute.String("provider", provider), attribute.String("outcome", outcome)))
}

func recordRepayment(ctx context.Context, rail, outcome string) {
	repayments.Add(ctx, 1, metric.WithAttributes(attribute.String("rail", rail), attribute.String("outcome", outcome)))
}
