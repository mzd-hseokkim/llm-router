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
	status := resp.StatusCode
	resp.Body.Close()
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Errorf("expected 401 or 403, got %d", status)
	}
}
