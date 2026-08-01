package views

import (
	"fmt"
	"strings"
	"time"
)

// Money renders fiat cents as a grouped decimal amount with its currency.
func Money(cents int64, currency string) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole := fmt.Sprintf("%d", cents/100)

	var grouped strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(r)
	}

	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%s %s.%02d", sign, currency, grouped.String(), cents%100)
}

// Bps renders basis points as a percentage.
func Bps(bps int32) string {
	return fmt.Sprintf("%.2f%%", float64(bps)/100)
}

// BpsPtr renders an optional basis-point value, falling back to an em dash.
func BpsPtr(bps *int32) string {
	if bps == nil {
		return "—"
	}
	return Bps(*bps)
}

// Days renders a day range, collapsing equal bounds.
func Days(minDays, maxDays int) string {
	if minDays == maxDays {
		return fmt.Sprintf("%d days", minDays)
	}
	return fmt.Sprintf("%d–%d days", minDays, maxDays)
}

// Date renders a timestamp in the admin's fixed display format.
func Date(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}

// Schedules renders a repayment-schedule list for display.
func Schedules(s []string) string {
	if len(s) == 0 {
		return "—"
	}
	return strings.Join(s, ", ")
}
