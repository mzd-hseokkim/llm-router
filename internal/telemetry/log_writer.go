package telemetry

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	channelSize    = 10_000
	batchSize      = 500
	flushInterval  = time.Second
	flushTimeout   = 10 * time.Second
)

// LogStoreWriter is the persistence interface consumed by LogWriter.
type LogStoreWriter interface {
	BatchInsert(ctx context.Context, entries []*LogEntry) error
}

// LogWriter buffers LogEntry values and flushes them to the store in batches.
// Logging failures are recorded in the structured log but never propagate to
// the request path. If the internal channel is full, new entries are dropped
// and counted.
type LogWriter struct {
	store   LogStoreWriter
	logger  *slog.Logger
	ch      chan *LogEntry
	stop    chan struct{}
	wg      sync.WaitGroup
	dropped atomic.Int64
}

// NewLogWriter creates a LogWriter and starts its background flush goroutine.
func NewLogWriter(store LogStoreWriter, logger *slog.Logger) *LogWriter {
	w := &LogWriter{
		store:  store,
		logger: logger,
		ch:     make(chan *LogEntry, channelSize),
		stop:   make(chan struct{}),
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// Write enqueues an entry. The call never blocks; if the buffer is full the
// entry is silently dropped and the drop counter is incremented.
func (w *LogWriter) Write(entry *LogEntry) {
	select {
	case w.ch <- entry:
	default:
		n := w.dropped.Add(1)
		w.logger.Warn("log buffer full; entry dropped", "total_dropped", n)
	}
}

// Close signals the background goroutine to stop, drains any buffered entries,
// and waits for the flush to complete.
func (w *LogWriter) Close() {
	close(w.stop)
	w.wg.Wait()
}

// run is the background flush loop.
func (w *LogWriter) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	buf := make([]*LogEntry, 0, batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		defer cancel()
		if err := w.store.BatchInsert(ctx, buf); err != nil {
			w.logger.Error("log batch insert failed", "count", len(buf), "error", err)
		}
		buf = buf[:0]
	}

	for {
		select {
		case <-w.stop:
			// Drain remaining entries from the channel before exiting.
			for {
				select {
				case e := <-w.ch:
					buf = append(buf, e)
					if len(buf) >= batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}

		case <-ticker.C:
			flush()

		case e := <-w.ch:
			buf = append(buf, e)
			if len(buf) >= batchSize {
				flush()
			}
		}
	}
}
