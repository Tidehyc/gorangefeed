package notification

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	TypeLike    = "like"
	TypeComment = "comment"
	TypeFollow  = "follow"
	TypeMention = "mention"
	TypeSystem  = "system"

	TargetVideo   = "video"
	TargetComment = "comment"
	TargetUser    = "user"

	MaxTitleLength   = 128
	MaxContentLength = 1024
	DefaultLimit     = 20
	MaxLimit         = 100
)

// ValidTypes defines supported notification types.
var ValidTypes = map[string]bool{
	TypeLike:    true,
	TypeComment: true,
	TypeFollow:  true,
	TypeMention: true,
	TypeSystem:  true,
}

// ActorInfo represents the sender's public identity summary.
type ActorInfo struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// Notification represents an in-app system notification aggregate root.
type Notification struct {
	ID          int64      `json:"id"`
	RecipientID int64      `json:"recipient_id"`
	ActorID     int64      `json:"actor_id"`
	Type        string     `json:"type"`
	TargetType  string     `json:"target_type"`
	TargetID    int64      `json:"target_id"`
	Title       string     `json:"title"`
	Content     string     `json:"content"`
	EventID     string     `json:"event_id"`
	IsRead      bool       `json:"is_read"`
	CreatedAt   time.Time  `json:"created_at"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	Actor       *ActorInfo `json:"actor,omitempty"`
}

// NotificationCursor represents a keyset cursor for chronological pagination.
type NotificationCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

type cursorPayload struct {
	CreatedAt string `json:"created_at"`
	ID        int64  `json:"id"`
}

// Validate checks business integrity constraints on Notification.
func (n *Notification) Validate() error {
	if n == nil {
		return errors.New("notification cannot be nil")
	}
	if n.RecipientID <= 0 {
		return ErrInvalidRecipientID
	}
	if n.ActorID <= 0 && n.Type != TypeSystem {
		return ErrInvalidActorID
	}
	if n.RecipientID == n.ActorID && n.Type != TypeSystem {
		return ErrSelfNotification
	}

	nType := strings.ToLower(strings.TrimSpace(n.Type))
	if !ValidTypes[nType] {
		return ErrInvalidType
	}

	title := strings.TrimSpace(n.Title)
	if title == "" {
		return ErrEmptyTitle
	}
	if len([]rune(title)) > MaxTitleLength {
		return ErrTitleTooLong
	}

	content := strings.TrimSpace(n.Content)
	if len([]rune(content)) > MaxContentLength {
		return ErrContentTooLong
	}

	return nil
}

// NewNotification constructs and validates a new Notification entity.
func NewNotification(
	recipientID, actorID int64,
	nType, targetType string,
	targetID int64,
	title, content, eventID string,
) (*Notification, error) {
	n := &Notification{
		RecipientID: recipientID,
		ActorID:     actorID,
		Type:        strings.ToLower(strings.TrimSpace(nType)),
		TargetType:  strings.ToLower(strings.TrimSpace(targetType)),
		TargetID:    targetID,
		Title:       strings.TrimSpace(title),
		Content:     strings.TrimSpace(content),
		EventID:     strings.TrimSpace(eventID),
		IsRead:      false,
		CreatedAt:   time.Now().UTC(),
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}
	return n, nil
}

// MarkAsRead transitions the notification to read state.
func (n *Notification) MarkAsRead(readAt time.Time) {
	if n == nil || n.IsRead {
		return
	}
	if readAt.IsZero() {
		readAt = time.Now().UTC()
	}
	n.IsRead = true
	n.ReadAt = &readAt
}

// WithActor populates sender identity information.
func (n *Notification) WithActor(actor *ActorInfo) {
	if n == nil || actor == nil {
		return
	}
	n.Actor = actor
}

// Cursor extracts the NotificationCursor from the entity.
func (n *Notification) Cursor() *NotificationCursor {
	if n == nil || n.ID <= 0 {
		return nil
	}
	return &NotificationCursor{
		CreatedAt: n.CreatedAt,
		ID:        n.ID,
	}
}

// Encode serializes the cursor into a URL-safe Base64 string.
func (c *NotificationCursor) Encode() string {
	if c == nil || c.ID <= 0 || c.CreatedAt.IsZero() {
		return ""
	}
	payload := cursorPayload{
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339Nano),
		ID:        c.ID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeNotificationCursor deserializes a Base64 cursor string into a NotificationCursor.
func DecodeNotificationCursor(raw string) (*NotificationCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		data, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, ErrInvalidCursor
		}
	}

	var payload cursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, ErrInvalidCursor
	}

	if payload.ID <= 0 {
		return nil, ErrInvalidCursor
	}

	t, err := time.Parse(time.RFC3339Nano, payload.CreatedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, payload.CreatedAt)
		if err != nil {
			return nil, ErrInvalidCursor
		}
	}

	return &NotificationCursor{
		CreatedAt: t.UTC(),
		ID:        payload.ID,
	}, nil
}

// SSEEventID formats the event ID for SSE stream (timestamp in ms + notification id).
func (n *Notification) SSEEventID() string {
	if n == nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", n.CreatedAt.UnixMilli(), n.ID)
}

// ParseSSEEventID parses a Last-Event-ID header (e.g. "1730000000000:101") into timestamp and notification ID.
func ParseSSEEventID(raw string) (time.Time, int64, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return time.Time{}, 0, ErrInvalidEventID
	}
	ms, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}, 0, ErrInvalidEventID
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return time.Time{}, 0, ErrInvalidEventID
	}
	return time.UnixMilli(ms).UTC(), id, nil
}

