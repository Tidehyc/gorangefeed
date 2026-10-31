package outbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
)

type mockOutboxRepo struct {
	mu           sync.Mutex
	events       []*domainoutbox.Event
	sentIDs      []string
	failedCalls  []failedCall
	reclaimCount int64
}

type failedCall struct {
	eventID   string
	nextRetry time.Time
	lastError string
	dead      bool
}

func (m *mockOutboxRepo) SaveInTx(ctx context.Context, tx any, event *domainoutbox.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockOutboxRepo) ClaimLease(ctx context.Context, leaseToken string, leaseDuration time.Duration, limit int) ([]*domainoutbox.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var claimed []*domainoutbox.Event
	for _, e := range m.events {
		if e.Status == domainoutbox.StatusPending {
			e.Status = domainoutbox.StatusSending
			e.LeaseToken = &leaseToken
			leaseUntil := time.Now().Add(leaseDuration)
			e.LeaseUntil = &leaseUntil
			e.Attempts++
			claimed = append(claimed, e)
			if len(claimed) >= limit {
				break
			}
		}
	}
	return claimed, nil
}

func (m *mockOutboxRepo) ReclaimExpiredLeases(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reclaimCount, nil
}

func (m *mockOutboxRepo) MarkSent(ctx context.Context, eventID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentIDs = append(m.sentIDs, eventID)
	for _, e := range m.events {
		if e.EventID == eventID {
			e.Status = domainoutbox.StatusSent
		}
	}
	return nil
}

func (m *mockOutboxRepo) MarkFailedWithBackoff(ctx context.Context, eventID string, nextRetry time.Time, lastErr string, dead bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedCalls = append(m.failedCalls, failedCall{
		eventID:   eventID,
		nextRetry: nextRetry,
		lastError: lastErr,
		dead:      dead,
	})
	for _, e := range m.events {
		if e.EventID == eventID {
			if dead {
				e.Status = domainoutbox.StatusDead
			} else {
				e.Status = domainoutbox.StatusPending
			}
		}
	}
	return nil
}

func (m *mockOutboxRepo) CountByStatus(ctx context.Context, status domainoutbox.Status) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	for _, e := range m.events {
		if e.Status == status {
			count++
		}
	}
	return count, nil
}

type mockPublisher struct {
	mu           sync.Mutex
	publishedIDs []string
	publishErr   map[string]error
}

func (p *mockPublisher) Publish(ctx context.Context, event *domainoutbox.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.publishedIDs = append(p.publishedIDs, event.EventID)
	if err, ok := p.publishErr[event.EventID]; ok {
		return err
	}
	return nil
}

func TestCalculateBackoff(t *testing.T) {
	maxAttempts := 10

	tests := []struct {
		attempts int
		expected time.Duration
		dead     bool
	}{
		{attempts: 1, expected: 1 * time.Second, dead: false},
		{attempts: 2, expected: 5 * time.Second, dead: false},
		{attempts: 3, expected: 25 * time.Second, dead: false},
		{attempts: 4, expected: 2 * time.Minute, dead: false},
		{attempts: 5, expected: 10 * time.Minute, dead: false},
		{attempts: 9, expected: 10 * time.Minute, dead: false},
		{attempts: 10, expected: 0, dead: true},
		{attempts: 12, expected: 0, dead: true},
	}

	for _, tt := range tests {
		backoff, dead := CalculateBackoff(tt.attempts, maxAttempts)
		if backoff != tt.expected || dead != tt.dead {
			t.Errorf("CalculateBackoff(%d, %d) = (%v, %v); expected (%v, %v)",
				tt.attempts, maxAttempts, backoff, dead, tt.expected, tt.dead)
		}
	}
}

func TestScheduler_PollAndDispatch_Success(t *testing.T) {
	repo := &mockOutboxRepo{
		events: []*domainoutbox.Event{
			{EventID: "evt-1", EventType: "video.published", Status: domainoutbox.StatusPending},
			{EventID: "evt-2", EventType: "interaction.liked", Status: domainoutbox.StatusPending},
		},
	}
	pub := &mockPublisher{publishErr: make(map[string]error)}

	scheduler, err := NewScheduler(repo, pub, WithBatchSize(10))
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	ctx := context.Background()
	n, err := scheduler.PollAndDispatch(ctx)
	if err != nil {
		t.Fatalf("PollAndDispatch failed: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 dispatched events, got %d", n)
	}

	if len(repo.sentIDs) != 2 {
		t.Fatalf("expected 2 sentIDs, got %d", len(repo.sentIDs))
	}
	if repo.sentIDs[0] != "evt-1" || repo.sentIDs[1] != "evt-2" {
		t.Errorf("unexpected sentIDs: %v", repo.sentIDs)
	}
}

func TestScheduler_PollAndDispatch_FailureAndDead(t *testing.T) {
	repo := &mockOutboxRepo{
		events: []*domainoutbox.Event{
			{EventID: "evt-fail", EventType: "video.published", Status: domainoutbox.StatusPending, Attempts: 1},
			{EventID: "evt-dead", EventType: "interaction.liked", Status: domainoutbox.StatusPending, Attempts: 9},
		},
	}
	pub := &mockPublisher{
		publishErr: map[string]error{
			"evt-fail": errors.New("network timeout"),
			"evt-dead": errors.New("broker unavailable"),
		},
	}

	scheduler, err := NewScheduler(repo, pub, WithMaxAttempts(10))
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	ctx := context.Background()
	n, err := scheduler.PollAndDispatch(ctx)
	if err != nil {
		t.Fatalf("PollAndDispatch failed: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 dispatched events, got %d", n)
	}

	if len(repo.failedCalls) != 2 {
		t.Fatalf("expected 2 failed calls, got %d", len(repo.failedCalls))
	}

	// First event: attempts was 1, incremented to 2 in ClaimLease -> not dead
	if repo.failedCalls[0].eventID != "evt-fail" || repo.failedCalls[0].dead != false {
		t.Errorf("expected evt-fail not dead, got %v", repo.failedCalls[0])
	}

	// Second event: attempts was 9, incremented to 10 in ClaimLease -> reached maxAttempts -> dead
	if repo.failedCalls[1].eventID != "evt-dead" || repo.failedCalls[1].dead != true {
		t.Errorf("expected evt-dead dead=true, got %v", repo.failedCalls[1])
	}
}

func TestScheduler_ReclaimExpired(t *testing.T) {
	repo := &mockOutboxRepo{reclaimCount: 5}
	pub := &mockPublisher{}

	scheduler, err := NewScheduler(repo, pub)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	reclaimed, err := scheduler.ReclaimExpired(context.Background())
	if err != nil {
		t.Fatalf("ReclaimExpired failed: %v", err)
	}
	if reclaimed != 5 {
		t.Errorf("expected 5 reclaimed, got %d", reclaimed)
	}
}

func TestScheduler_Lifecycle(t *testing.T) {
	repo := &mockOutboxRepo{}
	pub := &mockPublisher{}

	scheduler, err := NewScheduler(repo, pub,
		WithPollInterval(20*time.Millisecond),
		WithReclaimInterval(20*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewScheduler failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Attempting to start again should return error
	if err := scheduler.Start(ctx); err == nil {
		t.Error("expected error when starting already running scheduler")
	}

	time.Sleep(60 * time.Millisecond)

	// Stop gracefully
	scheduler.Stop()

	// Calling Stop again should be safe and idempotent
	scheduler.Stop()
}
