package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/llm-router/gateway/internal/telemetry"
)

// RequestRecorder receives a request result for provider health tracking.
// Implemented by health.ProviderTracker.
type RequestRecorder interface {
	Record(providerName string, success bool)
}

// RequestLogger returns a middleware that records a LogEntry for every LLM API request
// and updates Prometheus metrics. Non-API paths (e.g. /ping) are skipped because
// handlers never call telemetry.SetModel.
func RequestLogger(w *telemetry.LogWriter, logger *slog.Logger, recorder RequestRecorder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ctx, lc := telemetry.NewRequestLogContext(r.Context())
			r = r.WithContext(ctx)

			sw := &statusWriter{ResponseWriter: rw}

			next.ServeHTTP(sw, r)

			if lc.Model == "" {
				return
			}

			latencyMs := time.Since(start).Milliseconds()
			statusCode := sw.status()

			entry := &telemetry.LogEntry{
				RequestID:        chimiddleware.GetReqID(r.Context()),
				Timestamp:        start,
				Model:            lc.Model,
				Provider:         lc.Provider,
				VirtualKeyID:     lc.VirtualKeyID,
				UserID:           lc.UserID,
				TeamID:           lc.TeamID,
				OrgID:            lc.OrgID,
				PromptTokens:     lc.PromptTokens,
				CompletionTokens: lc.CompletionTokens,
				TotalTokens:      lc.TotalTokens,
				CostUSD:          lc.CostUSD,
				FinishReason:     lc.FinishReason,
				ErrorCode:        lc.ErrorCode,
				ErrorMessage:     lc.ErrorMessage,
				IsStreaming:      lc.IsStreaming,
				StatusCode:       statusCode,
				LatencyMs:        latencyMs,
			}

			if !lc.TTFTAt.IsZero() {
				ttft := lc.TTFTAt.Sub(start).Milliseconds()
				entry.TTFTMs = &ttft
			}

			logger.Info("request_completed",
				"request_id", entry.RequestID,
				"model", entry.Model,
				"provider", entry.Provider,
				"latency_ms", entry.LatencyMs,
				"tokens", entry.TotalTokens,
				"status", entry.StatusCode,
			)

			w.Write(entry)

			// Update Prometheus metrics.
			if lc.Provider != "" {
				statusLabel := fmt.Sprintf("%d", statusCode)
				telemetry.RequestsTotal.WithLabelValues(lc.Provider, lc.Model, statusLabel).Inc()
				telemetry.RequestDurationSeconds.WithLabelValues(lc.Provider, lc.Model).
					Observe(float64(latencyMs) / 1000.0)
			}

			// Update provider health tracker.
			if recorder != nil && lc.Provider != "" {
				recorder.Record(lc.Provider, statusCode < 500)
			}
		})
	}
}

// statusWriter wraps http.ResponseWriter to capture the HTTP status code.
// It forwards Flush and Unwrap so that SSE streaming and
// http.ResponseController (for write deadlines) continue to work.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	if sw.code == 0 {
		sw.code = code
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if sw.code == 0 {
		sw.code = http.StatusOK
	}
	return sw.ResponseWriter.Write(b)
}

// Flush implements http.Flusher so SSE streaming works through this wrapper.
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying ResponseWriter so http.ResponseController
// (used in proxy/stream.go for SetWriteDeadline) can reach it.
func (sw *statusWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}

func (sw *statusWriter) status() int {
	if sw.code == 0 {
		return http.StatusOK
	}
	return sw.code
}
