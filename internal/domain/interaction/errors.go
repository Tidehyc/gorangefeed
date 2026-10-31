package interaction

import "errors"

var (
	ErrVideoNotFound          = errors.New("video not found")
	ErrCommentNotFound        = errors.New("comment not found")
	ErrCommentForbidden       = errors.New("forbidden to operate this comment")
	ErrEmptyCommentContent    = errors.New("comment content cannot be empty")
	ErrCommentContentTooLong  = errors.New("comment content exceeds maximum length")
	ErrEmptyIdempotencyKey    = errors.New("idempotency key cannot be empty")
	ErrIdempotencyKeyTooLong  = errors.New("idempotency key exceeds maximum length")
	ErrInvalidParentComment   = errors.New("invalid parent comment")
	ErrInvalidUserID          = errors.New("invalid user id")
	ErrInvalidVideoID         = errors.New("invalid video id")
)
