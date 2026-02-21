// Package cohere implements a provider adapter for Cohere.
// Cohere v2 Chat API uses an OpenAI-compatible messages format.
package cohere

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/provider"
)

const defaultBaseURL = "https://api.cohere.com/v2"

// Adapter implements provider.Provider for Cohere.
type Adapter struct {
	keyFunc func(ctx context.Context) (string, error)
	baseURL string
	client  *http.Client
}

// New returns a Cohere Adapter with a static API key.
func New(apiKey, baseURL string) *Adapter {
	return newAdapter(func(_ context.Context) (string, error) { return apiKey, nil }, baseURL)
}

// NewManaged returns a Cohere Adapter that resolves its API key from km at request time.
func NewManaged(km provider.KeyProvider, baseURL string) *Adapter {
	return newAdapter(func(ctx context.Context) (string, error) {
		return km.SelectKey(ctx, "cohere", "")
	}, baseURL)
}

func newAdapter(keyFunc func(context.Context) (string, error), baseURL string) *Adapter {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Adapter{
		keyFunc: keyFunc,
		baseURL: baseURL,
		client:  newHTTPClient(),
	}
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   100,
			ForceAttemptHTTP2:     true,
		},
	}
}

func (a *Adapter) Name() string { return "cohere" }

func (a *Adapter) Models() []types.ModelInfo {
	return []types.ModelInfo{
		{ID: "cohere/command-r-plus-08-2024", Object: "model", OwnedBy: "cohere"},
		{ID: "cohere/command-r-08-2024", Object: "model", OwnedBy: "cohere"},
		{ID: "cohere/command-r7b-12-2024", Object: "model", OwnedBy: "cohere"},
	}
}

// cohereRequest is the Cohere v2 Chat API request body.
type cohereRequest struct {
	Model       string              `json:"model"`
	Messages    []types.Message     `json:"messages"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
	Temperature *float64            `json:"temperature,omitempty"`
	Stream      bool                `json:"stream,omitempty"`
}

// cohereResponse is the Cohere v2 Chat API response body.
type cohereResponse struct {
	ID      string `json:"id"`
	Message struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
	Usage        struct {
		BilledUnits struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"billed_units"`
		Tokens struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"tokens"`
	} `json:"usage"`
}

// ChatCompletion sends a chat completion request to Cohere v2.
func (a *Adapter) ChatCompletion(ctx context.Context, model string, req *types.ChatCompletionRequest, _ []byte) (*types.ChatCompletionResponse, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve cohere api key: %w", err)
	}

	cohReq := cohereRequest{
		Model:       model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	body, err := json.Marshal(cohReq)
	if err != nil {
		return nil, fmt.Errorf("marshal cohere request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create cohere request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read cohere response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, provider.NormalizeHTTPError(resp.StatusCode, string(respBody), resp.Header)
	}

	return parseCohereResponse(req.Model, respBody)
}

// ChatCompletionStream initiates a streaming request to Cohere v2.
func (a *Adapter) ChatCompletionStream(ctx context.Context, model string, req *types.ChatCompletionRequest, _ []byte) (<-chan provider.StreamChunk, error) {
	apiKey, err := a.keyFunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve cohere api key: %w", err)
	}

	cohReq := cohereRequest{
		Model:       model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}
	body, err := json.Marshal(cohReq)
	if err != nil {
		return nil, fmt.Errorf("marshal cohere stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create cohere stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, provider.NewNetworkError(err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, provider.NormalizeHTTPError(resp.StatusCode, string(body), resp.Header)
	}

	return streamCohereSSE(ctx, resp.Body, req.Model), nil
}

func parseCohereResponse(originalModel string, respBody []byte) (*types.ChatCompletionResponse, error) {
	var cr cohereResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return nil, fmt.Errorf("unmarshal cohere response: %w", err)
	}

	// Extract text content from the first content block.
	content := ""
	for _, c := range cr.Message.Content {
		if c.Type == "text" {
			content = c.Text
			break
		}
	}

	finishReason := "stop"
	if cr.FinishReason != "" {
		finishReason = strings.ToLower(cr.FinishReason)
	}

	resp := &types.ChatCompletionResponse{
		ID:     cr.ID,
		Object: "chat.completion",
		Model:  originalModel,
		Choices: []types.Choice{{
			Index: 0,
			Message: types.Message{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: finishReason,
		}},
		Usage: &types.Usage{
			PromptTokens:     cr.Usage.Tokens.InputTokens,
			CompletionTokens: cr.Usage.Tokens.OutputTokens,
			TotalTokens:      cr.Usage.Tokens.InputTokens + cr.Usage.Tokens.OutputTokens,
		},
	}
	return resp, nil
}

// streamCohereSSE reads Cohere's SSE stream and converts it to StreamChunks.
// Cohere v2 streaming uses JSON lines with event types.
func streamCohereSSE(ctx context.Context, body io.ReadCloser, model string) <-chan provider.StreamChunk {
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
				leftover = parseCohereLines(ctx, ch, leftover)
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

func parseCohereLines(ctx context.Context, ch chan<- provider.StreamChunk, buf []byte) []byte {
	for {
		idx := strings.IndexByte(string(buf), '\n')
		if idx < 0 {
			return buf
		}
		line := strings.TrimRight(string(buf[:idx]), "\r")
		buf = buf[idx+1:]

		if line == "" {
			continue
		}

		// Cohere v2 streams JSON lines.
		var event struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Delta struct {
				Message struct {
					Content struct {
						Index int    `json:"index"`
						Text  string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Also try SSE format (data: {...}).
			if strings.HasPrefix(line, "data: ") {
				line = line[6:]
				json.Unmarshal([]byte(line), &event) //nolint:errcheck
			} else {
				continue
			}
		}

		sc := provider.StreamChunk{}
		switch event.Type {
		case "content-delta":
			sc.Delta = event.Delta.Message.Content.Text
		case "message-end":
			fr := strings.ToLower(event.FinishReason)
			if fr == "" {
				fr = "stop"
			}
			sc.FinishReason = &fr
		default:
			continue
		}

		select {
		case <-ctx.Done():
			return buf
		case ch <- sc:
		}
	}
}
