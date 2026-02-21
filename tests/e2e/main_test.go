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
