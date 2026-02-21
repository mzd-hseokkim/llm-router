//go:build e2e

package e2e_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth_Live(t *testing.T) {
	resp := client.NoAuthGet(t, "/health/live")
	body := requireStatus(t, resp, 200)
	assert.Equal(t, "ok", body["status"])
}

func TestHealth_Ready(t *testing.T) {
	resp := client.NoAuthGet(t, "/health/ready")
	body := requireStatus(t, resp, 200)
	// "ready" 또는 "ok" 모두 허용
	statusVal := fmt.Sprintf("%v", body["status"])
	assert.True(t, statusVal == "ok" || statusVal == "ready",
		"status should be 'ok' or 'ready', got: %s", statusVal)
	// DB, Redis 연결 상태 확인
	checks, ok := body["checks"].(map[string]any)
	require.True(t, ok, "checks 필드 존재해야 함")
	assert.Equal(t, "ok", checks["database"])
	assert.Equal(t, "ok", checks["redis"])
}

func TestHealth_Providers(t *testing.T) {
	resp := client.NoAuthGet(t, "/health/providers")
	requireStatus(t, resp, 200)
}

func TestMetrics_PrometheusFormat(t *testing.T) {
	resp := client.NoAuthGet(t, "/metrics")
	body := requireStatusRaw(t, resp, 200)
	assert.True(t, strings.Contains(string(body), "# HELP"), "Prometheus HELP 라인 존재해야 함")
}

func TestOpenAPI_JSON(t *testing.T) {
	resp := client.NoAuthGet(t, "/docs/openapi.json")
	body := requireStatus(t, resp, 200)
	assert.NotEmpty(t, body["openapi"], "openapi 버전 필드 존재해야 함")
}
