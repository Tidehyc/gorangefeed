package notification

import (
	"encoding/base64"
	"encoding/json"
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

// NewNotification constructs and validates a new Notification entity.
func NewNotification(
	recipientID, actorID int64,
	nType, targetType string,
	targetID int64,
	title, content, eventID string,
) (*Notification, error) {
	if recipientID <= 0 {
		return nil, ErrInvalidRecipientID
	}
	if actorID <= 0 && nType != TypeSystem {
		return nil, ErrInvalidActorID
	}
	if recipientID == actorID && nType != TypeSystem {
		return nil, ErrSelfNotification
	}

	nType = strings.ToLower(strings.TrimSpace(nType))
	if !ValidTypes[nType] {
		return nil, ErrInvalidType
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return nil, ErrEmptyTitle
	}
	if len([]rune(title)) > MaxTitleLength {
		return nil, ErrTitleTooLong
	}

	content = strings.TrimSpace(content)
	if len([]rune(content)) > MaxContentLength {
		return nil, ErrContentTooLong
	}

	return &Notification{
		RecipientID: recipientID,
		ActorID:     actorID,
		Type:        nType,
		TargetType:  strings.ToLower(strings.TrimSpace(targetType)),
		TargetID:    targetID,
		Title:       title,
		Content:     content,
		EventID:     strings.TrimSpace(eventID),
		IsRead:      false,
		CreatedAt:   time.Now().UTC(),
	}, nil
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
