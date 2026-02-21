package alerting

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

// EmailNotifier sends alert emails via SMTP.
type EmailNotifier struct {
	name     string
	smtpHost string
	smtpPort int
	from     string
	to       []string
}

// NewEmailNotifier creates an EmailNotifier.
func NewEmailNotifier(name, smtpHost string, smtpPort int, from string, to []string) *EmailNotifier {
	return &EmailNotifier{
		name:     name,
		smtpHost: smtpHost,
		smtpPort: smtpPort,
		from:     from,
		to:       to,
	}
}

func (n *EmailNotifier) Name() string { return n.name }

func (n *EmailNotifier) Send(_ context.Context, e *Event) error {
	subject := fmt.Sprintf("[%s] %s - %s", strings.ToUpper(string(e.Severity)), e.EventType, e.Title)

	var body strings.Builder
	body.WriteString("Alert Details:\n\n")
	body.WriteString(fmt.Sprintf("Event:     %s\n", e.EventType))
	body.WriteString(fmt.Sprintf("Severity:  %s\n", e.Severity))
	body.WriteString(fmt.Sprintf("Timestamp: %s\n", e.Timestamp.Format(time.RFC3339)))
	if len(e.Details) > 0 {
		body.WriteString("\nDetails:\n")
		for k, v := range e.Details {
			body.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		n.from,
		strings.Join(n.to, ", "),
		subject,
		body.String(),
	)

	addr := fmt.Sprintf("%s:%d", n.smtpHost, n.smtpPort)
	if err := smtp.SendMail(addr, nil, n.from, n.to, []byte(msg)); err != nil {
		return fmt.Errorf("email: send: %w", err)
	}
	return nil
}
