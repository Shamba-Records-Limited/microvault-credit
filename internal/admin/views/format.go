package views

import (
	"fmt"
	"strings"
	"time"
)

// assetDivisor is the number of minor units per whole unit. Stellar assets
// are stored in stroops (10^7); fiat is stored in cents.
func assetDivisor(asset string) int64 {
	switch strings.ToUpper(asset) {
	case "USDC", "XLM":
		return 10_000_000
	default:
		return 100
	}
}

// Money renders a minor-unit amount as a grouped decimal with its asset code.
func Money(amount int64, asset string) string {
	divisor := assetDivisor(asset)
	neg := amount < 0
	if neg {
		amount = -amount
	}
	whole := fmt.Sprintf("%d", amount/divisor)

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
	return fmt.Sprintf("%s%s %s.%02d", sign, asset, grouped.String(), amount%divisor/(divisor/100))
}

// Minor renders a minor-unit amount with digit grouping but no asset code, for
// configuration values whose denomination is not stored alongside them.
func Minor(amount int64) string {
	return group(fmt.Sprintf("%d", amount))
}

// Count renders an integer with digit grouping.
func Count(n int) string {
	return group(fmt.Sprintf("%d", n))
}

// StringPtr renders an optional string, falling back to an em dash.
func StringPtr(s *string) string {
	if s == nil || *s == "" {
		return "—"
	}
	return *s
}

func group(digits string) string {
	neg := strings.HasPrefix(digits, "-")
	digits = strings.TrimPrefix(digits, "-")

	var out strings.Builder
	if neg {
		out.WriteByte('-')
	}
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(r)
	}
	return out.String()
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

// DatePtr renders an optional timestamp, falling back to an em dash.
func DatePtr(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return Date(*t)
}

// queryString builds one pager query parameter, empty when the filter is unset.
func queryString(key, value string) string {
	if value == "" {
		return ""
	}
	return key + "=" + value
}

// Schedules renders a repayment-schedule list for display.
func Schedules(s []string) string {
	if len(s) == 0 {
		return "—"
	}
	return strings.Join(s, ", ")
}
