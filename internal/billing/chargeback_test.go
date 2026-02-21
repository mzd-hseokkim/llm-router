package billing

import (
	"context"
	"testing"
	"time"
)

// mockStore satisfies the Store interface for testing.
type mockStore struct {
	markups map[string]*MarkupConfig
	teams   []*TeamUsage
}

func (m *mockStore) GetMarkupConfig(_ context.Context, teamID *string) (*MarkupConfig, error) {
	key := ""
	if teamID != nil {
		key = *teamID
	}
	if cfg, ok := m.markups[key]; ok {
		return cfg, nil
	}
	// Return the global default.
	if cfg, ok := m.markups[""]; ok {
		return cfg, nil
	}
	return &MarkupConfig{}, nil
}

func (m *mockStore) GetTeamUsage(_ context.Context, _, _ time.Time) ([]*TeamUsage, error) {
	return m.teams, nil
}

func (m *mockStore) GetModelBreakdown(_ context.Context, _ string, _, _ time.Time) ([]*ModelBreakdown, error) {
	return nil, nil
}

func (m *mockStore) GetTagBreakdown(_ context.Context, _ string, _, _ time.Time) ([]*TagBreakdown, error) {
	return nil, nil
}

func TestMarkupApply(t *testing.T) {
	cases := []struct {
		cfg       MarkupConfig
		base      float64
		wantTotal float64
	}{
		{MarkupConfig{Percentage: 20}, 100, 120},
		{MarkupConfig{Percentage: 20, CapUSD: 10}, 100, 110},  // cap applies
		{MarkupConfig{FixedUSD: 5}, 100, 105},
		{MarkupConfig{Percentage: 50, FixedUSD: 2}, 100, 152},
	}
	for _, tc := range cases {
		got := tc.cfg.Apply(tc.base)
		if got != tc.wantTotal {
			t.Errorf("Apply(%v, base=%.2f) = %.2f, want %.2f", tc.cfg, tc.base, got, tc.wantTotal)
		}
	}
}

func TestBuildReport(t *testing.T) {
	teamID := "team-abc"
	store := &mockStore{
		markups: map[string]*MarkupConfig{
			teamID: {Percentage: 10},
			"":     {Percentage: 0},
		},
		teams: []*TeamUsage{
			{TeamID: teamID, TeamName: "ML Team", CostUSD: 100, Tokens: 1000, Requests: 5},
		},
	}
	svc := NewChargebackService(store)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	report, err := svc.BuildReport(context.Background(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ByTeam) != 1 {
		t.Fatalf("expected 1 team entry, got %d", len(report.ByTeam))
	}
	entry := report.ByTeam[0]
	if entry.MarkupUSD != 10 {
		t.Errorf("markup = %.2f, want 10", entry.MarkupUSD)
	}
	if entry.TotalChargedUSD != 110 {
		t.Errorf("total_charged = %.2f, want 110", entry.TotalChargedUSD)
	}
}

func TestReportToCSV(t *testing.T) {
	r := &ChargebackReport{
		Period:   "2026-01",
		Currency: "USD",
		ByTeam: []*TeamChargebackEntry{
			{TeamID: "t1", TeamName: "Team A", CostUSD: 50, TotalChargedUSD: 55, Tokens: 1000, Requests: 10},
		},
	}
	csv, err := r.ToCSV()
	if err != nil {
		t.Fatal(err)
	}
	if len(csv) == 0 {
		t.Fatal("empty CSV output")
	}
}
