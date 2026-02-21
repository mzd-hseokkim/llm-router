package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/guardrail"
)

// GuardrailCheck returns a middleware that runs input and output guardrails
// on chat completion requests. Input guardrails inspect user messages before
// the request is forwarded to the provider. Output guardrails inspect the
// response for non-streaming requests.
//
// The middleware fetches the active pipeline from the Manager on each request,
// so pipeline hot-reloads via Manager.SetPipeline take effect immediately.
func GuardrailCheck(manager *guardrail.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fetch the current pipeline; skip if none configured.
			pipeline := manager.Pipeline()
			if pipeline == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Only apply to chat completions path
			if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			// Restore body for the next handler
			r.Body = io.NopCloser(bytes.NewReader(body))

			var req types.ChatCompletionRequest
			if err := json.Unmarshal(body, &req); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Check each user message independently so PII masking is applied
			// per-message and previous messages are not left unmasked.
			bodyChanged := false
			for i, m := range req.Messages {
				if m.Role != "user" || m.Content == "" {
					continue
				}
				modified, blockErr, err := pipeline.CheckInput(r.Context(), m.Content)
				if err == nil && blockErr != nil {
					writeGuardrailError(w, blockErr)
					return
				}
				if err == nil && modified != m.Content {
					req.Messages[i].Content = modified
					bodyChanged = true
				}
			}
			if bodyChanged {
				newBody, _ := json.Marshal(req)
				r.Body = io.NopCloser(bytes.NewReader(newBody))
			}

			// For non-streaming with output guardrails: wrap ResponseWriter
			if !req.Stream && pipeline.HasOutput() {
				rw := &responseCapture{ResponseWriter: w, body: &bytes.Buffer{}, statusCode: http.StatusOK}
				next.ServeHTTP(rw, r)
				outputText := rw.body.String()
				if outputText != "" {
					filtered, blockErr, _ := pipeline.CheckOutput(r.Context(), outputText)
					if blockErr != nil {
						// Replace response with guardrail error
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						writeGuardrailError(w, blockErr) //nolint:errcheck
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(rw.statusCode)
					w.Write([]byte(filtered)) //nolint:errcheck
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// responseCapture buffers a non-streaming response for output guardrail inspection.
type responseCapture struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (rc *responseCapture) WriteHeader(code int) {
	rc.statusCode = code
	// Don't write headers yet; we may replace the response
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	return rc.body.Write(b)
}

func writeGuardrailError(w http.ResponseWriter, be *guardrail.BlockError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"error": map[string]any{
			"message": be.Message,
			"type":    "content_policy_violation",
			"code":    be.Guardrail,
			"param": map[string]string{
				"guardrail": be.Guardrail,
				"category":  be.Category,
			},
		},
	})
}
