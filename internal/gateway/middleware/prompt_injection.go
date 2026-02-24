package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/prompt"
)

// PromptInjector injects rendered prompt templates as system messages into
// POST /v1/chat/completions requests that include a "prompt_slug" field.
func PromptInjector(svc *prompt.Service, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only intercept POST /v1/chat/completions.
			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			var req types.ChatCompletionRequest
			if err := json.Unmarshal(body, &req); err != nil || req.PromptSlug == "" {
				// Not a prompt-injection request — restore body and continue.
				r.Body = io.NopCloser(bytes.NewReader(body))
				next.ServeHTTP(w, r)
				return
			}

			rendered, _, err := svc.RenderActive(r.Context(), req.PromptSlug, req.PromptVariables)
			if err != nil {
				logger.Warn("prompt injection: render failed",
					"slug", req.PromptSlug, "error", err)
				// Fail open: continue without injecting.
				r.Body = io.NopCloser(bytes.NewReader(body))
				next.ServeHTTP(w, r)
				return
			}

			// Prepend a system message with the rendered prompt.
			messages := make([]types.Message, 0, len(req.Messages)+1)
			messages = append(messages, types.Message{Role: "system", Content: rendered})
			messages = append(messages, req.Messages...)
			req.Messages = messages

			slug := req.PromptSlug

			// Clear gateway-specific fields before forwarding.
			req.PromptSlug = ""
			req.PromptVariables = nil

			newBody, err := json.Marshal(req)
			if err != nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
				next.ServeHTTP(w, r)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(newBody))
			r.ContentLength = int64(len(newBody))

			logger.Debug("prompt injected", "slug", slug)
			next.ServeHTTP(w, r)
		})
	}
}
