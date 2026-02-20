package anthropic

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
)

const defaultMaxTokens = 4096

// --- Anthropic request types ---

type request struct {
	Model         string    `json:"model"`
	MaxTokens     int       `json:"max_tokens"`
	System        string    `json:"system,omitempty"`
	Messages      []message `json:"messages"`
	Temperature   *float64  `json:"temperature,omitempty"`
	TopP          *float64  `json:"top_p,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
	Stream        bool      `json:"stream,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// --- Anthropic response types ---

type response struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Role       string    `json:"role"`
	Model      string    `json:"model"`
	Content    []content `json:"content"`
	StopReason string    `json:"stop_reason"`
	Usage      usage     `json:"usage"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// BuildRequest converts an OpenAI ChatCompletionRequest to Anthropic's format.
// system role messages are extracted into the top-level system field.
// max_tokens defaults to 4096 if not set (required by Anthropic API).
func BuildRequest(model string, req *types.ChatCompletionRequest) ([]byte, error) {
	ar := request{
		Model:       model,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	if req.MaxTokens != nil {
		ar.MaxTokens = *req.MaxTokens
	} else {
		ar.MaxTokens = defaultMaxTokens
	}

	// Parse stop (string or []string)
	if len(req.Stop) > 0 {
		var s string
		if err := json.Unmarshal(req.Stop, &s); err == nil {
			ar.StopSequences = []string{s}
		} else {
			var ss []string
			if err := json.Unmarshal(req.Stop, &ss); err == nil {
				ar.StopSequences = ss
			}
		}
	}

	// Separate system messages; keep user/assistant in messages array
	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			ar.System = msg.Content
		default:
			ar.Messages = append(ar.Messages, message{Role: msg.Role, Content: msg.Content})
		}
	}

	return json.Marshal(ar)
}

// ParseResponse converts an Anthropic messages response to OpenAI format.
func ParseResponse(originalModel string, respBody []byte) (*types.ChatCompletionResponse, error) {
	var ar response
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return nil, fmt.Errorf("unmarshal anthropic response: %w", err)
	}

	text := ""
	for _, c := range ar.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}

	return &types.ChatCompletionResponse{
		ID:      "chatcmpl-" + ar.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   originalModel,
		Choices: []types.Choice{{
			Index:        0,
			Message:      types.Message{Role: "assistant", Content: text},
			FinishReason: mapStopReason(ar.StopReason),
		}},
		Usage: &types.Usage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		},
	}, nil
}

func mapStopReason(reason string) string {
	switch reason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}
