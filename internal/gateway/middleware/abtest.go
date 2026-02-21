package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/llm-router/gateway/internal/abtest"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/telemetry"
	pgstore "github.com/llm-router/gateway/internal/store/postgres"
)

type abTestCtxKey struct{}

type abTestCtxVal struct {
	experiment *abtest.Experiment
	variant    string
}

// ABTestMiddleware routes a fraction of chat traffic to alternative models
// based on active A/B experiment configurations.
type ABTestMiddleware struct {
	store  *pgstore.ABTestStore
	logger *slog.Logger

	mu    sync.RWMutex
	tests []*abtest.Experiment
}

// NewABTestMiddleware creates the middleware. Call Reload to populate experiments.
func NewABTestMiddleware(store *pgstore.ABTestStore, logger *slog.Logger) *ABTestMiddleware {
	return &ABTestMiddleware{store: store, logger: logger}
}

// Reload re-fetches running experiments from the DB.
func (m *ABTestMiddleware) Reload(ctx context.Context) error {
	all, err := m.store.List(ctx)
	if err != nil {
		return err
	}
	var active []*abtest.Experiment
	for _, e := range all {
		if e.Status == abtest.StatusRunning {
			active = append(active, e)
		}
	}
	m.mu.Lock()
	m.tests = active
	m.mu.Unlock()
	return nil
}

// Handler returns the chi-compatible middleware function.
func (m *ABTestMiddleware) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/chat/completions") || r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			m.mu.RLock()
			tests := m.tests
			m.mu.RUnlock()

			if len(tests) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			var req types.ChatCompletionRequest
			if err := json.Unmarshal(body, &req); err != nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
				next.ServeHTTP(w, r)
				return
			}

			// Use the Authorization header token as stable entity ID.
			entityID := r.Header.Get("Authorization")
			if entityID == "" {
				entityID = r.RemoteAddr
			}

			var matched *abtest.Experiment
			var variant string
			for _, exp := range tests {
				v := abtest.Assign(exp, entityID)
				if v == "" {
					continue
				}
				matched = exp
				variant = v
				break
			}

			if matched == nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
				next.ServeHTTP(w, r)
				return
			}

			// Override model with variant's model (fail-open: keep original on error).
			variantModel := matched.ModelForVariant(variant)
			if variantModel != "" {
				req.Model = variantModel
			}

			newBody, err := json.Marshal(req)
			if err != nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
				next.ServeHTTP(w, r)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(newBody))
			r.ContentLength = int64(len(newBody))
			w.Header().Set("X-AB-Test-ID", matched.ID)
			w.Header().Set("X-AB-Test-Variant", variant)

			// Store assignment in context so result collector can read it.
			ctx := context.WithValue(r.Context(), abTestCtxKey{}, &abTestCtxVal{
				experiment: matched,
				variant:    variant,
			})

			start := time.Now()
			next.ServeHTTP(w, r.WithContext(ctx))

			// Async result recording — does not block the response.
			go m.recordResult(r.Context(), matched, variant, req.Model, start,
				chimiddleware.GetReqID(ctx))
		})
	}
}

// recordResult collects post-request metrics from the RequestLogContext and stores them.
func (m *ABTestMiddleware) recordResult(
	origCtx context.Context,
	exp *abtest.Experiment,
	variant, model string,
	start time.Time,
	requestID string,
) {
	lc := telemetry.GetRequestLogContext(origCtx)

	latencyMs := int(time.Since(start).Milliseconds())
	isErr := false
	finishReason := ""
	promptTokens, completionTokens := 0, 0
	costUSD := 0.0

	if lc != nil {
		if lc.ErrorCode != "" {
			isErr = true
		}
		finishReason = lc.FinishReason
		promptTokens = lc.PromptTokens
		completionTokens = lc.CompletionTokens
		costUSD = lc.CostUSD
	}

	result := abtest.Result{
		TestID:           exp.ID,
		Variant:          variant,
		RequestID:        requestID,
		Timestamp:        start,
		Model:            model,
		LatencyMs:        latencyMs,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		CostUSD:          costUSD,
		Error:            isErr,
		FinishReason:     finishReason,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.store.InsertResult(ctx, result); err != nil {
		m.logger.Warn("abtest: failed to record result", "error", err)
	}

	// Auto-stop if error rate exceeds 20% (checked after insert).
	m.checkAutoStop(ctx, exp, variant)
}

// checkAutoStop stops the experiment early when error rate exceeds threshold.
func (m *ABTestMiddleware) checkAutoStop(ctx context.Context, exp *abtest.Experiment, variant string) {
	stats, err := m.store.GetVariantStats(ctx, exp.ID, variant)
	if err != nil || stats.Samples < 100 {
		return
	}
	if stats.ErrorRate > 0.20 {
		m.logger.Warn("abtest: auto-stopping experiment — error rate exceeded 20%",
			"test_id", exp.ID, "variant", variant, "error_rate", stats.ErrorRate)
		_ = m.store.UpdateStatus(ctx, exp.ID, string(abtest.StatusStopped), "")
		_ = m.Reload(ctx)
	}
}
