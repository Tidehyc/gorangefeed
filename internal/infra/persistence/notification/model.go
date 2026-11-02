package notification

import (
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
)

// NotificationModel maps to the `notifications` database table.
type NotificationModel struct {
	ID          int64      `gorm:"primaryKey;autoIncrement"`
	RecipientID int64      `gorm:"column:recipient_id;not null;index:idx_recipient_created_id,priority:1;index:idx_recipient_unread,priority:1;uniqueIndex:uk_recipient_event,priority:1"`
	ActorID     int64      `gorm:"column:actor_id;not null;index:idx_actor"`
	Type        string     `gorm:"column:type;type:varchar(32);not null"`
	TargetType  string     `gorm:"column:target_type;type:varchar(32);not null;default:''"`
	TargetID    int64      `gorm:"column:target_id;not null;default:0"`
	Title       string     `gorm:"column:title;type:varchar(128);not null;default:''"`
	Content     string     `gorm:"column:content;type:varchar(1024);not null;default:''"`
	EventID     string     `gorm:"column:event_id;type:varchar(64);not null;default:'';uniqueIndex:uk_recipient_event,priority:2"`
	IsRead      bool       `gorm:"column:is_read;not null;default:0;index:idx_recipient_unread,priority:2"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3);index:idx_recipient_created_id,priority:2"`
	ReadAt      *time.Time `gorm:"column:read_at"`
}

func (NotificationModel) TableName() string {
	return "notifications"
}

func toDomain(m *NotificationModel) *domainnotification.Notification {
	if m == nil {
		return nil
	}
	return &domainnotification.Notification{
		ID:          m.ID,
		RecipientID: m.RecipientID,
		ActorID:     m.ActorID,
		Type:        m.Type,
		TargetType:  m.TargetType,
		TargetID:    m.TargetID,
		Title:       m.Title,
		Content:     m.Content,
		EventID:     m.EventID,
		IsRead:      m.IsRead,
		CreatedAt:   m.CreatedAt,
		ReadAt:      m.ReadAt,
	}
}

func fromDomain(n *domainnotification.Notification) *NotificationModel {
	if n == nil {
		return nil
	}
	return &NotificationModel{
		ID:          n.ID,
		RecipientID: n.RecipientID,
		ActorID:     n.ActorID,
		Type:        n.Type,
		TargetType:  n.TargetType,
		TargetID:    n.TargetID,
		Title:       n.Title,
		Content:     n.Content,
		EventID:     n.EventID,
		IsRead:      n.IsRead,
		CreatedAt:   n.CreatedAt,
		ReadAt:      n.ReadAt,
	}
}
