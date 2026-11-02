package creator

import "errors"

var (
	ErrInvalidAuthorID       = errors.New("invalid author id")
	ErrDraftNotFound         = errors.New("draft not found")
	ErrDraftForbidden        = errors.New("permission denied for draft")
	ErrEmptyDraftTitle       = errors.New("draft title cannot be empty when publishing")
	ErrEmptyDraftPlayURL     = errors.New("draft play url cannot be empty when publishing")
	ErrDraftTitleTooLong     = errors.New("draft title exceeds maximum length")
	ErrDraftDescTooLong      = errors.New("draft description exceeds maximum length")
	ErrDraftPlayURLTooLong   = errors.New("draft play url exceeds maximum length")
	ErrDraftCoverURLTooLong  = errors.New("draft cover url exceeds maximum length")
	ErrDraftAlreadyPublished = errors.New("draft is already published")
	ErrDraftAlreadyScheduled = errors.New("draft is currently scheduled for publishing")
	ErrInvalidCursor         = errors.New("invalid draft cursor")

	ErrInvalidTaskID        = errors.New("invalid task id")
	ErrInvalidScheduledTime = errors.New("scheduled time must be between 5 minutes and 30 days in the future")
	ErrTaskNotFound         = errors.New("scheduled task not found")
	ErrTaskForbidden        = errors.New("permission denied for scheduled task")
	ErrTaskNotScheduled     = errors.New("scheduled task is not in scheduled state")
	ErrTaskAlreadyExecuting = errors.New("scheduled task is currently executing")
)
