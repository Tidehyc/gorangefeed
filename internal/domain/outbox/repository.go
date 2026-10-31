package outbox

import (
	"context"
	"time"
)

// Repository defines storage contracts for transactional outbox pattern.
type Repository interface {
	// SaveInTx saves an outbox event within an existing database transaction (e.g. *gorm.DB).
	SaveInTx(ctx context.Context, tx any, event *Event) error

	// ClaimLease atomically claims up to limit pending events that are ready for delivery,
	// updating their status to sending and assigning the given lease token.
	ClaimLease(ctx context.Context, leaseToken string, leaseDuration time.Duration, limit int) ([]*Event, error)

	// ReclaimExpiredLeases resets sending events whose lease has expired back to pending.
	ReclaimExpiredLeases(ctx context.Context) (int64, error)

	// MarkSent marks an event as successfully published.
	MarkSent(ctx context.Context, eventID string) error

	// MarkFailedWithBackoff schedules a retry with exponential backoff or marks as dead if max attempts exceeded.
	MarkFailedWithBackoff(ctx context.Context, eventID string, nextRetry time.Time, lastErr string, dead bool) error
}
