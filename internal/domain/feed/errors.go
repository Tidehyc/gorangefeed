package feed

import "errors"

var (
	// ErrInvalidCursor indicates the cursor passed to pagination is invalid.
	ErrInvalidCursor = errors.New("invalid cursor: must be non-negative")
	// ErrInvalidLimit indicates the pagination limit is outside allowed range.
	ErrInvalidLimit = errors.New("invalid limit: must be between 1 and 30")
	// ErrVideoNotFound indicates the specified video was not found.
	ErrVideoNotFound = errors.New("video not found")
	// ErrFeedEmpty indicates no feed items could be retrieved.
	ErrFeedEmpty = errors.New("feed is empty")
	// ErrInvalidEventType indicates an unrecognized view event type.
	ErrInvalidEventType = errors.New("invalid view event type")
	// ErrInvalidVideoID indicates the video ID is invalid (<= 0).
	ErrInvalidVideoID = errors.New("invalid video id")
)
