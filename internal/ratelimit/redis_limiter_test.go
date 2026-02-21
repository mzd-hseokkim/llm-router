package ratelimit_test

import (
	"testing"

	"github.com/llm-router/gateway/internal/ratelimit"
)

func TestConcurrencyLimiter(t *testing.T) {
	lim := ratelimit.NewConcurrencyLimiter(2)

	rel1, err := lim.Acquire(t.Context())
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	rel2, err := lim.Acquire(t.Context())
	if err != nil {
		t.Fatalf("second acquire failed: %v", err)
	}

	// Third acquire should fail immediately.
	_, err = lim.Acquire(t.Context())
	if err == nil {
		t.Fatal("expected error on third acquire, got nil")
	}

	// Release one slot and retry.
	rel1()
	rel3, err := lim.Acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	rel2()
	rel3()
}

func TestWindowMinute(t *testing.T) {
	w := ratelimit.WindowMinute()
	if len(w) != 12 {
		t.Errorf("WindowMinute() = %q, want 12-char string", w)
	}
}
