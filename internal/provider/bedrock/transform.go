package bedrock

import (
	"encoding/json"
	"fmt"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// converseRequest is the AWS Bedrock Converse API request format.
type converseRequest struct {
	Messages        []converseMessage `json:"messages"`
	System          []systemContent   `json:"system,omitempty"`
	InferenceConfig *inferenceConfig  `json:"inferenceConfig,omitempty"`
}

type converseMessage struct {
	Role    string            `json:"role"`
	Content []contentBlock    `json:"content"`
}

type contentBlock struct {
	Text string `json:"text"`
}

type systemContent struct {
	Text string `json:"text"`
}

type inferenceConfig struct {
	MaxTokens   *int     `json:"maxTokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
}

// converseResponse is the AWS Bedrock Converse API response format.
type converseResponse struct {
	Output struct {
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	} `json:"output"`
	StopReason string `json:"stopReason"`
	Usage      struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
		TotalTokens  int `json:"totalTokens"`
	} `json:"usage"`
}

// converseStreamEvent is a single event in the Bedrock ConverseStream response.
type converseStreamEvent struct {
	ContentBlockDelta *struct {
		Delta struct {
			Text string `json:"text"`
		} `json:"delta"`
		ContentBlockIndex int `json:"contentBlockIndex"`
	} `json:"contentBlockDelta"`
	MessageStop *struct {
		StopReason string `json:"stopReason"`
	} `json:"messageStop"`
	Metadata *struct {
		Usage struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
			TotalTokens  int `json:"totalTokens"`
		} `json:"usage"`
	} `json:"metadata"`
}

// BuildConverseRequest converts an OpenAI ChatCompletionRequest to a Bedrock Converse API request.
func BuildConverseRequest(req *types.ChatCompletionRequest) ([]byte, error) {
	cr := converseRequest{
		InferenceConfig: &inferenceConfig{
			MaxTokens:   req.MaxTokens,
			Temperature: req.Temperature,
		},
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			cr.System = append(cr.System, systemContent{Text: msg.Content})
		case "user", "assistant":
			cr.Messages = append(cr.Messages, converseMessage{
				Role:    msg.Role,
				Content: []contentBlock{{Text: msg.Content}},
			})
		}
	}

	body, err := json.Marshal(cr)
	if err != nil {
		return nil, fmt.Errorf("marshal bedrock request: %w", err)
	}
	return body, nil
}

// ParseConverseResponse converts a Bedrock Converse API response to OpenAI format.
func ParseConverseResponse(originalModel string, respBody []byte) (*types.ChatCompletionResponse, error) {
	var cr converseResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return nil, fmt.Errorf("unmarshal bedrock response: %w", err)
	}

	content := ""
	for _, c := range cr.Output.Message.Content {
		content += c.Text
	}

	finishReason := mapStopReason(cr.StopReason)

	resp := &types.ChatCompletionResponse{
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
			PromptTokens:     cr.Usage.InputTokens,
			CompletionTokens: cr.Usage.OutputTokens,
			TotalTokens:      cr.Usage.TotalTokens,
		},
	}
	return resp, nil
}

func mapStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}
