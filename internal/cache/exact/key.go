package exact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/llm-router/gateway/internal/gateway/types"
)

// cacheKeyData defines the fields that constitute a unique cache key.
// The stream field is intentionally excluded: a cached non-streaming response
// can be replayed as a streaming response.
type cacheKeyData struct {
	Model       string          `json:"model"`
	Messages    []types.Message `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
}

// BuildKey derives a deterministic SHA-256 cache key from the request.
// Returns an empty string if the request should not be cached.
func BuildKey(req *types.ChatCompletionRequest) string {
	kd := cacheKeyData{
		Model:    req.Model,
		Messages: req.Messages,
	}
	if req.Temperature != nil {
		kd.Temperature = *req.Temperature
	}
	if req.MaxTokens != nil {
		kd.MaxTokens = *req.MaxTokens
	}
	if req.TopP != nil {
		kd.TopP = *req.TopP
	}

	// Deterministic JSON: sort object keys by marshaling with sorted struct fields.
	// Go's json.Marshal already serialises struct fields in definition order,
	// which is deterministic. For the messages slice the caller order is preserved.
	data, err := json.Marshal(kd)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// IsCacheable returns true when the request can be cached.
// Requests are not cached if temperature > 0 (non-deterministic),
// if no max_tokens is set, or if the caller explicitly opts out.
func IsCacheable(req *types.ChatCompletionRequest, temperatureZeroOnly bool, headers map[string]string) bool {
	if headers["Cache-Control"] == "no-cache" || headers["X-Gateway-No-Cache"] == "true" {
		return false
	}
	if req.MaxTokens == nil {
		return false
	}
	if temperatureZeroOnly {
		if req.Temperature != nil && *req.Temperature > 0 {
			return false
		}
	}
	return true
}

// embeddingCacheKeyData defines the fields that constitute a unique embedding cache key.
type embeddingCacheKeyData struct {
	Model          string `json:"model"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Input          any    `json:"input"`
}

// BuildEmbeddingKey derives a deterministic SHA-256 cache key from an embedding request.
// The input is normalized by unmarshaling and re-marshaling to eliminate whitespace differences.
// Returns an empty string if the key cannot be built.
func BuildEmbeddingKey(req *types.EmbeddingRequest) string {
	var input any
	if err := json.Unmarshal(req.Input, &input); err != nil {
		return ""
	}
	kd := embeddingCacheKeyData{
		Model:          req.Model,
		EncodingFormat: req.EncodingFormat,
		Input:          input,
	}
	data, err := json.Marshal(kd)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// IsEmbeddingCacheable returns true when the embedding request can be cached.
// Embeddings are deterministic, so they are cached unless the caller opts out.
func IsEmbeddingCacheable(headers map[string]string) bool {
	return headers["Cache-Control"] != "no-cache" && headers["X-Gateway-No-Cache"] != "true"
}

// sortedKeys returns a sorted copy of the map keys (helper for deterministic output).
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// suppress unused import warning
var _ = sortedKeys
