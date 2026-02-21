package budget

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Scheduler periodically resets expired budget periods and syncs Redis → DB.
type Scheduler struct {
	manager  *Manager
	interval time.Duration
	logger   *slog.Logger
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewScheduler creates a Scheduler that ticks every interval.
func NewScheduler(manager *Manager, interval time.Duration, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		manager:  manager,
		interval: interval,
		logger:   logger,
		stop:     make(chan struct{}),
	}
}

// Start launches the background goroutine.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
}

// Stop signals the goroutine to exit and waits for it.
func (s *Scheduler) Stop() {
	close(s.stop)
	s.wg.Wait()
}

func (s *Scheduler) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	n, err := s.manager.store.ResetExpired(ctx, now)
	if err != nil {
		s.logger.Error("budget scheduler: reset expired failed", "error", err)
		return
	}
	if n > 0 {
		s.logger.Info("budget scheduler: reset expired periods", "count", n)
	}

	s.manager.SyncDB(ctx)
}
