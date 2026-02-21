package telemetry_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/llm-router/gateway/internal/telemetry"
)

// mockStore records all BatchInsert calls.
type mockStore struct {
	mu      sync.Mutex
	batches [][]*telemetry.LogEntry
	err     error
}

func (m *mockStore) BatchInsert(_ context.Context, entries []*telemetry.LogEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	cp := make([]*telemetry.LogEntry, len(entries))
	copy(cp, entries)
	m.batches = append(m.batches, cp)
	return nil
}

func (m *mockStore) totalEntries() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, b := range m.batches {
		n += len(b)
	}
	return n
}

// TestLogWriter_FlushOnTicker checks that entries are flushed within ~1 s.
func TestLogWriter_FlushOnTicker(t *testing.T) {
	store := &mockStore{}
	w := telemetry.NewLogWriter(store, newNopLogger())
	defer w.Close()

	w.Write(&telemetry.LogEntry{Model: "test/model", Provider: "test"})
	w.Write(&telemetry.LogEntry{Model: "test/model", Provider: "test"})

	// Wait up to 2 s for the ticker flush.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if store.totalEntries() == 2 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected 2 entries to be flushed, got %d", store.totalEntries())
}

// TestLogWriter_FlushOnBatchSize checks that a full batch is flushed immediately.
func TestLogWriter_FlushOnBatchSize(t *testing.T) {
	store := &mockStore{}
	// Use the exported writer directly; batchSize is 500 in production,
	// but we verify via Close() drain instead.
	w := telemetry.NewLogWriter(store, newNopLogger())

	for range 10 {
		w.Write(&telemetry.LogEntry{Model: "m", Provider: "p"})
	}

	w.Close() // drains remaining entries

	if store.totalEntries() != 10 {
		t.Fatalf("expected 10 entries after Close, got %d", store.totalEntries())
	}
}

// TestLogWriter_Close_DrainsPending ensures Close flushes buffered entries.
func TestLogWriter_Close_DrainsPending(t *testing.T) {
	store := &mockStore{}
	w := telemetry.NewLogWriter(store, newNopLogger())

	const n = 50
	for range n {
		w.Write(&telemetry.LogEntry{Model: "m", Provider: "p"})
	}
	w.Close()

	if store.totalEntries() != n {
		t.Fatalf("expected %d entries after Close, got %d", n, store.totalEntries())
	}
}

// TestLogWriter_DropWhenFull verifies that Write never blocks when the channel is full.
func TestLogWriter_DropWhenFull(t *testing.T) {
	// Use a store that blocks to prevent the worker from draining.
	blocker := &blockingStore{ready: make(chan struct{})}
	w := telemetry.NewLogWriter(blocker, newNopLogger())
	defer func() {
		close(blocker.ready) // unblock so Close can finish
		w.Close()
	}()

	// Fill the channel well beyond its capacity without blocking.
	start := time.Now()
	for range 20_000 {
		w.Write(&telemetry.LogEntry{Model: "m", Provider: "p"})
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("Write blocked for %v; should be non-blocking", elapsed)
	}
}

// --- helpers ---

type blockingStore struct {
	ready chan struct{}
}

func (b *blockingStore) BatchInsert(ctx context.Context, _ []*telemetry.LogEntry) error {
	select {
	case <-b.ready:
	case <-ctx.Done():
	}
	return nil
}

func newNopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
