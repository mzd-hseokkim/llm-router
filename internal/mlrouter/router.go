package mlrouter

import (
	"context"
	"log/slog"

	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/gateway/fallback"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/health"
)

// Mode controls how the ML router interacts with the normal routing path.
const (
	ModeShadow = "shadow" // recommend but use normal routing; log comparison
	ModeLive   = "live"   // replace normal routing with ML recommendation
)

// Config holds ML routing configuration.
type Config struct {
	Mode         string
	Weights      Weights
	QualityTiers map[string][]modelEntry // tier → ordered provider+model list
}

// Router implements the advancedResolver interface for the ChatHandler.
// In live mode it returns the ML-selected chain.
// In shadow mode it returns (chain, false) so normal routing is used,
// but logs the ML recommendation for comparison.
type Router struct {
	classifier *Classifier
	scorer     *Scorer
	feedback   *FeedbackCollector
	mode       string
	logger     *slog.Logger
}

// NewRouter builds an ML Router from config.
func NewRouter(
	cfg Config,
	pricing *cost.Calculator,
	tracker *health.ProviderTracker,
	logger *slog.Logger,
) *Router {
	scorer := NewScorer(cfg.Weights, cfg.QualityTiers, pricing, tracker)
	return &Router{
		classifier: NewClassifier(),
		scorer:     scorer,
		feedback:   NewFeedbackCollector(logger),
		mode:       cfg.Mode,
		logger:     logger,
	}
}

// Resolve classifies the request and selects the best provider chain.
// Returns (chain, true) in live mode, (chain, false) in shadow mode.
// Implements the advancedResolver interface required by ChatHandler.
func (r *Router) Resolve(ctx context.Context, req *types.ChatCompletionRequest) (fallback.Chain, bool) {
	features := r.classifier.Classify(req)

	// Estimate output tokens as ~50% of input (conservative default).
	outputEst := features.TokenEstimate / 2
	chain := r.scorer.RankTier(features.Tier, features.TokenEstimate, outputEst)

	if r.mode == ModeLive {
		r.logger.Debug("ml_routing_live",
			"tier", features.Tier,
			"complexity", features.ComplexityScore,
			"selected_chain", chain.Name,
			"targets", len(chain.Targets),
		)
		return chain, len(chain.Targets) > 0
	}

	// Shadow mode: log recommendation but signal ChatHandler to use normal routing.
	if len(chain.Targets) > 0 {
		r.logger.Info("ml_routing_shadow",
			"tier", features.Tier,
			"complexity_score", features.ComplexityScore,
			"ml_recommendation", chain.Targets[0].Provider+"/"+chain.Targets[0].Model,
			"note", "actual routing uses normal chain",
		)
	}
	return fallback.Chain{}, false
}

// Close shuts down the feedback collector.
func (r *Router) Close() {
	r.feedback.Close()
}

// BuildTiers converts config slices into the internal modelEntry map.
// qualityOrder maps tier name to quality score:
//
//	economy=0.3, medium=0.65, premium=1.0
func BuildTiers(cfgTiers []TierConfig) map[string][]modelEntry {
	tierScores := map[string]float64{
		ComplexitySimple:  0.30,
		ComplexityMedium:  0.65,
		ComplexityComplex: 1.00,
	}
	result := make(map[string][]modelEntry, len(cfgTiers))
	for _, t := range cfgTiers {
		score := tierScores[t.Name]
		if score == 0 {
			score = 0.5 // unknown tier = medium quality
		}
		entries := make([]modelEntry, len(t.Models))
		for i, m := range t.Models {
			entries[i] = modelEntry{
				Provider:     m.Provider,
				Model:        m.Model,
				QualityScore: score,
			}
		}
		result[t.Name] = entries
	}
	return result
}

// TierConfig is a plain-data struct used by main.go to avoid importing config.
type TierConfig struct {
	Name   string
	Models []ModelConfig
}

// ModelConfig is a provider+model pair for tier configuration.
type ModelConfig struct {
	Provider string
	Model    string
}
