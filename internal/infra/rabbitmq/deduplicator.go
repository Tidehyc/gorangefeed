package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// Deduplicator defines transactional idempotency contracts on the consumed_events table.
type Deduplicator interface {
	// MarkConsumed records the eventID into consumed_events.
	// Returns (isNew: true, err: nil) if successfully recorded for the first time.
	// Returns (isNew: false, err: nil) if already consumed previously (duplicate replay).
	// Consumers call this last, once their side effects have landed.
	MarkConsumed(ctx context.Context, eventID string) (bool, error)

	// IsConsumed checks if an event has already been consumed.
	IsConsumed(ctx context.Context, eventID string) (bool, error)

	// PurgeConsumedBefore deletes markers consumed strictly before cutoff and returns the number of
	// rows removed. Markers are only needed to cover the broker's redelivery window, so keeping
	// them forever is dead weight.
	PurgeConsumedBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

type gormDeduplicator struct {
	db *gorm.DB
}

// NewDeduplicator creates a new consumer deduplicator backed by MySQL.
func NewDeduplicator(db *gorm.DB) Deduplicator {
	return &gormDeduplicator{db: db}
}

func (d *gormDeduplicator) MarkConsumed(ctx context.Context, eventID string) (bool, error) {
	if eventID == "" {
		return false, errors.New("event ID cannot be empty")
	}

	err := d.db.WithContext(ctx).Exec(
		"INSERT INTO consumed_events (event_id, consumed_at) VALUES (?, ?)",
		eventID,
		time.Now(),
	).Error

	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			// Duplicate primary key: event already consumed
			return false, nil
		}
		return false, fmt.Errorf("failed to mark consumed event %s: %w", eventID, err)
	}

	return true, nil
}

func (d *gormDeduplicator) IsConsumed(ctx context.Context, eventID string) (bool, error) {
	if eventID == "" {
		return false, errors.New("event ID cannot be empty")
	}

	var count int64
	err := d.db.WithContext(ctx).Table("consumed_events").Where("event_id = ?", eventID).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check consumed event %s: %w", eventID, err)
	}

	return count > 0, nil
}

func (d *gormDeduplicator) PurgeConsumedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, errors.New("purge cutoff cannot be zero")
	}

	result := d.db.WithContext(ctx).Exec(
		"DELETE FROM consumed_events WHERE consumed_at < ?",
		cutoff,
	)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to purge consumed events before %s: %w", cutoff.Format(time.RFC3339), result.Error)
	}

	return result.RowsAffected, nil
}
