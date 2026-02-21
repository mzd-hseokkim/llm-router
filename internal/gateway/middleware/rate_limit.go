package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/llm-router/gateway/internal/auth"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/ratelimit"
)

// RateLimit returns a middleware that enforces per-key RPM and TPM limits.
// Limits are read from the authenticated VirtualKey in context.
// If no key is present or no limit is set, the request passes through.
func RateLimit(limiter ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := auth.GetVirtualKey(r.Context())
			if key == nil {
				next.ServeHTTP(w, r)
				return
			}

			window := time.Minute
			minute := ratelimit.WindowMinute()

			// --- RPM check ---
			if key.RPMLimit != nil && *key.RPMLimit > 0 {
				rpmKey := ratelimit.KeyForKeyRPM(key.ID.String(), minute)
				res, err := limiter.Allow(r.Context(), rpmKey, *key.RPMLimit, 1, window)
				if err == nil && !res.Allowed {
					writeRateLimitError(w, "rpm", *key.RPMLimit, res)
					return
				}
			}

			next.ServeHTTP(w, r)

			// --- Post-request: TPM is recorded by the caller via RecordTPM ---
			// TPM enforcement is "next-request" based: the TPM counter is incremented
			// after each request, and checked on the following request.
		})
	}
}

// RecordTPM adds token usage to the per-key TPM sliding window after a request completes.
// Call this after the handler returns, using the actual token counts.
// Errors are silently discarded — token recording is best-effort.
func RecordTPM(limiter ratelimit.Limiter, key *auth.VirtualKey, tokens int, r *http.Request) {
	if key == nil || key.TPMLimit == nil || tokens == 0 {
		return
	}
	minute := ratelimit.WindowMinute()
	tpmKey := ratelimit.KeyForKeyTPM(key.ID.String(), minute)
	// Use a very high limit so ZADD always succeeds; the real limit check happens
	// on the next request by calling CheckTPM.
	_, _ = limiter.Allow(r.Context(), tpmKey, *key.TPMLimit+tokens, tokens, time.Minute)
}

// CheckTPM checks the TPM counter for the current minute.
// Returns true (blocked) if the limit is exceeded.
func CheckTPM(limiter ratelimit.Limiter, key *auth.VirtualKey, w http.ResponseWriter, r *http.Request) bool {
	if key == nil || key.TPMLimit == nil || *key.TPMLimit <= 0 {
		return false
	}
	minute := ratelimit.WindowMinute()
	tpmKey := ratelimit.KeyForKeyTPM(key.ID.String(), minute)
	// cost=0 means peek-only: check remaining without consuming.
	// We use cost=1 for a tiny probe; the real cost is recorded post-request.
	res, err := limiter.Allow(r.Context(), tpmKey, *key.TPMLimit, 0, time.Minute)
	if err != nil {
		return false // fail open on Redis error
	}
	if !res.Allowed {
		writeRateLimitError(w, "tpm", *key.TPMLimit, res)
		return true
	}
	return false
}

func writeRateLimitError(w http.ResponseWriter, metric string, limit int, res ratelimit.Result) {
	retryAfter := 60
	if !res.ResetAt.IsZero() {
		secs := int(time.Until(res.ResetAt).Seconds())
		if secs > 0 {
			retryAfter = secs
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
	w.Header().Set(fmt.Sprintf("X-RateLimit-Limit-%s", metric), fmt.Sprintf("%d", limit))
	w.Header().Set(fmt.Sprintf("X-RateLimit-Remaining-%s", metric), fmt.Sprintf("%d", res.Remaining))
	if !res.ResetAt.IsZero() {
		w.Header().Set(fmt.Sprintf("X-RateLimit-Reset-%s", metric), fmt.Sprintf("%d", res.ResetAt.Unix()))
	}
	w.WriteHeader(http.StatusTooManyRequests)

	json.NewEncoder(w).Encode(types.ErrorResponse{
		Error: types.ErrorDetail{
			Message: fmt.Sprintf("Rate limit exceeded: %d %s. Retry after %d seconds.", limit, metric, retryAfter),
			Type:    "rate_limit_error",
			Code:    "rate_limit_exceeded",
		},
	})
}
