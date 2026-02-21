//go:build e2e

package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Circuit Breaker 상태 조회 — 구조 검증
func TestCircuitBreaker_StateList(t *testing.T) {
	resp := client.AdminGet(t, "/admin/circuit-breakers")
	body := requireStatus(t, resp, 200)
	// 응답에 circuit_breakers, providers, states, data 키 중 하나가 있어야 함
	assert.True(t,
		body["circuit_breakers"] != nil || body["providers"] != nil ||
			body["states"] != nil || body["data"] != nil,
		"circuit breaker 상태 목록이 있어야 함 (got keys: %v)", mapKeys(body))
}

// 존재하지 않는 Provider의 Circuit Breaker 리셋 → 404 또는 200
func TestCircuitBreaker_Reset_UnknownProvider(t *testing.T) {
	resp := client.AdminPost(t, "/admin/circuit-breakers/nonexistent-provider-xyz/reset", nil)
	resp.Body.Close()
	assert.Contains(t, []int{200, 404}, resp.StatusCode)
}

// Rate Limit 리셋 — Virtual Key에 대한 리셋
func TestRateLimit_Reset(t *testing.T) {
	vk := createVirtualKey(t, "test-rl-reset")
	resp := client.AdminPost(t, "/admin/rate-limits/"+vk.ID+"/reset", nil)
	resp.Body.Close()
	// 리셋 성공 또는 not found (사용량이 없으면 404일 수 있음)
	assert.Contains(t, []int{200, 204, 404}, resp.StatusCode)
}

// Usage Top Spenders — 구조 검증
func TestUsage_TopSpenders(t *testing.T) {
	resp := client.AdminGet(t, "/admin/usage/top-spenders")
	body := requireStatus(t, resp, 200)
	require.NotNil(t, body)
}

