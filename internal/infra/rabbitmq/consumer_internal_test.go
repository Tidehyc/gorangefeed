package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestConsumer_ShouldRequeue(t *testing.T) {
	c := &Consumer{maxAttempts: 5}

	cases := []struct {
		name      string
		attempts  int
		permanent bool
		want      bool
	}{
		{"first transient failure is retried", 1, false, true},
		{"last attempt inside the budget is retried", 4, false, true},
		{"attempt at the budget is dead-lettered", 5, false, false},
		{"attempt beyond the budget is dead-lettered", 6, false, false},
		{"permanent failure is never retried", 1, true, false},
		{"permanent failure stays permanent at the budget", 5, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.shouldRequeue(tc.attempts, tc.permanent); got != tc.want {
				t.Fatalf("shouldRequeue(attempts=%d, permanent=%v) = %v, want %v",
					tc.attempts, tc.permanent, got, tc.want)
			}
		})
	}
}

func TestPermanent_SurvivesWrapping(t *testing.T) {
	base := errors.New("malformed payload")

	if got := Permanent(nil); got != nil {
		t.Fatalf("Permanent(nil) = %v, want nil", got)
	}
	if !IsPermanent(Permanent(base)) {
		t.Fatalf("Permanent(base) not recognized as permanent")
	}
	if IsPermanent(base) {
		t.Fatalf("plain error must not be treated as permanent")
	}

	// Handlers are free to add context before returning; the marker has to survive that.
	wrapped := fmt.Errorf("handler failed: %w", Permanent(base))
	if !IsPermanent(wrapped) {
		t.Fatalf("wrapped permanent error not recognized: %v", wrapped)
	}
	if !errors.Is(wrapped, base) {
		t.Fatalf("wrapping the permanent error lost the underlying cause: %v", wrapped)
	}
}

func TestConsumer_AttemptKey_StableAcrossRedeliveries(t *testing.T) {
	c := &Consumer{queue: "queue.notification"}
	body := []byte(`{"video_id":1}`)

	first := c.attemptKey(amqp.Delivery{MessageId: "evt-1", Body: body})
	redelivered := c.attemptKey(amqp.Delivery{MessageId: "evt-1", Body: body, Redelivered: true})
	if first != redelivered {
		t.Fatalf("attempt key changed across redelivery: %q vs %q", first, redelivered)
	}
	if !strings.HasPrefix(first, "queue.notification:") {
		t.Fatalf("attempt key %q is not namespaced by queue", first)
	}

	// Publishers are not required to set MessageId; the body hash has to keep the counter stable.
	noID := c.attemptKey(amqp.Delivery{Body: body})
	noIDRedelivered := c.attemptKey(amqp.Delivery{Body: body, Redelivered: true})
	if noID != noIDRedelivered {
		t.Fatalf("body-hash attempt key changed across redelivery: %q vs %q", noID, noIDRedelivered)
	}
	if noID == c.attemptKey(amqp.Delivery{Body: []byte(`{"video_id":2}`)}) {
		t.Fatalf("distinct bodies produced the same attempt key %q", noID)
	}
}

func TestMemoryAttemptTracker_IncrementsUntilReset(t *testing.T) {
	tracker := newMemoryAttemptTracker()
	ctx := context.Background()

	for want := 1; want <= 3; want++ {
		got, err := tracker.Increment(ctx, "k")
		if err != nil {
			t.Fatalf("Increment failed: %v", err)
		}
		if got != want {
			t.Fatalf("Increment #%d = %d, want %d", want, got, want)
		}
	}

	if err := tracker.Reset(ctx, "k"); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}
	got, err := tracker.Increment(ctx, "k")
	if err != nil {
		t.Fatalf("Increment after Reset failed: %v", err)
	}
	if got != 1 {
		t.Fatalf("Increment after Reset = %d, want 1", got)
	}
}

func TestMemoryAttemptTracker_ExpiredCountersRestartAndGetCollected(t *testing.T) {
	tracker := newMemoryAttemptTracker()
	ctx := context.Background()

	if _, err := tracker.Increment(ctx, "k"); err != nil {
		t.Fatalf("Increment failed: %v", err)
	}

	now := time.Now()
	tracker.mu.Lock()
	tracker.entries["k"] = attemptEntry{count: 4, expires: now.Add(-time.Second)}
	tracker.entries["stale"] = attemptEntry{count: 2, expires: now.Add(-time.Minute)}
	tracker.lastSweep = now.Add(-2 * attemptSweepInterval)
	tracker.mu.Unlock()

	got, err := tracker.Increment(ctx, "k")
	if err != nil {
		t.Fatalf("Increment failed: %v", err)
	}
	if got != 1 {
		t.Fatalf("expired counter did not restart: Increment = %d, want 1", got)
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if _, ok := tracker.entries["stale"]; ok {
		t.Fatalf("expired entry was not collected by the periodic sweep")
	}
}
