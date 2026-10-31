package outbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	"gorm.io/gorm"
)

type gormRepository struct {
	db *gorm.DB
}

// NewRepository creates a new Outbox repository backed by GORM.
func NewRepository(db *gorm.DB) domainoutbox.Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) SaveInTx(ctx context.Context, tx any, event *domainoutbox.Event) error {
	gormTx, ok := tx.(*gorm.DB)
	if !ok || gormTx == nil {
		return errors.New("invalid gorm transaction handle")
	}
	return SaveEventInTx(ctx, gormTx, event)
}

func (r *gormRepository) ClaimLease(ctx context.Context, leaseToken string, leaseDuration time.Duration, limit int) ([]*domainoutbox.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	now := time.Now()
	leaseUntil := now.Add(leaseDuration)

	var claimedEvents []*domainoutbox.Event
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Atomically claim events with status=pending and next_retry_at <= now
		res := tx.Model(&OutboxModel{}).
			Where("status = ? AND next_retry_at <= ?", string(domainoutbox.StatusPending), now).
			Order("id ASC").
			Limit(limit).
			Updates(map[string]any{
				"status":      string(domainoutbox.StatusSending),
				"lease_token": leaseToken,
				"lease_until": leaseUntil,
				"attempts":    gorm.Expr("attempts + 1"),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}

		// 2. Fetch the claimed records by lease_token
		var models []OutboxModel
		if err := tx.Where("lease_token = ? AND status = ?", leaseToken, string(domainoutbox.StatusSending)).
			Find(&models).Error; err != nil {
			return err
		}

		for i := range models {
			m := &models[i]
			claimedEvents = append(claimedEvents, &domainoutbox.Event{
				ID:          m.ID,
				EventID:     m.EventID,
				EventType:   m.EventType,
				Payload:     m.Payload,
				Status:      domainoutbox.Status(m.Status),
				Attempts:    m.Attempts,
				NextRetryAt: m.NextRetryAt,
				LeaseToken:  m.LeaseToken,
				LeaseUntil:  m.LeaseUntil,
				LastError:   m.LastError,
				CreatedAt:   m.CreatedAt,
				SentAt:      m.SentAt,
			})
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to claim outbox lease: %w", err)
	}
	return claimedEvents, nil
}

func (r *gormRepository) ReclaimExpiredLeases(ctx context.Context) (int64, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&OutboxModel{}).
		Where("status = ? AND lease_until < ?", string(domainoutbox.StatusSending), now).
		Updates(map[string]any{
			"status":      string(domainoutbox.StatusPending),
			"lease_token": nil,
			"lease_until": nil,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("failed to reclaim expired leases: %w", res.Error)
	}
	return res.RowsAffected, nil
}

func (r *gormRepository) MarkSent(ctx context.Context, eventID string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&OutboxModel{}).
		Where("event_id = ?", eventID).
		Updates(map[string]any{
			"status":      string(domainoutbox.StatusSent),
			"sent_at":     now,
			"lease_token": nil,
			"lease_until": nil,
		})
	if res.Error != nil {
		return fmt.Errorf("failed to mark outbox event sent: %w", res.Error)
	}
	return nil
}

func (r *gormRepository) MarkFailedWithBackoff(ctx context.Context, eventID string, nextRetry time.Time, lastErr string, dead bool) error {
	status := domainoutbox.StatusPending
	if dead {
		status = domainoutbox.StatusDead
	}

	res := r.db.WithContext(ctx).Model(&OutboxModel{}).
		Where("event_id = ?", eventID).
		Updates(map[string]any{
			"status":        string(status),
			"next_retry_at": nextRetry,
			"last_error":    lastErr,
			"lease_token":   nil,
			"lease_until":   nil,
		})
	if res.Error != nil {
		return fmt.Errorf("failed to update outbox failure backoff: %w", res.Error)
	}
	return nil
}
