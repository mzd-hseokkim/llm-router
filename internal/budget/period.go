package budget

import (
	"fmt"
	"time"
)

// ValidPeriods lists the accepted budget period strings.
var ValidPeriods = []string{"hourly", "daily", "weekly", "monthly", "lifetime"}

// PeriodStart returns the start of the current period for now (UTC).
func PeriodStart(period string, now time.Time) (time.Time, error) {
	now = now.UTC()
	switch period {
	case "hourly":
		return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC), nil
	case "daily":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	case "weekly":
		// ISO week starts Monday.
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -(weekday - 1))
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC), nil
	case "monthly":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	case "lifetime":
		return time.Time{}, nil // zero = no period boundary
	default:
		return time.Time{}, fmt.Errorf("unknown period: %q", period)
	}
}

// PeriodEnd returns the exclusive end of the current period for now (UTC).
// Returns the far future for "lifetime".
func PeriodEnd(period string, now time.Time) (time.Time, error) {
	now = now.UTC()
	switch period {
	case "hourly":
		return time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, time.UTC), nil
	case "daily":
		return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC), nil
	case "weekly":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -(weekday - 1))
		nextMonday := monday.AddDate(0, 0, 7)
		return time.Date(nextMonday.Year(), nextMonday.Month(), nextMonday.Day(), 0, 0, 0, 0, time.UTC), nil
	case "monthly":
		return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC), nil
	case "lifetime":
		return time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), nil
	default:
		return time.Time{}, fmt.Errorf("unknown period: %q", period)
	}
}

// IsExpired reports whether the period_end has passed.
func IsExpired(periodEnd time.Time) bool {
	if periodEnd.IsZero() {
		return false
	}
	return time.Now().UTC().After(periodEnd)
}
