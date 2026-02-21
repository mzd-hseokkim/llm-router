package exact

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// ReplayAsStream replays a cached non-streaming response as an SSE stream.
// This allows streaming requests to be served from the exact-match cache.
func ReplayAsStream(w http.ResponseWriter, cached *types.ChatCompletionResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Cache", "HIT")
	w.WriteHeader(http.StatusOK)

	flusher, canFlush := w.(http.Flusher)

	if len(cached.Choices) == 0 {
		writeSSEDone(w)
		return
	}

	content := cached.Choices[0].Message.Content
	words := strings.Fields(content)

	// Emit role delta first
	roleChunk := types.ChatCompletionChunk{
		ID:      cached.ID,
		Object:  "chat.completion.chunk",
		Created: cached.Created,
		Model:   cached.Model,
		Choices: []types.ChunkChoice{
			{Index: 0, Delta: types.Delta{Role: "assistant"}},
		},
	}
	writeSSEChunk(w, &roleChunk)
	if canFlush {
		flusher.Flush()
	}

	for i, word := range words {
		text := word
		if i < len(words)-1 {
			text += " "
		}
		chunk := types.ChatCompletionChunk{
			ID:      cached.ID,
			Object:  "chat.completion.chunk",
			Created: cached.Created,
			Model:   cached.Model,
			Choices: []types.ChunkChoice{
				{Index: 0, Delta: types.Delta{Content: text}},
			},
		}
		writeSSEChunk(w, &chunk)
		if canFlush {
			flusher.Flush()
		}
		time.Sleep(time.Millisecond) // natural streaming cadence
	}

	// Final chunk with finish_reason
	finalChunk := types.ChatCompletionChunk{
		ID:      cached.ID,
		Object:  "chat.completion.chunk",
		Created: cached.Created,
		Model:   cached.Model,
		Choices: []types.ChunkChoice{
			{Index: 0, Delta: types.Delta{}, FinishReason: cached.Choices[0].FinishReason},
		},
		Usage: cached.Usage,
	}
	writeSSEChunk(w, &finalChunk)
	if canFlush {
		flusher.Flush()
	}

	writeSSEDone(w)
	if canFlush {
		flusher.Flush()
	}
}

func writeSSEChunk(w http.ResponseWriter, chunk *types.ChatCompletionChunk) {
	data, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data) //nolint:errcheck
}

func writeSSEDone(w http.ResponseWriter) {
	fmt.Fprint(w, "data: [DONE]\n\n") //nolint:errcheck
}
