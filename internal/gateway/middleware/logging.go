package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/llm-router/gateway/internal/telemetry"
)

// RequestLogger returns a middleware that records a LogEntry for every request
// that has model information (i.e. LLM API calls). Non-API paths like /ping
// are skipped because handlers never call telemetry.SetModel.
func RequestLogger(w *telemetry.LogWriter, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Inject a mutable log context so handlers can record model/token data.
			ctx, lc := telemetry.NewRequestLogContext(r.Context())
			r = r.WithContext(ctx)

			// Wrap the writer to capture the HTTP status code.
			sw := &statusWriter{ResponseWriter: rw}

			next.ServeHTTP(sw, r)

			// Skip logging for requests that aren't LLM API calls.
			if lc.Model == "" {
				return
			}

			latencyMs := time.Since(start).Milliseconds()

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
				FinishReason:     lc.FinishReason,
				ErrorCode:        lc.ErrorCode,
				ErrorMessage:     lc.ErrorMessage,
				IsStreaming:      lc.IsStreaming,
				StatusCode:       sw.status(),
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
