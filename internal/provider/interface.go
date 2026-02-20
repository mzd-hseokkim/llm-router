package provider

import (
	"context"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// StreamChunk is a single delta emitted during a streaming chat completion.
type StreamChunk struct {
	Delta        string       // incremental text content
	FinishReason *string      // non-nil on the final content chunk
	Usage        *types.Usage // non-nil when usage data is available (may follow FinishReason)
	Error        error        // non-nil if the stream encountered an error
}

// Provider is the interface all LLM provider adapters must implement.
type Provider interface {
	// Name returns the provider identifier (e.g. "openai", "anthropic").
	Name() string
	// Models returns the list of models this provider exposes.
	Models() []types.ModelInfo
	// ChatCompletion sends a non-streaming chat completion request.
	// rawBody is the original request JSON, available for providers that support
	// pass-through of unknown parameters.
	ChatCompletion(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (*types.ChatCompletionResponse, error)
	// ChatCompletionStream initiates a streaming chat completion.
	// It returns a channel that emits StreamChunks until the stream ends.
	// The channel is closed when the stream ends (normally or on error).
	ChatCompletionStream(ctx context.Context, model string, req *types.ChatCompletionRequest, rawBody []byte) (<-chan StreamChunk, error)
}

// Registry manages registered providers and supports lookup by name.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider to the registry, keyed by its name.
func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Get returns the provider for the given name, or false if not found.
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// AllModels returns the combined model list from all registered providers.
func (r *Registry) AllModels() []types.ModelInfo {
	var models []types.ModelInfo
	for _, p := range r.providers {
		models = append(models, p.Models()...)
	}
	return models
}
