package outbox

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Status represents the processing status of an outbox event.
type Status string

const (
	StatusPending Status = "pending"
	StatusSending Status = "sending"
	StatusSent    Status = "sent"
	StatusDead    Status = "dead"
)

// Common Outbox Event Types
const (
	EventTypeVideoPublished           = "video.published"
	EventTypeInteractionLiked         = "interaction.liked"
	EventTypeInteractionUnliked       = "interaction.unliked"
	EventTypeInteractionCommented     = "interaction.commented"
	EventTypeInteractionCommentDelete = "interaction.comment_deleted"
	EventTypeRelationFollowed         = "relation.followed"
	EventTypeRelationUnfollowed       = "relation.unfollowed"
)

// Event represents an outbox message entity stored alongside domain state transactions.
type Event struct {
	ID          int64      `json:"id"`
	EventID     string     `json:"event_id"`
	EventType   string     `json:"event_type"`
	Payload     []byte     `json:"payload"`
	Status      Status     `json:"status"`
	Attempts    int        `json:"attempts"`
	NextRetryAt time.Time  `json:"next_retry_at"`
	LeaseToken  *string    `json:"lease_token,omitempty"`
	LeaseUntil  *time.Time `json:"lease_until,omitempty"`
	LastError   *string    `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
}

// NewEvent creates a new pending outbox event with a generated UUID.
func NewEvent(eventType string, payload any) (*Event, error) {
	if eventType == "" {
		return nil, errors.New("event type cannot be empty")
	}

	var data []byte
	var err error
	switch v := payload.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		data, err = json.Marshal(v)
		if err != nil {
			return nil, err
		}
	}

	now := time.Now()
	return &Event{
		EventID:     uuid.New().String(),
		EventType:   eventType,
		Payload:     data,
		Status:      StatusPending,
		Attempts:    0,
		NextRetryAt: now,
		CreatedAt:   now,
	}, nil
}
