package notification

import (
	"context"
	"time"
)

// Repository defines the persistence contract for notifications.
// It is completely independent of external database frameworks.
type Repository interface {
	// Create stores a new notification entity.
	// Must handle uk_recipient_event idempotently (ignore duplicates or return existing).
	Create(ctx context.Context, n *Notification) error

	// BatchCreate bulk inserts notifications, ignoring duplicates based on uk_recipient_event.
	BatchCreate(ctx context.Context, ns []*Notification) error

	// ListByRecipient returns a paginated list of notifications for the given recipient,
	// ordered by (created_at DESC, id DESC), optionally filtered by notification type.
	ListByRecipient(ctx context.Context, recipientID int64, cursor *NotificationCursor, limit int, filterType string) ([]*Notification, *NotificationCursor, bool, error)

	// CountUnread returns the total count of unread notifications (is_read = false) for the recipient.
	CountUnread(ctx context.Context, recipientID int64) (int64, error)

	// MarkAsRead marks specified notification IDs as read for the recipient.
	// Returns the number of newly marked records.
	MarkAsRead(ctx context.Context, recipientID int64, notificationIDs []int64) (int64, error)

	// MarkAllAsRead marks all unread notifications as read for the recipient.
	// Returns the number of newly marked records.
	MarkAllAsRead(ctx context.Context, recipientID int64) (int64, error)

	// ListAfter retrieves unread notifications created after the specified cursor timestamp/ID.
	// Used by the 60s overlapping catch-up window upon SSE client reconnection.
	// Ordered by (created_at ASC, id ASC) up to limit.
	ListAfter(ctx context.Context, recipientID int64, after time.Time, minID int64, limit int) ([]*Notification, error)
}
