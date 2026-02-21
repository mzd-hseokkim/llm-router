//go:build e2e

package e2e_test

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChat_InvalidPayload_Returns400(t *testing.T) {
	vk := createVirtualKey(t, "test-chat-400")
	// messages 필드 없는 잘못된 요청
	resp := client.VKPost(t, "/v1/chat/completions", vk.Key, map[string]any{
		"model": "some-model",
		// messages 누락
	})
	resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
}

func TestChat_UnknownModel_ReturnsError(t *testing.T) {
	vk := createVirtualKey(t, "test-chat-unknown-model")
	resp := client.VKPost(t, "/v1/chat/completions", vk.Key, map[string]any{
		"model":    "nonexistent-model-xyz",
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})
	resp.Body.Close()
	// 400/404/401/503 — 알 수 없는 모델이므로 4xx/5xx 중 하나
	assert.NotEqual(t, 200, resp.StatusCode, "존재하지 않는 모델은 성공하면 안 됨")
}

// 실제 LLM 호출 — Anthropic API 필요
func TestChat_NonStreaming_Success(t *testing.T) {
	vk := createVirtualKey(t, "test-chat-nstream")
	model := firstAvailableModel(t, vk.Key)

	resp := client.VKPost(t, "/v1/chat/completions", vk.Key, map[string]any{
		"model":      model,
		"max_tokens": 5,
		"messages":   []map[string]any{{"role": "user", "content": "say hi"}},
	})
	body := requireStatus(t, resp, 200)

	choices, ok := body["choices"].([]any)
	require.True(t, ok, "choices 배열 존재해야 함")
	require.Greater(t, len(choices), 0)

	msg := choices[0].(map[string]any)["message"].(map[string]any)
	content := msg["content"].(string)
	assert.NotEmpty(t, content)

	// usage 필드 확인
	usage, ok := body["usage"].(map[string]any)
	require.True(t, ok, "usage 필드 존재해야 함")
	assert.Greater(t, usage["total_tokens"].(float64), 0.0)
}

// SSE 스트리밍 — data: chunk 순서 검증
func TestChat_Streaming_SSE(t *testing.T) {
	vk := createVirtualKey(t, "test-chat-stream")
	model := firstAvailableModel(t, vk.Key)

	resp := client.VKPost(t, "/v1/chat/completions", vk.Key, map[string]any{
		"model":      model,
		"max_tokens": 10,
		"stream":     true,
		"messages":   []map[string]any{{"role": "user", "content": "count 1 2 3"}},
	})
	require.Equal(t, 200, resp.StatusCode)
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	assert.Contains(t, contentType, "text/event-stream")

	// SSE chunk 읽기
	scanner := bufio.NewScanner(resp.Body)
	chunkCount := 0
	doneSeen := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			doneSeen = true
			break
		}
		var chunk map[string]any
		err := json.Unmarshal([]byte(data), &chunk)
		require.NoError(t, err, "SSE chunk가 유효한 JSON이어야 함")
		chunkCount++
	}
	assert.True(t, doneSeen, "[DONE] 마커 수신해야 함")
	assert.Greater(t, chunkCount, 0, "최소 1개 이상의 chunk 수신해야 함")
}
