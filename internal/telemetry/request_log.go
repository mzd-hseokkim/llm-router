package telemetry

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// LogEntry holds all metadata captured for a single API request.
type LogEntry struct {
	RequestID        string         `json:"request_id"`
	Timestamp        time.Time      `json:"timestamp"`
	Model            string         `json:"model"`
	Provider         string         `json:"provider"`
	VirtualKeyID     *uuid.UUID     `json:"virtual_key_id,omitempty"`
	UserID           *uuid.UUID     `json:"user_id,omitempty"`
	TeamID           *uuid.UUID     `json:"team_id,omitempty"`
	OrgID            *uuid.UUID     `json:"org_id,omitempty"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	TotalTokens      int            `json:"total_tokens"`
	CostUSD          float64        `json:"cost_usd"`
	LatencyMs        int64          `json:"latency_ms"`
	TTFTMs           *int64         `json:"ttft_ms,omitempty"`
	StatusCode       int            `json:"status_code"`
	FinishReason     string         `json:"finish_reason,omitempty"`
	CacheHit         bool           `json:"cache_hit"`
	IsStreaming      bool           `json:"is_streaming"`
	ErrorCode        string         `json:"error_code,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// requestLogCtxKey is the private context key for RequestLogContext.
type requestLogCtxKey struct{}

// RequestLogContext is a mutable struct stored in the request context.
// Handlers populate it during request processing; the logging middleware reads
// it once after the handler returns.
//
// No mutex needed: all writes happen inside the handler (synchronously),
// and the middleware reads only after the handler has returned.
type RequestLogContext struct {
	// Set by auth middleware
	VirtualKeyID *uuid.UUID
	UserID       *uuid.UUID
	TeamID       *uuid.UUID
	OrgID        *uuid.UUID

	// Set by handlers
	Model            string
	Provider         string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	FinishReason     string
	ErrorCode        string
	ErrorMessage     string
	IsStreaming      bool
	TTFTAt           time.Time // zero if first-token time was not recorded

	// Set by budget middleware after handler returns
	CostUSD float64

	// Set by cache middleware (hit|semantic_hit|miss)
	CacheResult string
}

// NewRequestLogContext injects a fresh RequestLogContext into ctx and returns both.
func NewRequestLogContext(ctx context.Context) (context.Context, *RequestLogContext) {
	lc := &RequestLogContext{}
	return context.WithValue(ctx, requestLogCtxKey{}, lc), lc
}

// GetRequestLogContext retrieves the RequestLogContext from ctx.
// Returns nil if none was set.
func GetRequestLogContext(ctx context.Context) *RequestLogContext {
	v, _ := ctx.Value(requestLogCtxKey{}).(*RequestLogContext)
	return v
}

// SetVirtualKeyInfo records virtual key ownership metadata.
// Called by the auth middleware right after key validation.
func SetVirtualKeyInfo(ctx context.Context, keyID, userID, teamID, orgID *uuid.UUID) {
	if lc := GetRequestLogContext(ctx); lc != nil {
		lc.VirtualKeyID = keyID
		lc.UserID = userID
		lc.TeamID = teamID
		lc.OrgID = orgID
	}
}

// SetModel records the full model string and resolved provider name.
func SetModel(ctx context.Context, model, provider string) {
	if lc := GetRequestLogContext(ctx); lc != nil {
		lc.Model = model
		lc.Provider = provider
	}
}

// SetTokens records prompt/completion/total token counts.
func SetTokens(ctx context.Context, prompt, completion, total int) {
	if lc := GetRequestLogContext(ctx); lc != nil {
		lc.PromptTokens = prompt
		lc.CompletionTokens = completion
		lc.TotalTokens = total
	}
}

// SetFinishReason records the completion finish reason (e.g. "stop", "length").
func SetFinishReason(ctx context.Context, reason string) {
	if lc := GetRequestLogContext(ctx); lc != nil {
		lc.FinishReason = reason
	}
}

// SetError records the error code and message for a failed request.
func SetError(ctx context.Context, code, message string) {
	if lc := GetRequestLogContext(ctx); lc != nil {
		lc.ErrorCode = code
		lc.ErrorMessage = message
	}
}

// SetStreaming marks the request as a streaming (SSE) request.
func SetStreaming(ctx context.Context) {
	if lc := GetRequestLogContext(ctx); lc != nil {
		lc.IsStreaming = true
	}
}

// RecordTTFT records the time of the first content token.
// Subsequent calls are no-ops (only the first token time is kept).
func RecordTTFT(ctx context.Context) {
	if lc := GetRequestLogContext(ctx); lc != nil && lc.TTFTAt.IsZero() {
		lc.TTFTAt = time.Now()
	}
}

// SetCacheResult records the cache lookup result ("hit", "semantic_hit", "miss").
func SetCacheResult(ctx context.Context, result string) {
	if lc := GetRequestLogContext(ctx); lc != nil {
		lc.CacheResult = result
	}
}
