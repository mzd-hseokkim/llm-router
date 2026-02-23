package types

import "encoding/json"

// Message is an OpenAI-compatible chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamOptions controls streaming behavior.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ChatCompletionRequest is the OpenAI-compatible chat completions request body.
// Known fields are parsed into struct fields; unknown/future fields are preserved
// via the raw body at the handler level for provider pass-through.
type ChatCompletionRequest struct {
	Model            string            `json:"model"`
	Messages         []Message         `json:"messages"`
	Temperature      *float64          `json:"temperature,omitempty"`
	MaxTokens        *int              `json:"max_tokens,omitempty"`
	TopP             *float64          `json:"top_p,omitempty"`
	N                *int              `json:"n,omitempty"`
	Stream           bool              `json:"stream,omitempty"`
	StreamOptions    *StreamOptions    `json:"stream_options,omitempty"`
	Stop             json.RawMessage   `json:"stop,omitempty"` // string or []string
	PresencePenalty  *float64          `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64          `json:"frequency_penalty,omitempty"`
	User             string            `json:"user,omitempty"`
	Tools            []json.RawMessage `json:"tools,omitempty"`            // tool definitions
	Metadata         map[string]string `json:"metadata,omitempty"`         // gateway routing metadata
	// PromptSlug and PromptVariables are Gateway-specific fields consumed by the
	// prompt injection middleware before the request is forwarded to a provider.
	PromptSlug      string            `json:"prompt_slug,omitempty"`
	PromptVariables map[string]string `json:"prompt_variables,omitempty"`
}

// ChatCompletionResponse is the OpenAI-compatible chat completions response.
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Choice is a single completion choice in a chat response.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage contains token usage information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionChunk is a single SSE chunk in a streaming chat response.
type ChatCompletionChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"`
}

// ChunkChoice is a single choice in a streaming chunk.
type ChunkChoice struct {
	Index        int    `json:"index"`
	Delta        Delta  `json:"delta"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// Delta carries the incremental content in a streaming chunk.
type Delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// CompletionRequest is the OpenAI-compatible legacy text completions request.
type CompletionRequest struct {
	Model       string          `json:"model"`
	Prompt      json.RawMessage `json:"prompt"` // string or []string
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	User        string          `json:"user,omitempty"`
}

// CompletionResponse is the OpenAI-compatible legacy text completions response.
type CompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []CompletionChoice `json:"choices"`
	Usage   *Usage             `json:"usage,omitempty"`
}

// CompletionChoice is a single choice in a text completion response.
type CompletionChoice struct {
	Index        int    `json:"index"`
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
}

// CompletionStreamChunk is a single SSE chunk in a streaming text completion response.
type CompletionStreamChunk struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"` // "text_completion"
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []CompletionStreamChoice `json:"choices"`
	Usage   *Usage                   `json:"usage,omitempty"`
}

// CompletionStreamChoice is a single choice in a streaming text completion chunk.
type CompletionStreamChoice struct {
	Index        int    `json:"index"`
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// EmbeddingRequest is the OpenAI-compatible embeddings request.
type EmbeddingRequest struct {
	Model          string          `json:"model"`
	Input          json.RawMessage `json:"input"` // string or []string
	EncodingFormat string          `json:"encoding_format,omitempty"`
	User           string          `json:"user,omitempty"`
}

// EmbeddingResponse is the OpenAI-compatible embeddings response.
type EmbeddingResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  *Usage      `json:"usage,omitempty"`
}

// Embedding is a single embedding vector.
type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// ModelInfo represents a single model entry in the models list.
type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelListResponse is the response body for GET /v1/models.
type ModelListResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

// ErrorResponse is the OpenAI-compatible error response body.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error details.
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// ParsedModel holds the provider and model extracted from a model identifier.
// e.g. "anthropic/claude-sonnet-4-20250514" → {Provider:"anthropic", Model:"claude-sonnet-4-20250514"}
type ParsedModel struct {
	Provider string
	Model    string
}
