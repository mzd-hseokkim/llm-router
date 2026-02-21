# Phase 5 — Task 02: Go E2E 테스트 인프라

## 목표

`tests/e2e/` 패키지를 생성한다. 실행 중인 서버에 연결하는 방식으로,
별도 인프라 시작 없이 시나리오별 E2E 테스트를 구조적으로 작성할 수 있게 한다.

---

## 전제 조건

- 서버 실행 중 (`http://localhost:8080`)
- `go test -tags e2e` 빌드 태그 사용
- 외부 의존성 추가 없음 (`testify` 이미 존재)

---

## 파일 구조

```
tests/
└── e2e/
    ├── main_test.go        # TestMain — 서버 연결 확인, 글로벌 클라이언트 초기화
    └── helpers_test.go     # API 클라이언트, 공통 어설션, Virtual Key 픽스처
```

시나리오 파일은 Task 03에서 작성한다.

---

## 구현 상세

### `tests/e2e/main_test.go`

```go
//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

var (
	gatewayURL string
	masterKey  string
	client     *APIClient
)

func TestMain(m *testing.M) {
	gatewayURL = envOrDefault("GATEWAY_URL", "http://localhost:8080")
	masterKey = envOrDefault("MASTER_KEY", "admin123")

	if err := waitForServer(gatewayURL, 10*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "서버에 연결할 수 없습니다 (%s): %v\n", gatewayURL, err)
		fmt.Fprintf(os.Stderr, "서버를 먼저 시작하세요: make run\n")
		os.Exit(1)
	}

	client = newAPIClient(gatewayURL, masterKey)
	os.Exit(m.Run())
}

func waitForServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/health/live")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout after %s", timeout)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

---

### `tests/e2e/helpers_test.go`

**APIClient 구조체**

```go
//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// APIClient — 테스트용 HTTP 클라이언트 래퍼
type APIClient struct {
	baseURL    string
	masterKey  string
	httpClient *http.Client
}

func newAPIClient(baseURL, masterKey string) *APIClient {
	return &APIClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		masterKey: masterKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// AdminGet — Master Key로 GET 요청
func (c *APIClient) AdminGet(t *testing.T, path string) *http.Response {
	t.Helper()
	return c.do(t, "GET", path, c.masterKey, nil)
}

// AdminPost — Master Key로 POST 요청
func (c *APIClient) AdminPost(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	return c.do(t, "POST", path, c.masterKey, body)
}

// AdminPatch — Master Key로 PATCH 요청
func (c *APIClient) AdminPatch(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	return c.do(t, "PATCH", path, c.masterKey, body)
}

// AdminDelete — Master Key로 DELETE 요청
func (c *APIClient) AdminDelete(t *testing.T, path string) *http.Response {
	t.Helper()
	return c.do(t, "DELETE", path, c.masterKey, nil)
}

// VKGet — Virtual Key로 GET 요청
func (c *APIClient) VKGet(t *testing.T, path, vk string) *http.Response {
	t.Helper()
	return c.do(t, "GET", path, vk, nil)
}

// VKPost — Virtual Key로 POST 요청
func (c *APIClient) VKPost(t *testing.T, path, vk string, body any) *http.Response {
	t.Helper()
	return c.do(t, "POST", path, vk, body)
}

// NoAuthGet — 인증 없이 GET 요청
func (c *APIClient) NoAuthGet(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	require.NoError(t, err)
	resp, err := c.httpClient.Do(req)
	require.NoError(t, err)
	return resp
}

func (c *APIClient) do(t *testing.T, method, path, token string, body any) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	require.NoError(t, err)
	return resp
}
```

**어설션 헬퍼**

```go
// requireStatus — HTTP 상태 코드 검증 + body 파싱
func requireStatus(t *testing.T, resp *http.Response, expected int) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, expected, resp.StatusCode,
		"path=%s body=%s", resp.Request.URL.Path, string(b))
	if len(b) == 0 {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m) // 파싱 실패해도 패닉하지 않음
	return m
}

// requireStatusRaw — 상태 코드 검증 후 raw body 반환
func requireStatusRaw(t *testing.T, resp *http.Response, expected int) []byte {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, expected, resp.StatusCode,
		"path=%s body=%s", resp.Request.URL.Path, string(b))
	return b
}
```

**Virtual Key 픽스처**

```go
// VirtualKeyFixture — 테스트용 Virtual Key를 생성하고 t.Cleanup으로 자동 삭제
type VirtualKeyFixture struct {
	ID  string
	Key string
}

func createVirtualKey(t *testing.T, name string) VirtualKeyFixture {
	t.Helper()
	resp := client.AdminPost(t, "/admin/keys", map[string]any{
		"name":      name,
		"rpm_limit": 60,
		"tpm_limit": 100000,
	})
	body := requireStatus(t, resp, 201)
	id := fmt.Sprintf("%v", body["id"])
	key := fmt.Sprintf("%v", body["key"])
	require.NotEmpty(t, id)
	require.NotEmpty(t, key)

	t.Cleanup(func() {
		resp := client.AdminDelete(t, "/admin/keys/"+id)
		resp.Body.Close()
	})
	return VirtualKeyFixture{ID: id, Key: key}
}
```

**모델 조회 헬퍼**

```go
// firstAvailableModel — /v1/models 에서 첫 번째 모델 ID 반환
// 모델이 없으면 t.Skip()
func firstAvailableModel(t *testing.T, vk string) string {
	t.Helper()
	resp := client.VKGet(t, "/v1/models", vk)
	body := requireStatus(t, resp, 200)
	data, ok := body["data"].([]any)
	if !ok || len(data) == 0 {
		t.Skip("사용 가능한 모델 없음 — API 키 확인 필요")
	}
	first := data[0].(map[string]any)
	return fmt.Sprintf("%v", first["id"])
}
```

---

## Makefile 타겟 추가

기존 Makefile에 다음을 추가한다:

```makefile
.PHONY: e2e-smoke e2e

e2e-smoke:
	bash scripts/e2e_smoke.sh

e2e:
	go test -v -tags e2e ./tests/e2e/... -timeout 5m

e2e-run:
	GATEWAY_URL=$(GATEWAY_URL) MASTER_KEY=$(MASTER_KEY) \
	go test -v -tags e2e ./tests/e2e/... -timeout 5m -run $(TEST)
```

---

## 완료 기준

- [ ] `tests/e2e/main_test.go` — TestMain, waitForServer, envOrDefault
- [ ] `tests/e2e/helpers_test.go` — APIClient, requireStatus, VirtualKeyFixture, firstAvailableModel
- [ ] `go build -tags e2e ./tests/e2e/...` 컴파일 성공
- [ ] Makefile에 `e2e-smoke`, `e2e` 타겟 추가
