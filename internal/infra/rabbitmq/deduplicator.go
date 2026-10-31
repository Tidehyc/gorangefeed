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
	MarkConsumed(ctx context.Context, eventID string) (bool, error)

	// ExecuteWithDeduplication executes a business operation fn inside a transaction that records eventID.
	// If the event was already consumed, it safely returns nil without executing fn (idempotent skip).
	ExecuteWithDeduplication(ctx context.Context, eventID string, fn func(tx *gorm.DB) error) error

	// IsConsumed checks if an event has already been consumed.
	IsConsumed(ctx context.Context, eventID string) (bool, error)
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

func (d *gormDeduplicator) ExecuteWithDeduplication(ctx context.Context, eventID string, fn func(tx *gorm.DB) error) error {
	if eventID == "" {
		return errors.New("event ID cannot be empty")
	}

	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Attempt to insert into consumed_events
		err := tx.Exec(
			"INSERT INTO consumed_events (event_id, consumed_at) VALUES (?, ?)",
			eventID,
			time.Now(),
		).Error

		if err != nil {
			var mysqlErr *mysql.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
				// Duplicate primary key: already consumed! Return nil to complete idempotently
				return nil
			}
			return fmt.Errorf("failed to record consumed event in tx: %w", err)
		}

		// 2. Execute business handler in the same transaction
		if fn != nil {
			if err := fn(tx); err != nil {
				return err
			}
		}

		return nil
	})
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
