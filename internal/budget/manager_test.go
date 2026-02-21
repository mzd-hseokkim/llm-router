package budget_test

import (
	"testing"
	"time"

	"github.com/llm-router/gateway/internal/budget"
)

func TestPeriodStart(t *testing.T) {
	now := time.Date(2026, 2, 21, 15, 30, 0, 0, time.UTC) // Saturday

	tests := []struct {
		period string
		want   time.Time
	}{
		{"hourly", time.Date(2026, 2, 21, 15, 0, 0, 0, time.UTC)},
		{"daily", time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC)},
		{"weekly", time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)}, // Monday
		{"monthly", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			got, err := budget.PeriodStart(tt.period, now)
			if err != nil {
				t.Fatalf("PeriodStart(%q) error: %v", tt.period, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("PeriodStart(%q) = %v, want %v", tt.period, got, tt.want)
			}
		})
	}
}

func TestPeriodEnd(t *testing.T) {
	now := time.Date(2026, 2, 21, 15, 30, 0, 0, time.UTC)

	tests := []struct {
		period string
		want   time.Time
	}{
		{"hourly", time.Date(2026, 2, 21, 16, 0, 0, 0, time.UTC)},
		{"daily", time.Date(2026, 2, 22, 0, 0, 0, 0, time.UTC)},
		{"weekly", time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC)}, // next Monday
		{"monthly", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			got, err := budget.PeriodEnd(tt.period, now)
			if err != nil {
				t.Fatalf("PeriodEnd(%q) error: %v", tt.period, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("PeriodEnd(%q) = %v, want %v", tt.period, got, tt.want)
			}
		})
	}
}

func TestPeriodStartEnd_InvalidPeriod(t *testing.T) {
	now := time.Now()
	if _, err := budget.PeriodStart("quarterly", now); err == nil {
		t.Error("expected error for unknown period 'quarterly'")
	}
	if _, err := budget.PeriodEnd("quarterly", now); err == nil {
		t.Error("expected error for unknown period 'quarterly'")
	}
}

func TestIsExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	if !budget.IsExpired(past) {
		t.Error("past time should be expired")
	}
	if budget.IsExpired(future) {
		t.Error("future time should not be expired")
	}
	if budget.IsExpired(time.Time{}) {
		t.Error("zero time should not be expired (lifetime)")
	}
}

func TestErrBudgetExceeded(t *testing.T) {
	err := budget.ErrBudgetExceeded{Current: 105.50, Limit: 100.00, Period: "monthly"}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}
