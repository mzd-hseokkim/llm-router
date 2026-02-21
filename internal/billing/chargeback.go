package billing

import (
	"context"
	"time"
)

// Store is the data-access interface needed by the billing service.
type Store interface {
	GetMarkupConfig(ctx context.Context, teamID *string) (*MarkupConfig, error)
	GetTeamUsage(ctx context.Context, from, to time.Time) ([]*TeamUsage, error)
	GetModelBreakdown(ctx context.Context, teamID string, from, to time.Time) ([]*ModelBreakdown, error)
	GetTagBreakdown(ctx context.Context, teamID string, from, to time.Time) ([]*TagBreakdown, error)
}

// TeamUsage holds aggregated usage figures for a single team over a period.
type TeamUsage struct {
	TeamID   string
	TeamName string
	CostUSD  float64
	Tokens   int64
	Requests int64
}

// ModelBreakdown holds per-model cost figures for a team.
type ModelBreakdown struct {
	Model   string
	CostUSD float64
	Tokens  int64
}

// TagBreakdown holds per-tag cost figures derived from request metadata.
type TagBreakdown struct {
	Tag     string
	CostUSD float64
}

// ChargebackService computes chargeback and showback reports.
type ChargebackService struct {
	store Store
}

// NewChargebackService returns a ChargebackService.
func NewChargebackService(store Store) *ChargebackService {
	return &ChargebackService{store: store}
}

// BuildReport generates the full chargeback report for the given calendar period.
func (s *ChargebackService) BuildReport(ctx context.Context, from, to time.Time) (*ChargebackReport, error) {
	teams, err := s.store.GetTeamUsage(ctx, from, to)
	if err != nil {
		return nil, err
	}

	report := &ChargebackReport{
		Period:      from.Format("2006-01"),
		GeneratedAt: time.Now().UTC(),
		Currency:    "USD",
	}

	for _, tu := range teams {
		// Apply team-specific (or global) markup.
		markup, err := s.store.GetMarkupConfig(ctx, &tu.TeamID)
		if err != nil {
			// Fall back to global if team-specific config is missing.
			markup, _ = s.store.GetMarkupConfig(ctx, nil)
		}

		markupUSD := 0.0
		chargedUSD := tu.CostUSD
		if markup != nil {
			chargedUSD = markup.Apply(tu.CostUSD)
			markupUSD = chargedUSD - tu.CostUSD
		}

		models, _ := s.store.GetModelBreakdown(ctx, tu.TeamID, from, to)
		tags, _ := s.store.GetTagBreakdown(ctx, tu.TeamID, from, to)

		report.Summary.TotalCostUSD += tu.CostUSD
		report.Summary.TotalTokens += tu.Tokens
		report.Summary.TotalRequests += tu.Requests

		report.ByTeam = append(report.ByTeam, &TeamChargebackEntry{
			TeamID:           tu.TeamID,
			TeamName:         tu.TeamName,
			CostUSD:          tu.CostUSD,
			MarkupUSD:        markupUSD,
			TotalChargedUSD:  chargedUSD,
			Tokens:           tu.Tokens,
			Requests:         tu.Requests,
			ByModel:          models,
			ByProject:        tags,
		})
	}

	return report, nil
}

// ShowbackReport returns usage data for a specific team (no billing amounts).
func (s *ChargebackService) ShowbackReport(ctx context.Context, teamID string, from, to time.Time) (*ShowbackReport, error) {
	teams, err := s.store.GetTeamUsage(ctx, from, to)
	if err != nil {
		return nil, err
	}

	var tu *TeamUsage
	for _, t := range teams {
		if t.TeamID == teamID {
			tu = t
			break
		}
	}
	if tu == nil {
		tu = &TeamUsage{TeamID: teamID}
	}

	models, _ := s.store.GetModelBreakdown(ctx, teamID, from, to)
	tags, _ := s.store.GetTagBreakdown(ctx, teamID, from, to)

	return &ShowbackReport{
		Period:      from.Format("2006-01"),
		GeneratedAt: time.Now().UTC(),
		TeamID:      teamID,
		TeamName:    tu.TeamName,
		CostUSD:     tu.CostUSD,
		Tokens:      tu.Tokens,
		Requests:    tu.Requests,
		ByModel:     models,
		ByProject:   tags,
	}, nil
}
