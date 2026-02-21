package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookNotifier sends a JSON payload to an arbitrary HTTP endpoint.
type WebhookNotifier struct {
	name    string
	url     string
	method  string
	headers map[string]string
	retry   int
	client  *http.Client
}

// NewWebhookNotifier creates a WebhookNotifier.
func NewWebhookNotifier(name, url, method string, headers map[string]string, retry int) *WebhookNotifier {
	if method == "" {
		method = http.MethodPost
	}
	if retry == 0 {
		retry = 3
	}
	return &WebhookNotifier{
		name:    name,
		url:     url,
		method:  method,
		headers: headers,
		retry:   retry,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *WebhookNotifier) Name() string { return n.name }

func (n *WebhookNotifier) Send(ctx context.Context, e *Event) error {
	payload := map[string]any{
		"event":     e.EventType,
		"severity":  string(e.Severity),
		"timestamp": e.Timestamp.Format(time.RFC3339),
		"details":   e.Details,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}

	var lastErr error
	wait := 500 * time.Millisecond
	for attempt := 0; attempt <= n.retry; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
				wait *= 2
			}
		}

		req, err := http.NewRequestWithContext(ctx, n.method, n.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("webhook: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range n.headers {
			req.Header.Set(k, v)
		}

		resp, err := n.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("webhook: attempt %d: %w", attempt+1, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("webhook: attempt %d: status %d", attempt+1, resp.StatusCode)
	}
	return lastErr
}
