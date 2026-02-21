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
		r := client.AdminDelete(t, "/admin/keys/"+id)
		r.Body.Close()
	})
	return VirtualKeyFixture{ID: id, Key: key}
}

// firstAvailableModel — /v1/models 에서 첫 번째 모델 ID 반환 (Anthropic 우선)
// 모델이 없으면 t.Skip()
func firstAvailableModel(t *testing.T, vk string) string {
	t.Helper()
	resp := client.VKGet(t, "/v1/models", vk)
	body := requireStatus(t, resp, 200)
	data, ok := body["data"].([]any)
	if !ok || len(data) == 0 {
		t.Skip("사용 가능한 모델 없음 — API 키 확인 필요")
	}
	// Anthropic 모델 우선 선택
	for _, item := range data {
		m := item.(map[string]any)
		id := fmt.Sprintf("%v", m["id"])
		if strings.Contains(id, "anthropic") {
			return id
		}
	}
	first := data[0].(map[string]any)
	return fmt.Sprintf("%v", first["id"])
}

// mapKeys — map의 키 목록 반환 (디버깅용)
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
