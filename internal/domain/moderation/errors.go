package moderation

import "errors"

var (
	ErrInvalidVideoID        = errors.New("invalid video id")
	ErrInvalidReviewStatus   = errors.New("invalid review status")
	ErrInvalidPolicy         = errors.New("invalid review policy")
	ErrInvalidRiskScore      = errors.New("invalid risk score")
	ErrTaskNotFound          = errors.New("review task not found")
	ErrTaskAlreadyProcessed  = errors.New("review task is already processed and cannot transition to this state")
	ErrInvalidAction         = errors.New("invalid moderation action")
	ErrInvalidTargetID       = errors.New("invalid target id")
	ErrUnauthorizedReviewer  = errors.New("unauthorized reviewer id")
	ErrEmptyReviewNote       = errors.New("review note cannot be empty")
)
