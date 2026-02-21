package exact

import (
	"testing"

	"github.com/llm-router/gateway/internal/gateway/types"
)

func ptr[T any](v T) *T { return &v }

func TestBuildKey_Deterministic(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Model:     "openai/gpt-4o",
		Messages:  []types.Message{{Role: "user", Content: "hello"}},
		MaxTokens: ptr(100),
	}
	k1 := BuildKey(req)
	k2 := BuildKey(req)
	if k1 == "" || k1 != k2 {
		t.Errorf("BuildKey is not deterministic: %q vs %q", k1, k2)
	}
}

func TestBuildKey_DifferentModel(t *testing.T) {
	base := &types.ChatCompletionRequest{
		Model:     "openai/gpt-4o",
		Messages:  []types.Message{{Role: "user", Content: "hello"}},
		MaxTokens: ptr(100),
	}
	other := &types.ChatCompletionRequest{
		Model:     "anthropic/claude-3-5-sonnet",
		Messages:  []types.Message{{Role: "user", Content: "hello"}},
		MaxTokens: ptr(100),
	}
	if BuildKey(base) == BuildKey(other) {
		t.Error("different models should produce different keys")
	}
}

func TestBuildKey_DifferentMessages(t *testing.T) {
	a := &types.ChatCompletionRequest{
		Model:     "openai/gpt-4o",
		Messages:  []types.Message{{Role: "user", Content: "hello"}},
		MaxTokens: ptr(100),
	}
	b := &types.ChatCompletionRequest{
		Model:     "openai/gpt-4o",
		Messages:  []types.Message{{Role: "user", Content: "world"}},
		MaxTokens: ptr(100),
	}
	if BuildKey(a) == BuildKey(b) {
		t.Error("different messages should produce different keys")
	}
}

func TestIsCacheable(t *testing.T) {
	mt := ptr(100)
	t.Run("no max_tokens", func(t *testing.T) {
		req := &types.ChatCompletionRequest{Model: "m", Messages: nil}
		if IsCacheable(req, true, nil) {
			t.Error("should not cache without max_tokens")
		}
	})
	t.Run("temperature>0 with zero-only mode", func(t *testing.T) {
		req := &types.ChatCompletionRequest{Model: "m", MaxTokens: mt, Temperature: ptr(0.5)}
		if IsCacheable(req, true, nil) {
			t.Error("should not cache temperature>0 in zero-only mode")
		}
	})
	t.Run("temperature=0 is cacheable", func(t *testing.T) {
		req := &types.ChatCompletionRequest{Model: "m", MaxTokens: mt, Temperature: ptr(0.0)}
		if !IsCacheable(req, true, nil) {
			t.Error("temperature=0 should be cacheable")
		}
	})
	t.Run("no-cache header", func(t *testing.T) {
		req := &types.ChatCompletionRequest{Model: "m", MaxTokens: mt}
		if IsCacheable(req, false, map[string]string{"Cache-Control": "no-cache"}) {
			t.Error("should not cache with no-cache header")
		}
	})
}
