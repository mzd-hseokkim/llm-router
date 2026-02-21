package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/llm-router/gateway/internal/auth"
	"github.com/llm-router/gateway/internal/budget"
	"github.com/llm-router/gateway/internal/cost"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/telemetry"
)

// BudgetCheck returns a middleware that enforces per-key budget limits.
// After the handler completes, it records the actual spend using token counts
// from the request log context.
func BudgetCheck(mgr *budget.Manager, calc *cost.Calculator, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := auth.GetVirtualKey(r.Context())
			if key == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Pre-flight: check hard budget limit for the key.
			if err := mgr.CheckBudget(r.Context(), "key", key.ID); err != nil {
				if exc, ok := budget.IsBudgetExceeded(err); ok {
					writeBudgetError(w, exc)
					return
				}
				logger.Error("budget check error", "error", err, "key_id", key.ID)
				// Fail open on unexpected errors.
			}

			next.ServeHTTP(w, r)

			// Post-flight: record actual spend.
			lc := telemetry.GetRequestLogContext(r.Context())
			if lc == nil || calc == nil {
				return
			}
			costUSD := calc.Calculate(lc.Model, lc.PromptTokens, lc.CompletionTokens)
			if costUSD == 0 {
				return
			}

			// Store cost in log context so the logger can pick it up.
			lc.CostUSD = costUSD

			// Record spend asynchronously (best-effort).
			go func() {
				mgr.RecordSpend(r.Context(), "key", key.ID, costUSD)
				if key.TeamID != nil {
					mgr.RecordSpend(r.Context(), "team", *key.TeamID, costUSD)
				}
				if key.OrgID != nil {
					mgr.RecordSpend(r.Context(), "org", *key.OrgID, costUSD)
				}
			}()
		})
	}
}

func writeBudgetError(w http.ResponseWriter, exc budget.ErrBudgetExceeded) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": fmt.Sprintf("Budget exceeded: hard limit $%.4f reached. Current spend: $%.4f (%s)",
				exc.Limit, exc.Current, exc.Period),
			"type": "budget_exceeded_error",
			"code": "budget_exceeded",
			"param": map[string]interface{}{
				"limit_usd":        exc.Limit,
				"current_spend_usd": exc.Current,
				"period":           exc.Period,
			},
		},
	})
}

// writeBudgetErrorSimple is used when only the key's budget_usd (lifetime) is exceeded.
func writeBudgetErrorSimple(w http.ResponseWriter, current, limit float64) {
	writeBudgetError(w, budget.ErrBudgetExceeded{
		Current: current,
		Limit:   limit,
		Period:  "lifetime",
	})
}

// Ensure types.ErrorResponse is imported to avoid IDE noise.
var _ = types.ErrorResponse{}
