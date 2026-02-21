// Package billing implements cost allocation (chargeback/showback) and markup pricing.
package billing

// MarkupConfig defines the pricing markup for a team (or global default when TeamID is nil).
type MarkupConfig struct {
	ID         string
	TeamID     *string
	Percentage float64 // e.g. 20 = 20% markup applied to base cost
	FixedUSD   float64 // flat per-request addition
	CapUSD     float64 // 0 = no cap
}

// Apply returns the total charged cost given a base LLM cost.
func (m *MarkupConfig) Apply(baseCost float64) float64 {
	markup := baseCost * m.Percentage / 100
	if m.CapUSD > 0 && markup > m.CapUSD {
		markup = m.CapUSD
	}
	return baseCost + markup + m.FixedUSD
}
