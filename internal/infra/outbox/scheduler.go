package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	"github.com/google/uuid"
)

// EventPublisher defines the contract for dispatching an outbox event to a message broker.
type EventPublisher interface {
	Publish(ctx context.Context, event *domainoutbox.Event) error
}

// Config configures the Outbox Scheduler poller and lease management.
type Config struct {
	PollInterval    time.Duration
	ReclaimInterval time.Duration
	LeaseDuration   time.Duration
	BatchSize       int
	MaxAttempts     int
}

// DefaultConfig provides recommended defaults for the outbox lease poller.
func DefaultConfig() Config {
	return Config{
		PollInterval:    500 * time.Millisecond,
		ReclaimInterval: 10 * time.Second,
		LeaseDuration:   30 * time.Second,
		BatchSize:       100,
		MaxAttempts:     10,
	}
}

// Option modifies scheduler configuration.
type Option func(*Config)

// WithPollInterval sets the polling interval.
func WithPollInterval(d time.Duration) Option {
	return func(c *Config) {
		if d > 0 {
			c.PollInterval = d
		}
	}
}

// WithReclaimInterval sets the lease reclamation interval.
func WithReclaimInterval(d time.Duration) Option {
	return func(c *Config) {
		if d > 0 {
			c.ReclaimInterval = d
		}
	}
}

// WithLeaseDuration sets the lease lock duration.
func WithLeaseDuration(d time.Duration) Option {
	return func(c *Config) {
		if d > 0 {
			c.LeaseDuration = d
		}
	}
}

// WithBatchSize sets the max batch size of events claimed per poll.
func WithBatchSize(size int) Option {
	return func(c *Config) {
		if size > 0 {
			c.BatchSize = size
		}
	}
}

// WithMaxAttempts sets the max retry attempts before transitioning to dead status.
func WithMaxAttempts(attempts int) Option {
	return func(c *Config) {
		if attempts > 0 {
			c.MaxAttempts = attempts
		}
	}
}

// CalculateBackoff calculates the exponential backoff duration based on attempts.
// If attempts reach or exceed maxAttempts, dead is returned as true.
// Backoff schedule:
// 1 attempt:  1s
// 2 attempts: 5s
// 3 attempts: 25s
// 4 attempts: 2m
// >= 5 attempts: 10m
func CalculateBackoff(attempts int, maxAttempts int) (backoff time.Duration, dead bool) {
	if attempts >= maxAttempts {
		return 0, true
	}
	switch attempts {
	case 1:
		return 1 * time.Second, false
	case 2:
		return 5 * time.Second, false
	case 3:
		return 25 * time.Second, false
	case 4:
		return 2 * time.Minute, false
	default:
		return 10 * time.Minute, false
	}
}

// Scheduler polls for pending outbox events using distributed lease claiming and dispatches them.
type Scheduler struct {
	repo      domainoutbox.Repository
	publisher EventPublisher
	cfg       Config
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
	running   bool
}

// NewScheduler creates a new Outbox lease scheduler instance.
func NewScheduler(repo domainoutbox.Repository, publisher EventPublisher, opts ...Option) (*Scheduler, error) {
	if repo == nil {
		return nil, errors.New("outbox repository cannot be nil")
	}
	if publisher == nil {
		return nil, errors.New("event publisher cannot be nil")
	}

	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Scheduler{
		repo:      repo,
		publisher: publisher,
		cfg:       cfg,
	}, nil
}

// PollAndDispatch claims a batch of pending events using a lease token,
// publishes them, and updates their lifecycle status.
// Returns the number of events processed and any system-level error.
func (s *Scheduler) PollAndDispatch(ctx context.Context) (int, error) {
	leaseToken := uuid.New().String()
	events, err := s.repo.ClaimLease(ctx, leaseToken, s.cfg.LeaseDuration, s.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("failed to claim outbox lease: %w", err)
	}
	if len(events) == 0 {
		return 0, nil
	}

	for _, event := range events {
		if err := s.dispatchEvent(ctx, event); err != nil {
			slog.Warn("failed to dispatch outbox event", "event_id", event.EventID, "type", event.EventType, "attempts", event.Attempts, "error", err)
		}
	}

	return len(events), nil
}

func (s *Scheduler) dispatchEvent(ctx context.Context, event *domainoutbox.Event) error {
	pubErr := s.publisher.Publish(ctx, event)
	if pubErr == nil {
		outboxDispatchedTotal.Inc()
		// Delivery successful -> mark as sent
		if err := s.repo.MarkSent(ctx, event.EventID); err != nil {
			return fmt.Errorf("failed to mark sent for event %s: %w", event.EventID, err)
		}
		return nil
	}

	// Delivery failed -> calculate backoff and schedule retry or mark dead
	backoff, dead := CalculateBackoff(event.Attempts, s.cfg.MaxAttempts)
	if dead {
		outboxDeadTotal.Inc()
	} else {
		outboxRetriedTotal.Inc()
	}

	nextRetry := time.Now().Add(backoff)
	if err := s.repo.MarkFailedWithBackoff(ctx, event.EventID, nextRetry, pubErr.Error(), dead); err != nil {
		return fmt.Errorf("failed to mark backoff for event %s: %w", event.EventID, err)
	}

	return pubErr
}

// ReclaimExpired scans for sending events whose lease has expired and resets them to pending.
func (s *Scheduler) ReclaimExpired(ctx context.Context) (int64, error) {
	reclaimed, err := s.repo.ReclaimExpiredLeases(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to reclaim expired outbox leases: %w", err)
	}
	if count, err := s.repo.CountByStatus(ctx, domainoutbox.StatusPending); err == nil {
		outboxBacklogGauge.Set(float64(count))
	}
	return reclaimed, nil
}

// Start begins background lease polling and expiration reclamation loops.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("outbox scheduler is already running")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running = true
	s.mu.Unlock()

	// 1. Polling worker goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.cfg.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if _, err := s.PollAndDispatch(runCtx); err != nil {
					if !errors.Is(err, context.Canceled) {
						slog.Error("outbox poll and dispatch error", "error", err)
					}
				}
			}
		}
	}()

	// 2. Lease reclamation worker goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		reclaimTicker := time.NewTicker(s.cfg.ReclaimInterval)
		defer reclaimTicker.Stop()

		for {
			select {
			case <-runCtx.Done():
				return
			case <-reclaimTicker.C:
				if reclaimed, err := s.ReclaimExpired(runCtx); err != nil {
					if !errors.Is(err, context.Canceled) {
						slog.Error("outbox lease reclamation error", "error", err)
					}
				} else if reclaimed > 0 {
					slog.Info("reclaimed expired outbox leases", "count", reclaimed)
				}
			}
		}
	}()

	return nil
}

// Stop gracefully stops the scheduler and waits for all polling goroutines to exit.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.cancel()
	s.running = false
	s.mu.Unlock()

	s.wg.Wait()
}
