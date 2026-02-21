package pii

import (
	"context"
	"testing"

	"github.com/llm-router/gateway/internal/guardrail"
)

func TestDetector_CreditCard(t *testing.T) {
	d := NewDetector(true, guardrail.ActionMask, nil)
	result, err := d.Check(context.Background(), "my card is 4111 1111 1111 1111", guardrail.DirectionInput)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered {
		t.Fatal("expected triggered")
	}
	if result.Category != "credit_card" {
		t.Errorf("expected credit_card, got %s", result.Category)
	}
	if result.Modified == "" || result.Modified == "my card is 4111 1111 1111 1111" {
		t.Errorf("expected masked text, got %q", result.Modified)
	}
}

func TestDetector_Email_Block(t *testing.T) {
	d := NewDetector(true, guardrail.ActionBlock, []string{"email"})
	result, err := d.Check(context.Background(), "contact user@example.com for help", guardrail.DirectionInput)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered {
		t.Fatal("expected triggered")
	}
	if result.Action != guardrail.ActionBlock {
		t.Errorf("expected block, got %s", result.Action)
	}
}

func TestDetector_NoMatch(t *testing.T) {
	d := NewDetector(true, guardrail.ActionMask, nil)
	result, err := d.Check(context.Background(), "hello world, how are you?", guardrail.DirectionInput)
	if err != nil {
		t.Fatal(err)
	}
	if result.Triggered {
		t.Fatal("expected not triggered")
	}
}

func TestDetector_Disabled(t *testing.T) {
	d := NewDetector(false, guardrail.ActionBlock, nil)
	result, err := d.Check(context.Background(), "card 4111-1111-1111-1111", guardrail.DirectionInput)
	if err != nil {
		t.Fatal(err)
	}
	if result.Triggered {
		t.Fatal("expected not triggered when disabled")
	}
}

func TestDetector_KoreanRRN(t *testing.T) {
	d := NewDetector(true, guardrail.ActionMask, []string{"korean_rrn"})
	result, err := d.Check(context.Background(), "주민번호: 901010-1234567", guardrail.DirectionInput)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered {
		t.Fatal("expected triggered for Korean RRN")
	}
}
