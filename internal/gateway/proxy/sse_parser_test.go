package proxy

import (
	"strings"
	"testing"
)

func collectEvents(input string) []SSEEvent {
	var events []SSEEvent
	for e := range ParseSSE(strings.NewReader(input)) {
		events = append(events, e)
	}
	return events
}

func TestParseSSE_SimpleDataEvent(t *testing.T) {
	input := "data: hello\n\n"
	events := collectEvents(input)

	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Data != "hello" {
		t.Errorf("want data %q, got %q", "hello", events[0].Data)
	}
	if events[0].Type != "message" {
		t.Errorf("want type %q, got %q", "message", events[0].Type)
	}
}

func TestParseSSE_EventType(t *testing.T) {
	input := "event: content_block_delta\ndata: {\"text\":\"hello\"}\n\n"
	events := collectEvents(input)

	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Type != "content_block_delta" {
		t.Errorf("want type %q, got %q", "content_block_delta", events[0].Type)
	}
	if events[0].Data != `{"text":"hello"}` {
		t.Errorf("want data %q, got %q", `{"text":"hello"}`, events[0].Data)
	}
}

func TestParseSSE_MultipleEvents(t *testing.T) {
	input := "data: first\n\ndata: second\n\ndata: [DONE]\n\n"
	events := collectEvents(input)

	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d", len(events))
	}
	if events[0].Data != "first" || events[1].Data != "second" || events[2].Data != "[DONE]" {
		t.Errorf("unexpected event data: %v", events)
	}
}

func TestParseSSE_MultilineData(t *testing.T) {
	input := "data: line1\ndata: line2\n\n"
	events := collectEvents(input)

	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Data != "line1\nline2" {
		t.Errorf("want %q, got %q", "line1\nline2", events[0].Data)
	}
}

func TestParseSSE_IDField(t *testing.T) {
	input := "id: 42\ndata: payload\n\n"
	events := collectEvents(input)

	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].ID != "42" {
		t.Errorf("want ID %q, got %q", "42", events[0].ID)
	}
}

func TestParseSSE_EmptyDataLinesDropped(t *testing.T) {
	// A blank line with no preceding data lines should not dispatch an event.
	input := "\n\ndata: real\n\n"
	events := collectEvents(input)

	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
}

func TestParseSSE_CommentLinesIgnored(t *testing.T) {
	input := ": this is a comment\ndata: value\n\n"
	events := collectEvents(input)

	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Data != "value" {
		t.Errorf("want %q, got %q", "value", events[0].Data)
	}
}

func TestParseSSE_DataWithNoSpace(t *testing.T) {
	// "data:value" (no space after colon) is valid per spec.
	input := "data:value\n\n"
	events := collectEvents(input)

	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if events[0].Data != "value" {
		t.Errorf("want %q, got %q", "value", events[0].Data)
	}
}

func TestParseSSE_OpenAIStyleStream(t *testing.T) {
	// Simulate a typical OpenAI SSE stream.
	input := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
		``,
	}, "\n")

	events := collectEvents(input)
	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d: %v", len(events), events)
	}
	if events[2].Data != "[DONE]" {
		t.Errorf("last event should be [DONE], got %q", events[2].Data)
	}
}
