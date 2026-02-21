// Package abtest implements A/B test routing: consistent variant assignment,
// result collection, and statistical analysis.
package abtest

import "time"

// Status represents the lifecycle state of an experiment.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusStopped   Status = "stopped"
)

// TrafficSplit defines one variant's share of traffic.
type TrafficSplit struct {
	Variant string `json:"variant"`
	Model   string `json:"model"`
	Weight  int    `json:"weight"` // 0-100; all weights must sum to 100
}

// Target restricts which entities participate in the experiment.
type Target struct {
	TeamIDs    []string `json:"team_ids,omitempty"` // nil/empty = all teams
	SampleRate float64  `json:"sample_rate"`        // 0.0–1.0; 1.0 = 100%
}

// Experiment is an A/B test configuration.
type Experiment struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Status          Status         `json:"status"`
	TrafficSplit    []TrafficSplit `json:"traffic_split"`
	Target          Target         `json:"target"`
	SuccessMetrics  []string       `json:"success_metrics"`
	MinSamples      int            `json:"min_samples"`
	ConfidenceLevel float64        `json:"confidence_level"`
	StartAt         *time.Time     `json:"start_at,omitempty"`
	EndAt           *time.Time     `json:"end_at,omitempty"`
	Winner          string         `json:"winner,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// IsActive returns true when the experiment is running and within its time window.
func (e *Experiment) IsActive() bool {
	if e.Status != StatusRunning {
		return false
	}
	now := time.Now()
	if e.StartAt != nil && now.Before(*e.StartAt) {
		return false
	}
	if e.EndAt != nil && now.After(*e.EndAt) {
		return false
	}
	return true
}

// ModelForVariant returns the model string configured for a variant.
func (e *Experiment) ModelForVariant(variant string) string {
	for _, s := range e.TrafficSplit {
		if s.Variant == variant {
			return s.Model
		}
	}
	return ""
}

// Result holds per-request metrics for one A/B test observation.
type Result struct {
	TestID           string
	Variant          string
	RequestID        string
	Timestamp        time.Time
	Model            string
	LatencyMs        int
	PromptTokens     int
	CompletionTokens int
	CostUSD          float64
	Error            bool
	FinishReason     string
}

// VariantStats is aggregated per-variant metrics used by the analyzer.
type VariantStats struct {
	Variant          string
	Samples          int
	LatencyP95Ms     float64
	AvgLatencyMs     float64
	AvgCostPerReq    float64
	ErrorRate        float64
	ErrorCount       int
	LatencyValues    []float64
}
