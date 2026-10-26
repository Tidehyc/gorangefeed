package video

import "errors"

var (
	ErrVideoNotFound           = errors.New("video not found")
	ErrVideoForbidden          = errors.New("forbidden: only video author can perform this operation")
	ErrInvalidAuthorID         = errors.New("invalid author id")
	ErrEmptyTitle              = errors.New("video title cannot be empty")
	ErrTitleTooLong            = errors.New("video title exceeds maximum length (128 characters)")
	ErrDescriptionTooLong      = errors.New("video description exceeds maximum length (1000 characters)")
	ErrEmptyPlayURL            = errors.New("video play_url cannot be empty")
	ErrIdempotencyKeyTooLong   = errors.New("idempotency_key exceeds maximum length (128 characters)")
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key for author")
	ErrInvalidCursor           = errors.New("invalid pagination cursor")
	ErrInvalidLimit            = errors.New("invalid pagination limit")
)
