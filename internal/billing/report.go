package billing

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"
)

// ChargebackReport is the full monthly chargeback document.
type ChargebackReport struct {
	Period      string                  `json:"period"`       // "2026-01"
	GeneratedAt time.Time               `json:"generated_at"`
	Currency    string                  `json:"currency"`
	Summary     ChargebackSummary       `json:"summary"`
	ByTeam      []*TeamChargebackEntry  `json:"by_team"`
}

// ChargebackSummary aggregates totals across all teams.
type ChargebackSummary struct {
	TotalCostUSD  float64 `json:"total_cost_usd"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalRequests int64   `json:"total_requests"`
}

// TeamChargebackEntry holds all chargeback data for one team.
type TeamChargebackEntry struct {
	TeamID          string            `json:"team_id"`
	TeamName        string            `json:"team_name"`
	CostUSD         float64           `json:"cost_usd"`
	MarkupUSD       float64           `json:"markup_usd"`
	TotalChargedUSD float64           `json:"total_charged_usd"`
	Tokens          int64             `json:"tokens"`
	Requests        int64             `json:"requests"`
	ByModel         []*ModelBreakdown `json:"by_model,omitempty"`
	ByProject       []*TagBreakdown   `json:"by_project,omitempty"`
}

// ShowbackReport is an informational cost view for a single team (no billing).
type ShowbackReport struct {
	Period      string            `json:"period"`
	GeneratedAt time.Time         `json:"generated_at"`
	TeamID      string            `json:"team_id"`
	TeamName    string            `json:"team_name"`
	CostUSD     float64           `json:"cost_usd"`
	Tokens      int64             `json:"tokens"`
	Requests    int64             `json:"requests"`
	ByModel     []*ModelBreakdown `json:"by_model,omitempty"`
	ByProject   []*TagBreakdown   `json:"by_project,omitempty"`
}

// BillingUsageItem is the external billing API line item.
type BillingUsageItem struct {
	TeamID      string            `json:"team_id"`
	PeriodStart string            `json:"period_start"`
	PeriodEnd   string            `json:"period_end"`
	Tokens      int64             `json:"quantity_tokens"`
	UnitPriceUSD float64          `json:"unit_price_usd"`
	AmountUSD   float64           `json:"amount_usd"`
	Metadata    map[string]string `json:"metadata"`
}

// ToJSON serialises the chargeback report as JSON bytes.
func (r *ChargebackReport) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// ToCSV serialises the chargeback report as CSV bytes.
// One row per team with summary columns.
func (r *ChargebackReport) ToCSV() ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	header := []string{
		"period", "team_id", "team_name",
		"cost_usd", "markup_usd", "total_charged_usd",
		"tokens", "requests",
	}
	if err := w.Write(header); err != nil {
		return nil, err
	}

	for _, e := range r.ByTeam {
		row := []string{
			r.Period,
			e.TeamID,
			e.TeamName,
			fmt.Sprintf("%.8f", e.CostUSD),
			fmt.Sprintf("%.8f", e.MarkupUSD),
			fmt.Sprintf("%.8f", e.TotalChargedUSD),
			fmt.Sprintf("%d", e.Tokens),
			fmt.Sprintf("%d", e.Requests),
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
