package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SlackNotifier sends alerts to a Slack incoming webhook.
type SlackNotifier struct {
	name       string
	webhookURL string
	client     *http.Client
}

// NewSlackNotifier creates a SlackNotifier.
func NewSlackNotifier(name, webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		name:       name,
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (n *SlackNotifier) Name() string { return n.name }

func (n *SlackNotifier) Send(ctx context.Context, e *Event) error {
	emoji := severityEmoji(e.Severity)
	header := fmt.Sprintf("%s %s Alert: %s", emoji, capitalize(string(e.Severity)), e.Title)

	fields := make([]map[string]any, 0, len(e.Details))
	for k, v := range e.Details {
		fields = append(fields, map[string]any{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*%s:*\n%v", k, v),
		})
	}

	payload := map[string]any{
		"blocks": []map[string]any{
			{
				"type": "header",
				"text": map[string]any{"type": "plain_text", "text": header},
			},
			{
				"type":   "section",
				"fields": fields,
			},
			{
				"type": "context",
				"elements": []map[string]any{
					{"type": "mrkdwn", "text": fmt.Sprintf("*Event:* `%s` · *Time:* %s", e.EventType, e.Timestamp.Format(time.RFC3339))},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func severityEmoji(s Severity) string {
	switch s {
	case SeverityCritical:
		return "🚨"
	case SeverityWarning:
		return "⚠️"
	default:
		return "ℹ️"
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
