package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

// BuildRequest strips the model field from the request body.
// Azure embeds the model/deployment in the URL, not the request body.
func BuildRequest(rawBody []byte) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return nil, fmt.Errorf("unmarshal azure request: %w", err)
	}
	delete(body, "model")
	return json.Marshal(body)
}

// BuildStreamRequest is like BuildRequest but also forces stream=true.
func BuildStreamRequest(rawBody []byte) ([]byte, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return nil, fmt.Errorf("unmarshal azure stream request: %w", err)
	}
	delete(body, "model")
	body["stream"] = json.RawMessage("true")
	return json.Marshal(body)
}

// ParseResponse decodes an Azure OpenAI response and normalizes the model field.
// Azure returns the deployment ID in the model field; we prefix it with "azure/".
func ParseResponse(originalModel string, respBody []byte) (*types.ChatCompletionResponse, error) {
	var resp types.ChatCompletionResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal azure response: %w", err)
	}
	resp.Model = originalModel
	return &resp, nil
}

// streamSSE reads an Azure OpenAI Server-Sent Events stream and emits StreamChunks.
// Azure uses the same SSE format as OpenAI.
func streamSSE(ctx context.Context, body io.ReadCloser, model string) <-chan provider.StreamChunk {
	ch := make(chan provider.StreamChunk, 16)
	go func() {
		defer close(ch)
		defer body.Close()

		buf := make([]byte, 4096)
		var leftover []byte

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := body.Read(buf)
			if n > 0 {
				leftover = append(leftover, buf[:n]...)
				leftover = processSSELines(ctx, ch, leftover)
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

// processSSELines parses SSE lines from buf and sends chunks on ch.
// Returns any incomplete line remaining in the buffer.
func processSSELines(ctx context.Context, ch chan<- provider.StreamChunk, buf []byte) []byte {
	for {
		idx := indexNewline(buf)
		if idx < 0 {
			return buf
		}
		line := string(buf[:idx])
		buf = buf[idx+1:]
		// Skip \r if present.
		if len(buf) > 0 && buf[0] == '\n' {
			buf = buf[1:]
		}

		if line == "" || line == "data: [DONE]" {
			continue
		}
		if len(line) < 6 || line[:6] != "data: " {
			continue
		}

		jsonData := line[6:]
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *types.Usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			continue
		}

		sc := provider.StreamChunk{}
		if len(chunk.Choices) > 0 {
			sc.Delta = chunk.Choices[0].Delta.Content
			sc.FinishReason = chunk.Choices[0].FinishReason
		}
		sc.Usage = chunk.Usage

		select {
		case <-ctx.Done():
			return buf
		case ch <- sc:
		}
	}
}

func indexNewline(buf []byte) int {
	for i, b := range buf {
		if b == '\n' {
			return i
		}
	}
	return -1
}
