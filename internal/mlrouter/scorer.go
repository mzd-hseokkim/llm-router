package mlrouter

import (
	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/health"
)

// Weights defines the relative importance of each scoring dimension.
// Values should sum to approximately 1.0 for interpretable scores.
type Weights struct {
	Cost        float64 // lower cost = higher score
	Quality     float64 // higher quality tier = higher score
	Latency     float64 // lower error rate proxy for latency
	Reliability float64 // inversely proportional to error rate
}

// DefaultWeights returns the balanced default weights.
func DefaultWeights() Weights {
	return Weights{Cost: 0.3, Quality: 0.4, Latency: 0.2, Reliability: 0.1}
}

// modelEntry holds a provider+model pair in a quality tier.
type modelEntry struct {
	Provider     string
	Model        string
	QualityScore float64 // 0.0–1.0 based on tier position
}

// Scorer ranks provider+model combinations using multi-criteria scoring.
type Scorer struct {
	weights  Weights
	tiers    map[string][]modelEntry // tier name → ordered models
	pricing  *cost.Calculator
	tracker  *health.ProviderTracker
}

// NewScorer creates a Scorer.
func NewScorer(
	weights Weights,
	tiers map[string][]modelEntry,
	pricing *cost.Calculator,
	tracker *health.ProviderTracker,
) *Scorer {
	return &Scorer{
		weights: weights,
		tiers:   tiers,
		pricing: pricing,
		tracker: tracker,
	}
}

// Score computes the composite score for a provider+model pair.
// Higher is better (range ~[0, 1]).
func (s *Scorer) Score(entry modelEntry, estimatedInputTokens, estimatedOutputTokens int) float64 {
	// Cost score: 1 - normalised cost (cheaper = higher score).
	costUSD := s.pricing.Calculate(entry.Model, estimatedInputTokens, estimatedOutputTokens)
	costScore := 1.0 - clamp(costUSD/0.10) // normalise against $0.10 ceiling

	// Reliability score: 1 - error rate.
	reliability := 1.0
	if s.tracker != nil {
		reliability = 1.0 - s.tracker.ErrorRate(entry.Provider)
	}

	// Latency score: use reliability as a proxy (high error rate → slow/unavailable).
	latency := reliability

	total := costScore*s.weights.Cost +
		entry.QualityScore*s.weights.Quality +
		latency*s.weights.Latency +
		reliability*s.weights.Reliability

	return clamp(total)
}

// RankTier returns the models in tier sorted by descending score, as a
// fallback.Chain ready for use by the ChatHandler.
func (s *Scorer) RankTier(tier string, inputTokens, outputTokens int) fallback.Chain {
	entries, ok := s.tiers[tier]
	if !ok || len(entries) == 0 {
		// Fallback: try medium tier, then economy.
		for _, fallbackTier := range []string{ComplexityMedium, ComplexitySimple} {
			if e, ok2 := s.tiers[fallbackTier]; ok2 && len(e) > 0 {
				entries = e
				break
			}
		}
	}

	type scored struct {
		entry modelEntry
		score float64
	}
	results := make([]scored, len(entries))
	for i, e := range entries {
		results[i] = scored{e, s.Score(e, inputTokens, outputTokens)}
	}

	// Sort descending by score (simple insertion sort; small N).
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].score > results[j-1].score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	targets := make([]fallback.Target, len(results))
	for i, r := range results {
		targets[i] = fallback.Target{Provider: r.entry.Provider, Model: r.entry.Model}
	}
	return fallback.Chain{Name: "ml-" + tier, Targets: targets}
}
