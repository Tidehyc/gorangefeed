package outbox

import (
	"context"
	"fmt"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	"gorm.io/gorm"
)

// OutboxModel maps to the `outbox` database table.
type OutboxModel struct {
	ID          int64      `gorm:"primaryKey;autoIncrement"`
	EventID     string     `gorm:"column:event_id;size:36;not null;uniqueIndex"`
	EventType   string     `gorm:"column:event_type;size:64;not null"`
	Payload     []byte     `gorm:"column:payload;type:json;not null"`
	Status      string     `gorm:"column:status;type:enum('pending','sending','sent','dead');not null;default:'pending'"`
	Attempts    int        `gorm:"column:attempts;not null;default:0"`
	NextRetryAt time.Time  `gorm:"column:next_retry_at;not null;default:CURRENT_TIMESTAMP(3)"`
	LeaseToken  *string    `gorm:"column:lease_token;size:36"`
	LeaseUntil  *time.Time `gorm:"column:lease_until"`
	LastError   *string    `gorm:"column:last_error;type:text"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	SentAt      *time.Time `gorm:"column:sent_at"`
}

func (OutboxModel) TableName() string {
	return "outbox"
}

// FromDomainEvent converts domain Event to database OutboxModel.
func FromDomainEvent(e *domainoutbox.Event) *OutboxModel {
	if e == nil {
		return nil
	}
	return &OutboxModel{
		ID:          e.ID,
		EventID:     e.EventID,
		EventType:   e.EventType,
		Payload:     e.Payload,
		Status:      string(e.Status),
		Attempts:    e.Attempts,
		NextRetryAt: e.NextRetryAt,
		LeaseToken:  e.LeaseToken,
		LeaseUntil:  e.LeaseUntil,
		LastError:   e.LastError,
		CreatedAt:   e.CreatedAt,
		SentAt:      e.SentAt,
	}
}

// SaveEventInTx inserts an outbox event within an existing transaction.
func SaveEventInTx(ctx context.Context, tx *gorm.DB, event *domainoutbox.Event) error {
	if event == nil {
		return nil
	}
	m := FromDomainEvent(event)
	if err := tx.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("failed to save outbox event in tx: %w", err)
	}
	event.ID = m.ID
	return nil
}
