package proxy

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
	"github.com/llm-router/gateway/internal/telemetry"
)

const (
	keepaliveInterval = 15 * time.Second
	streamMaxDuration = 5 * time.Minute
	writeTimeout      = 30 * time.Second
)

// StreamToClient proxies a streaming chat completion from p to the HTTP client.
// It sets SSE response headers, forwards each chunk immediately, sends keepalive
// comments every 15 s, and cancels the upstream request if the client disconnects.
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

	// Set SSE headers before the first write.
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Derive a context that is cancelled on stream timeout OR client disconnect.
	ctx := r.Context()

	rc := http.NewResponseController(w)

	chunks, err := p.ChatCompletionStream(ctx, parsed.Model, req, rawBody)
	if err != nil {
		logger.Error("provider stream init failed",
			"provider", parsed.Provider,
			"model", parsed.Model,
			"error", err)
		writeSSEError(w, err.Error())
		flusher.Flush()
		return
	}

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
			// Client disconnected or stream timed out; upstream is cancelled via ctx.
			return

		case <-ka.C:
			resetWriteDeadline(rc)
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()

		case chunk, open := <-chunks:
			if !open {
				// Provider goroutine closed the channel — stream ended normally.
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
		// Finish chunk: empty delta, finish_reason set.
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
	errData, _ := json.Marshal(map[string]string{"error": msg})
	fmt.Fprintf(w, "data: %s\n\n", errData)
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func resetWriteDeadline(rc *http.ResponseController) {
	// Best-effort; ignore if the transport doesn't support it.
	_ = rc.SetWriteDeadline(time.Now().Add(writeTimeout))
}

func newStreamID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("chatcmpl-%x", b)
}
