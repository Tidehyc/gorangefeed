package notification

import (
	"context"
	"errors"
	"fmt"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
)

// Service coordinates system notification queries, mark-as-read mutations, and dispatches.
type Service struct {
	repo        domainnotification.Repository
	broadcaster domainnotification.Broadcaster
}

// NewService creates a new notification Service instance.
func NewService(repo domainnotification.Repository, broadcaster domainnotification.Broadcaster) (*Service, error) {
	if repo == nil {
		return nil, errors.New("notification repository cannot be nil")
	}
	return &Service{
		repo:        repo,
		broadcaster: broadcaster,
	}, nil
}

// ListNotifications retrieves a paginated list of notifications for the recipient.
func (s *Service) ListNotifications(
	ctx context.Context,
	recipientID int64,
	filterType string,
	cursorStr string,
	limit int,
) ([]*domainnotification.Notification, string, bool, error) {
	if recipientID <= 0 {
		return nil, "", false, domainnotification.ErrInvalidRecipientID
	}
	if limit <= 0 {
		limit = domainnotification.DefaultLimit
	}
	if limit > domainnotification.MaxLimit {
		limit = domainnotification.MaxLimit
	}

	var cursor *domainnotification.NotificationCursor
	if cursorStr != "" {
		c, err := domainnotification.DecodeNotificationCursor(cursorStr)
		if err != nil {
			return nil, "", false, err
		}
		cursor = c
	}

	items, nextCursorObj, hasMore, err := s.repo.ListByRecipient(ctx, recipientID, cursor, limit, filterType)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to list notifications: %w", err)
	}

	var nextCursor string
	if nextCursorObj != nil {
		nextCursor = nextCursorObj.Encode()
	}

	return items, nextCursor, hasMore, nil
}

// GetUnreadCount returns the number of unread notifications for a recipient using index coverage.
func (s *Service) GetUnreadCount(ctx context.Context, recipientID int64) (int64, error) {
	if recipientID <= 0 {
		return 0, domainnotification.ErrInvalidRecipientID
	}
	return s.repo.CountUnread(ctx, recipientID)
}

// MarkAsRead marks a single notification as read for a recipient.
func (s *Service) MarkAsRead(ctx context.Context, recipientID, notificationID int64) error {
	if recipientID <= 0 {
		return domainnotification.ErrInvalidRecipientID
	}
	if notificationID <= 0 {
		return domainnotification.ErrInvalidNotificationID
	}
	_, err := s.repo.MarkAsRead(ctx, recipientID, []int64{notificationID})
	return err
}

// MarkBatchAsRead marks multiple notifications as read for a recipient.
func (s *Service) MarkBatchAsRead(ctx context.Context, recipientID int64, notificationIDs []int64) (int64, error) {
	if recipientID <= 0 {
		return 0, domainnotification.ErrInvalidRecipientID
	}
	if len(notificationIDs) == 0 {
		return 0, nil
	}
	return s.repo.MarkAsRead(ctx, recipientID, notificationIDs)
}

// MarkAllAsRead marks all unread notifications for a recipient as read atomically.
func (s *Service) MarkAllAsRead(ctx context.Context, recipientID int64) (int64, error) {
	if recipientID <= 0 {
		return 0, domainnotification.ErrInvalidRecipientID
	}
	return s.repo.MarkAllAsRead(ctx, recipientID)
}

// CreateNotification validates, creates, and optionally broadcasts a notification.
func (s *Service) CreateNotification(ctx context.Context, notif *domainnotification.Notification) error {
	if notif == nil {
		return errors.New("notification cannot be nil")
	}
	if err := notif.Validate(); err != nil {
		return err
	}

	if err := s.repo.Create(ctx, notif); err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}

	if s.broadcaster != nil {
		_ = s.broadcaster.Publish(ctx, notif)
	}
	return nil
}
