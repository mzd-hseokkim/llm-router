package circuitbreaker

import (
	"testing"
	"time"
)

func TestCircuitBreaker_InitiallyClosed(t *testing.T) {
	cb := New(DefaultConfig())
	if cb.IsOpen("openai") {
		t.Error("circuit should be closed initially")
	}
	if cb.ProviderStatus("openai") != "closed" {
		t.Errorf("status = %q, want %q", cb.ProviderStatus("openai"), "closed")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FailureThreshold = 3
	cb := New(cfg)

	for i := 0; i < 2; i++ {
		cb.RecordFailure("openai")
		if cb.IsOpen("openai") {
			t.Errorf("should not open after %d failures (threshold 3)", i+1)
		}
	}

	cb.RecordFailure("openai") // 3rd failure → open
	if !cb.IsOpen("openai") {
		t.Error("circuit should be open after 3 failures")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cfg := Config{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		OpenTimeout:      10 * time.Millisecond,
	}
	cb := New(cfg)

	cb.RecordFailure("openai") // open
	if !cb.IsOpen("openai") {
		t.Fatal("circuit should be open after 1 failure")
	}

	time.Sleep(20 * time.Millisecond) // wait for timeout
	if cb.IsOpen("openai") {
		t.Error("circuit should allow probe (HalfOpen) after timeout")
	}
	if cb.ProviderStatus("openai") != "half_open" {
		t.Errorf("status = %q, want half_open", cb.ProviderStatus("openai"))
	}
}

func TestCircuitBreaker_RecoverAfterSuccesses(t *testing.T) {
	cfg := Config{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		OpenTimeout:      10 * time.Millisecond,
	}
	cb := New(cfg)

	cb.RecordFailure("openai")
	time.Sleep(20 * time.Millisecond)
	cb.IsOpen("openai") // triggers HalfOpen transition

	cb.RecordSuccess("openai")
	if cb.ProviderStatus("openai") != "half_open" {
		t.Errorf("status = %q, want half_open after 1 success (threshold 2)", cb.ProviderStatus("openai"))
	}

	cb.RecordSuccess("openai") // 2nd success → Closed
	if cb.IsOpen("openai") {
		t.Error("circuit should be closed after 2 successes in HalfOpen")
	}
	if cb.ProviderStatus("openai") != "closed" {
		t.Errorf("status = %q, want closed", cb.ProviderStatus("openai"))
	}
}

func TestCircuitBreaker_FailureInHalfOpenReopens(t *testing.T) {
	cfg := Config{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		OpenTimeout:      10 * time.Millisecond,
	}
	cb := New(cfg)

	cb.RecordFailure("openai")
	time.Sleep(20 * time.Millisecond)
	cb.IsOpen("openai") // → HalfOpen

	cb.RecordFailure("openai") // failure in HalfOpen → re-Open
	if !cb.IsOpen("openai") {
		t.Error("circuit should re-open after failure in HalfOpen")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cfg := Config{FailureThreshold: 1, SuccessThreshold: 2, OpenTimeout: time.Minute}
	cb := New(cfg)

	cb.RecordFailure("openai")
	if !cb.IsOpen("openai") {
		t.Fatal("circuit should be open")
	}

	cb.Reset("openai")
	if cb.IsOpen("openai") {
		t.Error("circuit should be closed after Reset")
	}
	if cb.ProviderStatus("openai") != "closed" {
		t.Errorf("status = %q, want closed after reset", cb.ProviderStatus("openai"))
	}
}

func TestCircuitBreaker_AllStatus(t *testing.T) {
	cb := New(DefaultConfig())
	cb.RecordFailure("openai")
	cb.RecordFailure("anthropic")

	status := cb.AllStatus()
	if _, ok := status["openai"]; !ok {
		t.Error("openai should appear in AllStatus")
	}
	if _, ok := status["anthropic"]; !ok {
		t.Error("anthropic should appear in AllStatus")
	}
}

func TestCircuitBreaker_SuccessResetsClosed(t *testing.T) {
	cb := New(DefaultConfig())

	// Record some failures (below threshold).
	cb.RecordFailure("openai")
	cb.RecordFailure("openai")

	// A success should reset the failure count.
	cb.RecordSuccess("openai")
	cb.RecordFailure("openai") // this is now first failure again

	// Should still be closed (threshold 5, only 1 failure after reset).
	if cb.IsOpen("openai") {
		t.Error("circuit should remain closed after success reset")
	}
}
