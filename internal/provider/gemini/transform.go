package gemini

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// --- Gemini request types ---

type request struct {
	Contents          []content          `json:"contents"`
	SystemInstruction *systemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *generationConfig  `json:"generationConfig,omitempty"`
}

type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type systemInstruction struct {
	Parts []part `json:"parts"`
}

type generationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

// --- Gemini response types ---

type response struct {
	Candidates    []candidate   `json:"candidates"`
	UsageMetadata usageMetadata `json:"usageMetadata"`
}

type candidate struct {
	Content      content `json:"content"`
	FinishReason string  `json:"finishReason"`
	Index        int     `json:"index"`
}

type usageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// BuildRequest converts an OpenAI ChatCompletionRequest to Gemini format.
// system role messages become systemInstruction; user/assistant become contents.
// assistant role maps to "model" as required by Gemini.
func BuildRequest(req *types.ChatCompletionRequest) ([]byte, error) {
	gr := request{}

	gc := &generationConfig{
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxOutputTokens: req.MaxTokens,
	}

	// Parse stop (string or []string)
	if len(req.Stop) > 0 {
		var s string
		if err := json.Unmarshal(req.Stop, &s); err == nil {
			gc.StopSequences = []string{s}
		} else {
			var ss []string
			if err := json.Unmarshal(req.Stop, &ss); err == nil {
				gc.StopSequences = ss
			}
		}
	}
	gr.GenerationConfig = gc

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			gr.SystemInstruction = &systemInstruction{
				Parts: []part{{Text: msg.Content}},
			}
		case "assistant":
			gr.Contents = append(gr.Contents, content{
				Role:  "model",
				Parts: []part{{Text: msg.Content}},
			})
		default: // user
			gr.Contents = append(gr.Contents, content{
				Role:  msg.Role,
				Parts: []part{{Text: msg.Content}},
			})
		}
	}

	return json.Marshal(gr)
}

// ParseResponse converts a Gemini generateContent response to OpenAI format.
func ParseResponse(originalModel string, respBody []byte) (*types.ChatCompletionResponse, error) {
	var gr response
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return nil, fmt.Errorf("unmarshal gemini response: %w", err)
	}

	if len(gr.Candidates) == 0 {
		return nil, fmt.Errorf("gemini returned no candidates")
	}

	text := ""
	for _, p := range gr.Candidates[0].Content.Parts {
		text += p.Text
	}

	return &types.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-gemini-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   originalModel,
		Choices: []types.Choice{{
			Index:        0,
			Message:      types.Message{Role: "assistant", Content: text},
			FinishReason: mapFinishReason(gr.Candidates[0].FinishReason),
		}},
		Usage: &types.Usage{
			PromptTokens:     gr.UsageMetadata.PromptTokenCount,
			CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gr.UsageMetadata.TotalTokenCount,
		},
	}, nil
}

func mapFinishReason(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY":
		return "content_filter"
	default:
		return "stop"
	}
}
