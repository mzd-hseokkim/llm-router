package proxy

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/llm-router/gateway/internal/gateway/retry"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/telemetry"
)

const (
	keepaliveInterval = 15 * time.Second
	writeTimeout      = 30 * time.Second
)

// StreamToClient proxies a streaming chat completion from p to the HTTP client.
// The provider stream is initiated (with retry) before writing SSE headers, so
// a proper HTTP error status can be returned if init fails.
func StreamToClient(
	w http.ResponseWriter,
	r *http.Request,
	p provider.Provider,
	parsed types.ParsedModel,
	req *types.ChatCompletionRequest,
	rawBody []byte,
	logger *slog.Logger,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported by server", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	// Initiate the provider stream with retry BEFORE writing SSE headers.
	// This allows returning a proper HTTP error status if the provider fails.
	var chunks <-chan provider.StreamChunk
	if err := retry.Execute(ctx, retry.Default(), func() error {
		var e error
		chunks, e = p.ChatCompletionStream(ctx, parsed.Model, req, rawBody)
		return e
	}); err != nil {
		logger.Error("provider stream init failed",
			"provider", parsed.Provider,
			"model", parsed.Model,
			"error", err)
		var gwErr *provider.GatewayError
		if errors.As(err, &gwErr) {
			telemetry.SetError(ctx, string(gwErr.Code), gwErr.Message)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(gwErr.HTTPStatus)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": gwErr.Message,
					"type":    string(gwErr.Code),
				},
			})
		} else {
			telemetry.SetError(ctx, "stream_error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": err.Error(),
					"type":    "api_error",
				},
			})
		}
		return
	}

	// Stream init succeeded — now commit to SSE.
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	rc := http.NewResponseController(w)

	id := newStreamID()
	created := time.Now().Unix()
	model := req.Model

	// Send the role delta first (OpenAI convention).
	resetWriteDeadline(rc)
	writeChunk(w, types.ChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []types.ChunkChoice{{Index: 0, Delta: types.Delta{Role: "assistant"}}},
	})
	flusher.Flush()

	ka := time.NewTicker(keepaliveInterval)
	defer ka.Stop()

	for {
		select {

		case <-ctx.Done():
			// Client disconnected or context cancelled; upstream cancelled via ctx.
			return

		case <-ka.C:
			resetWriteDeadline(rc)
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()

		case chunk, open := <-chunks:
			if !open {
				resetWriteDeadline(rc)
				fmt.Fprint(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			if chunk.Error != nil {
				logger.Error("stream chunk error",
					"provider", parsed.Provider,
					"error", chunk.Error)
				telemetry.SetError(ctx, "stream_error", chunk.Error.Error())
				writeSSEError(w, chunk.Error.Error())
				flusher.Flush()
				return
			}

			if chunk.Delta != "" {
				telemetry.RecordTTFT(ctx)
			}
			if chunk.Usage != nil {
				telemetry.SetTokens(ctx, chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, chunk.Usage.TotalTokens)
			}
			if chunk.FinishReason != nil {
				telemetry.SetFinishReason(ctx, *chunk.FinishReason)
			}

			resetWriteDeadline(rc)
			writeChunk(w, buildChunk(id, created, model, chunk))
			flusher.Flush()
		}
	}
}

// buildChunk converts a provider.StreamChunk to the OpenAI SSE chunk format.
func buildChunk(id string, created int64, model string, sc provider.StreamChunk) types.ChatCompletionChunk {
	c := types.ChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Usage:   sc.Usage,
	}

	if sc.FinishReason != nil {
		c.Choices = []types.ChunkChoice{{
			Index:        0,
			Delta:        types.Delta{},
			FinishReason: *sc.FinishReason,
		}}
	} else {
		c.Choices = []types.ChunkChoice{{
			Index: 0,
			Delta: types.Delta{Content: sc.Delta},
		}}
	}

	return c
}

func writeChunk(w http.ResponseWriter, chunk types.ChatCompletionChunk) {
	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

func writeSSEError(w http.ResponseWriter, msg string) {
	errData, _ := json.Marshal(map[string]any{
		"error": map[string]string{
			"message": msg,
			"type":    "provider_error",
		},
	})
	fmt.Fprintf(w, "data: %s\n\n", errData)
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func resetWriteDeadline(rc *http.ResponseController) {
	_ = rc.SetWriteDeadline(time.Now().Add(writeTimeout))
}

func newStreamID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("chatcmpl-%x", b)
}

func newCmplStreamID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("cmpl-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("cmpl-%x", b)
}

// StreamTextChannelToClient streams an already-opened provider channel to the HTTP client
// as SSE in the legacy text_completion format (/v1/completions).
func StreamTextChannelToClient(
	w http.ResponseWriter,
	r *http.Request,
	chunks <-chan provider.StreamChunk,
	model, providerName string,
	logger *slog.Logger,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported by server", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	rc := http.NewResponseController(w)

	id := newCmplStreamID()
	created := time.Now().Unix()

	ka := time.NewTicker(keepaliveInterval)
	defer ka.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ka.C:
			resetWriteDeadline(rc)
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()

		case chunk, open := <-chunks:
			if !open {
				resetWriteDeadline(rc)
				fmt.Fprint(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			if chunk.Error != nil {
				logger.Error("stream chunk error",
					"provider", providerName,
					"error", chunk.Error)
				telemetry.SetError(ctx, "stream_error", chunk.Error.Error())
				writeSSEError(w, chunk.Error.Error())
				flusher.Flush()
				return
			}

			if chunk.Delta != "" {
				telemetry.RecordTTFT(ctx)
			}
			if chunk.Usage != nil {
				telemetry.SetTokens(ctx, chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, chunk.Usage.TotalTokens)
			}
			if chunk.FinishReason != nil {
				telemetry.SetFinishReason(ctx, *chunk.FinishReason)
			}

			resetWriteDeadline(rc)
			writeTextChunk(w, buildTextChunk(id, created, model, chunk))
			flusher.Flush()
		}
	}
}

// buildTextChunk converts a provider.StreamChunk to the legacy text_completion SSE chunk format.
func buildTextChunk(id string, created int64, model string, sc provider.StreamChunk) types.CompletionStreamChunk {
	c := types.CompletionStreamChunk{
		ID:      id,
		Object:  "text_completion",
		Created: created,
		Model:   model,
		Usage:   sc.Usage,
	}
	fr := ""
	if sc.FinishReason != nil {
		fr = *sc.FinishReason
	}
	c.Choices = []types.CompletionStreamChoice{{
		Index:        0,
		Text:         sc.Delta,
		FinishReason: fr,
	}}
	return c
}

func writeTextChunk(w http.ResponseWriter, chunk types.CompletionStreamChunk) {
	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// StreamChannelToClient streams an already-opened provider channel to the HTTP client as SSE.
// Use this when the stream has been opened by a FallbackRouter (so no retry/fallback is done here).
func StreamChannelToClient(
	w http.ResponseWriter,
	r *http.Request,
	chunks <-chan provider.StreamChunk,
	model, providerName string,
	logger *slog.Logger,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported by server", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	rc := http.NewResponseController(w)

	id := newStreamID()
	created := time.Now().Unix()

	resetWriteDeadline(rc)
	writeChunk(w, types.ChatCompletionChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []types.ChunkChoice{{Index: 0, Delta: types.Delta{Role: "assistant"}}},
	})
	flusher.Flush()

	ka := time.NewTicker(keepaliveInterval)
	defer ka.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ka.C:
			resetWriteDeadline(rc)
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()

		case chunk, open := <-chunks:
			if !open {
				resetWriteDeadline(rc)
				fmt.Fprint(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			if chunk.Error != nil {
				logger.Error("stream chunk error",
					"provider", providerName,
					"error", chunk.Error)
				telemetry.SetError(ctx, "stream_error", chunk.Error.Error())
				writeSSEError(w, chunk.Error.Error())
				flusher.Flush()
				return
			}

			if chunk.Delta != "" {
				telemetry.RecordTTFT(ctx)
			}
			if chunk.Usage != nil {
				telemetry.SetTokens(ctx, chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, chunk.Usage.TotalTokens)
			}
			if chunk.FinishReason != nil {
				telemetry.SetFinishReason(ctx, *chunk.FinishReason)
			}

			resetWriteDeadline(rc)
			writeChunk(w, buildChunk(id, created, model, chunk))
			flusher.Flush()
		}
	}
}
