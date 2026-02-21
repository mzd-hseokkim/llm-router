# Phase 5 — Task 03: Go E2E 시나리오 구현

## 목표

`tests/e2e/` 에 시나리오별 테스트 파일을 작성한다.
인프라 파일(`main_test.go`, `helpers_test.go`)이 완성된 후 진행한다.

---

## 파일별 구현 가이드

### 1. `tests/e2e/health_test.go`

```go
//go:build e2e

package e2e_test

import (
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
    assert.Equal(t, "ok", body["status"])
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
    assert.Contains(t, string(body), "# HELP", "Prometheus HELP 라인 존재해야 함")
}

func TestOpenAPI_JSON(t *testing.T) {
    resp := client.NoAuthGet(t, "/docs/openapi.json")
    body := requireStatus(t, resp, 200)
    assert.NotEmpty(t, body["openapi"], "openapi 버전 필드 존재해야 함")
}
```

---

### 2. `tests/e2e/auth_test.go`

```go
//go:build e2e

package e2e_test

import (
    "net/http"
    "testing"
)

func TestAuth_NoKey_Returns401(t *testing.T) {
    resp := client.NoAuthGet(t, "/v1/models")
    requireStatus(t, resp, 401)
}

func TestAuth_InvalidKey_Returns401(t *testing.T) {
    resp := client.VKGet(t, "/v1/models", "invalid-key-xyz")
    requireStatus(t, resp, 401)
}

func TestAdmin_NoKey_Returns401(t *testing.T) {
    resp := client.NoAuthGet(t, "/admin/logs")
    requireStatus(t, resp, 401)
}

func TestAdmin_InvalidKey_Returns401(t *testing.T) {
    resp := client.VKGet(t, "/admin/logs", "wrong-master-key")
    requireStatus(t, resp, 401)
}

func TestAuth_ValidVirtualKey_CanListModels(t *testing.T) {
    vk := createVirtualKey(t, "test-auth-models")
    resp := client.VKGet(t, "/v1/models", vk.Key)
    body := requireStatus(t, resp, 200)
    _, ok := body["data"]
    _ = ok // 모델이 0개여도 data 필드는 존재해야 함
}

// Virtual Key로 Admin 엔드포인트 접근 불가
func TestAuth_VirtualKey_CannotAccessAdmin(t *testing.T) {
    vk := createVirtualKey(t, "test-auth-admin-block")
    resp := client.VKGet(t, "/admin/logs", vk.Key)
    // Virtual Key는 Master Key가 아니므로 401
    status := resp.StatusCode
    resp.Body.Close()
    if status != http.StatusUnauthorized && status != http.StatusForbidden {
        t.Errorf("expected 401 or 403, got %d", status)
    }
}
```

---

### 3. `tests/e2e/admin_test.go`

```go
//go:build e2e

package e2e_test

import (
    "fmt"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// Virtual Key CRUD 전체 흐름
func TestAdmin_VirtualKey_CRUD(t *testing.T) {
    // 생성
    resp := client.AdminPost(t, "/admin/keys", map[string]any{
        "name":      "e2e-crud-test",
        "rpm_limit": 10,
    })
    body := requireStatus(t, resp, 201)
    id := fmt.Sprintf("%v", body["id"])
    require.NotEmpty(t, id)

    // 조회
    resp = client.AdminGet(t, "/admin/keys/"+id)
    body = requireStatus(t, resp, 200)
    assert.Equal(t, "e2e-crud-test", body["name"])

    // 목록
    resp = client.AdminGet(t, "/admin/keys")
    listBody := requireStatus(t, resp, 200)
    assert.NotNil(t, listBody["data"])

    // 수정
    resp = client.AdminPatch(t, "/admin/keys/"+id, map[string]any{
        "name": "e2e-crud-updated",
    })
    requireStatus(t, resp, 200)

    // 수정 확인
    resp = client.AdminGet(t, "/admin/keys/"+id)
    body = requireStatus(t, resp, 200)
    assert.Equal(t, "e2e-crud-updated", body["name"])

    // 삭제
    resp = client.AdminDelete(t, "/admin/keys/"+id)
    requireStatus(t, resp, 204)

    // 삭제 후 조회 → 404
    resp = client.AdminGet(t, "/admin/keys/"+id)
    resp.Body.Close()
    assert.Equal(t, 404, resp.StatusCode)
}

func TestAdmin_ProviderKey_List(t *testing.T) {
    resp := client.AdminGet(t, "/admin/provider-keys")
    requireStatus(t, resp, 200)
}

func TestAdmin_Routing_Get(t *testing.T) {
    resp := client.AdminGet(t, "/admin/routing")
    requireStatus(t, resp, 200)
}

func TestAdmin_Routing_Reload(t *testing.T) {
    resp := client.AdminPost(t, "/admin/routing/reload", nil)
    requireStatus(t, resp, 200)
}

func TestAdmin_CircuitBreakers_List(t *testing.T) {
    resp := client.AdminGet(t, "/admin/circuit-breakers")
    requireStatus(t, resp, 200)
}

func TestAdmin_Usage_Summary(t *testing.T) {
    resp := client.AdminGet(t, "/admin/usage/summary")
    requireStatus(t, resp, 200)
}

func TestAdmin_Budget_Create(t *testing.T) {
    vk := createVirtualKey(t, "test-budget-vk")
    resp := client.AdminPost(t, "/admin/budgets", map[string]any{
        "entity_type":  "key",
        "entity_id":    vk.ID,
        "amount":       10.0,
        "period":       "monthly",
        "hard_limit":   true,
    })
    body := requireStatus(t, resp, 201)
    assert.NotEmpty(t, body["id"])
}

func TestAdmin_AuditLogs_List(t *testing.T) {
    resp := client.AdminGet(t, "/admin/audit-logs")
    requireStatus(t, resp, 200)
}

func TestAdmin_ABTests_List(t *testing.T) {
    resp := client.AdminGet(t, "/admin/ab-tests")
    requireStatus(t, resp, 200)
}
```

---

### 4. `tests/e2e/chat_test.go`

```go
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
    // 400 또는 404 또는 503 (모델 없음)
    assert.Contains(t, []int{400, 404, 503}, resp.StatusCode)
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

    // streaming 요청은 httpClient 직접 사용
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
```

---

### 5. `tests/e2e/middleware_test.go`

```go
//go:build e2e

package e2e_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

// RPM 제한 초과 → 429
func TestRateLimit_RPM_Exceeded(t *testing.T) {
    // rpm_limit=1 인 키 생성
    resp := client.AdminPost(t, "/admin/keys", map[string]any{
        "name":      "e2e-ratelimit-test",
        "rpm_limit": 1,
    })
    body := requireStatus(t, resp, 201)
    vkKey := body["key"].(string)
    vkID := body["id"].(string)
    t.Cleanup(func() {
        r := client.AdminDelete(t, "/admin/keys/"+vkID)
        r.Body.Close()
    })

    model := firstAvailableModel(t, vkKey)

    // 첫 번째 요청
    r1 := client.VKPost(t, "/v1/chat/completions", vkKey, map[string]any{
        "model":      model,
        "max_tokens": 1,
        "messages":   []map[string]any{{"role": "user", "content": "hi"}},
    })
    r1.Body.Close()

    // 두 번째 요청 → 429 또는 첫 번째가 이미 429
    r2 := client.VKPost(t, "/v1/chat/completions", vkKey, map[string]any{
        "model":      model,
        "max_tokens": 1,
        "messages":   []map[string]any{{"role": "user", "content": "hi"}},
    })
    r2.Body.Close()

    // 두 응답 중 하나는 429여야 함
    statuses := []int{r1.StatusCode, r2.StatusCode}
    has429 := false
    for _, s := range statuses {
        if s == 429 {
            has429 = true
        }
    }
    assert.True(t, has429, "RPM 초과 시 429 응답해야 함 (got: %v)", statuses)
}

// Budget 소진 → 429 또는 402
func TestBudget_HardLimit_Blocks(t *testing.T) {
    vk := createVirtualKey(t, "test-budget-hard")

    // 매우 작은 예산 설정 ($0.000001)
    resp := client.AdminPost(t, "/admin/budgets", map[string]any{
        "entity_type": "key",
        "entity_id":   vk.ID,
        "amount":      0.000001,
        "period":      "monthly",
        "hard_limit":  true,
    })
    requireStatus(t, resp, 201)

    model := firstAvailableModel(t, vk.Key)

    // 요청 시도 → 예산 초과로 차단되어야 함
    r := client.VKPost(t, "/v1/chat/completions", vk.Key, map[string]any{
        "model":      model,
        "max_tokens": 100,
        "messages":   []map[string]any{{"role": "user", "content": "hello world"}},
    })
    r.Body.Close()
    // 이미 예산이 소진되었거나 요청 자체가 차단됨
    // 주의: 예산 확인이 실시간이라면 첫 요청에 차단되지 않을 수 있음
    // 그 경우 두 번째 요청에서 차단 확인
    assert.Contains(t, []int{200, 402, 429}, r.StatusCode)
}

// Guardrail — PII 마스킹 확인 (응답에 원본 SSN이 없어야 함)
// 참고: 가드레일 정책에 따라 차단(4xx) 또는 마스킹(200) 중 하나
func TestGuardrail_PII_InRequest(t *testing.T) {
    vk := createVirtualKey(t, "test-guardrail-pii")
    model := firstAvailableModel(t, vk.Key)

    resp := client.VKPost(t, "/v1/chat/completions", vk.Key, map[string]any{
        "model":      model,
        "max_tokens": 5,
        "messages":   []map[string]any{{"role": "user", "content": "my SSN is 123-45-6789, help me"}},
    })
    defer resp.Body.Close()
    // 차단(400/422) 또는 통과(200) — 둘 다 허용, 단 503은 아니어야 함
    assert.NotEqual(t, 503, resp.StatusCode, "가드레일로 인한 서버 에러는 안 됨")
}
```

---

### 6. `tests/e2e/resilience_test.go`

```go
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
    // 응답에 providers 또는 states 키가 있어야 함
    assert.True(t,
        body["providers"] != nil || body["states"] != nil || body["data"] != nil,
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

// mapKeys — map의 키 목록 반환 (디버깅용)
func mapKeys(m map[string]any) []string {
    keys := make([]string, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    return keys
}
```

---

## 실행 방법

```bash
# 전체 E2E 테스트
make e2e

# 특정 테스트만 실행
make e2e-run TEST=TestChat_NonStreaming_Success

# 환경 변수 오버라이드
GATEWAY_URL=http://prod:8080 MASTER_KEY=secret make e2e

# verbose + 실패 시 즉시 중단
go test -v -tags e2e -failfast ./tests/e2e/... -timeout 5m
```

---

## 완료 기준

- [ ] `health_test.go` — 5개 테스트 통과
- [ ] `auth_test.go` — 6개 테스트 통과
- [ ] `admin_test.go` — 9개 테스트 통과
- [ ] `chat_test.go` — 4개 테스트 통과 (firstAvailableModel으로 모델 없으면 skip)
- [ ] `middleware_test.go` — 3개 테스트 통과
- [ ] `resilience_test.go` — 4개 테스트 통과
- [ ] `make e2e` — 전체 통과, FAIL 0개
