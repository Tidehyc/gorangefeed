package notification

import "errors"

var (
	ErrInvalidRecipientID    = errors.New("invalid recipient id")
	ErrInvalidActorID        = errors.New("invalid actor id")
	ErrInvalidNotificationID = errors.New("invalid notification id")
	ErrInvalidType           = errors.New("invalid notification type")
	ErrInvalidCursor         = errors.New("invalid notification cursor")
	ErrNotificationNotFound  = errors.New("notification not found")
	ErrEmptyTitle            = errors.New("notification title cannot be empty")
	ErrTitleTooLong          = errors.New("notification title exceeds maximum length")
	ErrContentTooLong        = errors.New("notification content exceeds maximum length")
	ErrSelfNotification      = errors.New("cannot notify oneself")
	ErrTicketExpired         = errors.New("sse ticket is invalid or expired")
	ErrInvalidEventID        = errors.New("invalid event id format")
)
