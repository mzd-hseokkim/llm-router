package billing

import (
	"context"
	"log/slog"
	"time"
)

// ReportSender receives a generated report and distributes it (e.g. via email/Slack).
type ReportSender interface {
	Send(ctx context.Context, report *ChargebackReport) error
}

// Scheduler generates monthly chargeback reports on the 1st of each month.
type Scheduler struct {
	svc    *ChargebackService
	sender ReportSender // may be nil
	logger *slog.Logger
	stop   chan struct{}
}

// NewScheduler returns a Scheduler. sender may be nil (report generated but not sent).
func NewScheduler(svc *ChargebackService, sender ReportSender, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		svc:    svc,
		sender: sender,
		logger: logger,
		stop:   make(chan struct{}),
	}
}

// Start launches the background scheduler goroutine.
func (s *Scheduler) Start() {
	go s.run()
}

// Stop signals the scheduler to stop.
func (s *Scheduler) Stop() {
	close(s.stop)
}

func (s *Scheduler) run() {
	for {
		next := nextMonthStart()
		timer := time.NewTimer(time.Until(next))
		select {
		case <-s.stop:
			timer.Stop()
			return
		case t := <-timer.C:
			s.generate(t)
		}
	}
}

func (s *Scheduler) generate(now time.Time) {
	// Compute previous-month range.
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	from := firstOfMonth.AddDate(0, -1, 0)
	to := firstOfMonth

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	report, err := s.svc.BuildReport(ctx, from, to)
	if err != nil {
		s.logger.Error("billing scheduler: failed to generate report", "period", from.Format("2006-01"), "error", err)
		return
	}
	s.logger.Info("billing scheduler: monthly report generated", "period", report.Period)

	if s.sender != nil {
		if err := s.sender.Send(ctx, report); err != nil {
			s.logger.Warn("billing scheduler: failed to send report", "error", err)
		}
	}
}

// nextMonthStart returns the UTC start of the next calendar month (the 1st at 00:00).
func nextMonthStart() time.Time {
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return first
}
