package mlrouter

import (
	"log/slog"
	"time"
)

// FeedbackRecord captures the outcome of a routed request for model improvement.
type FeedbackRecord struct {
	RequestID    string
	Provider     string
	Model        string
	Tier         string
	ComplexScore float64
	LatencyMS    int64
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	Success      bool
	Timestamp    time.Time
}

// FeedbackCollector receives request outcomes and logs them asynchronously.
// In a production system the channel would drain into a ML training pipeline.
type FeedbackCollector struct {
	ch     chan FeedbackRecord
	logger *slog.Logger
}

// NewFeedbackCollector creates a FeedbackCollector with a buffered channel.
func NewFeedbackCollector(logger *slog.Logger) *FeedbackCollector {
	fc := &FeedbackCollector{
		ch:     make(chan FeedbackRecord, 1000),
		logger: logger,
	}
	go fc.drain()
	return fc
}

// Record enqueues a feedback record. Non-blocking: drops records when the
// buffer is full to avoid slowing down the hot path.
func (fc *FeedbackCollector) Record(rec FeedbackRecord) {
	select {
	case fc.ch <- rec:
	default:
		// Buffer full; drop rather than block.
	}
}

// drain processes records from the channel and logs structured data
// that could be consumed by a future ML training pipeline.
func (fc *FeedbackCollector) drain() {
	for rec := range fc.ch {
		fc.logger.Debug("ml_routing_feedback",
			"request_id", rec.RequestID,
			"provider", rec.Provider,
			"model", rec.Model,
			"tier", rec.Tier,
			"complexity_score", rec.ComplexScore,
			"latency_ms", rec.LatencyMS,
			"input_tokens", rec.InputTokens,
			"output_tokens", rec.OutputTokens,
			"cost_usd", rec.CostUSD,
			"success", rec.Success,
			"ts", rec.Timestamp,
		)
	}
}

// Close drains remaining records and shuts down the collector.
func (fc *FeedbackCollector) Close() {
	close(fc.ch)
}
