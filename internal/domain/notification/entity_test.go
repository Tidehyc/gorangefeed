package notification_test

import (
	"fmt"
	"testing"
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
)

func TestNewNotification_Validation(t *testing.T) {
	tests := []struct {
		name        string
		recipientID int64
		actorID     int64
		nType       string
		targetType  string
		targetID    int64
		title       string
		content     string
		eventID     string
		wantErr     error
	}{
		{
			name:        "valid like notification",
			recipientID: 100,
			actorID:     200,
			nType:       domainnotification.TypeLike,
			targetType:  domainnotification.TargetVideo,
			targetID:    1,
			title:       "点赞了你的视频",
			content:     "精彩视频内容",
			eventID:     "evt-1",
			wantErr:     nil,
		},
		{
			name:        "invalid recipient id",
			recipientID: 0,
			actorID:     200,
			nType:       domainnotification.TypeLike,
			title:       "test",
			wantErr:     domainnotification.ErrInvalidRecipientID,
		},
		{
			name:        "invalid actor id for non-system",
			recipientID: 100,
			actorID:     0,
			nType:       domainnotification.TypeLike,
			title:       "test",
			wantErr:     domainnotification.ErrInvalidActorID,
		},
		{
			name:        "self notification rejected",
			recipientID: 100,
			actorID:     100,
			nType:       domainnotification.TypeComment,
			title:       "test",
			wantErr:     domainnotification.ErrSelfNotification,
		},
		{
			name:        "system notification allowed with 0 actor",
			recipientID: 100,
			actorID:     0,
			nType:       domainnotification.TypeSystem,
			title:       "系统维护公告",
			content:     "系统将于今晚维护",
			wantErr:     nil,
		},
		{
			name:        "invalid notification type",
			recipientID: 100,
			actorID:     200,
			nType:       "unknown_type",
			title:       "test",
			wantErr:     domainnotification.ErrInvalidType,
		},
		{
			name:        "empty title",
			recipientID: 100,
			actorID:     200,
			nType:       domainnotification.TypeFollow,
			title:       "   ",
			wantErr:     domainnotification.ErrEmptyTitle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := domainnotification.NewNotification(
				tt.recipientID, tt.actorID,
				tt.nType, tt.targetType, tt.targetID,
				tt.title, tt.content, tt.eventID,
			)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				if n != nil {
					t.Fatalf("expected nil notification on error, got %v", n)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if n.IsRead {
					t.Fatalf("expected new notification to be unread")
				}
			}
		})
	}
}

func TestNotification_MarkAsReadAndWithActor(t *testing.T) {
	n, err := domainnotification.NewNotification(
		100, 200,
		domainnotification.TypeComment, domainnotification.TargetComment, 10,
		"回复了你的评论", "说得太对了！", "evt-comment-1",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	actor := &domainnotification.ActorInfo{
		ID:        200,
		Username:  "bob",
		Nickname:  "Bob Builder",
		AvatarURL: "https://example.com/bob.jpg",
	}
	n.WithActor(actor)
	if n.Actor == nil || n.Actor.Username != "bob" {
		t.Fatalf("expected actor bob, got %v", n.Actor)
	}

	now := time.Now().UTC()
	n.MarkAsRead(now)
	if !n.IsRead {
		t.Fatalf("expected is_read true")
	}
	if n.ReadAt == nil || !n.ReadAt.Equal(now) {
		t.Fatalf("expected read_at %v, got %v", now, n.ReadAt)
	}

	// Idempotent mark as read
	n.MarkAsRead(now.Add(time.Hour))
	if !n.ReadAt.Equal(now) {
		t.Fatalf("expected read_at unchanged on repeated MarkAsRead")
	}
}

func TestNotificationCursor_EncodeDecode(t *testing.T) {
	ts := time.Date(2026, 11, 2, 12, 0, 0, 123456000, time.UTC)
	cursor := &domainnotification.NotificationCursor{
		CreatedAt: ts,
		ID:        98765,
	}

	encoded := cursor.Encode()
	if encoded == "" {
		t.Fatalf("expected non-empty encoded string")
	}

	decoded, err := domainnotification.DecodeNotificationCursor(encoded)
	if err != nil {
		t.Fatalf("failed to decode cursor: %v", err)
	}
	if decoded.ID != cursor.ID {
		t.Fatalf("expected id %d, got %d", cursor.ID, decoded.ID)
	}
	if !decoded.CreatedAt.Equal(cursor.CreatedAt) {
		t.Fatalf("expected created_at %v, got %v", cursor.CreatedAt, decoded.CreatedAt)
	}

	// Test nil and empty decodes
	nilDecoded, err := domainnotification.DecodeNotificationCursor("")
	if err != nil || nilDecoded != nil {
		t.Fatalf("expected nil for empty cursor string, got %v, %v", nilDecoded, err)
	}

	// Test invalid base64 / json
	_, err = domainnotification.DecodeNotificationCursor("invalid_base64_!!!")
	if err != domainnotification.ErrInvalidCursor {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestNotification_SSEEventID(t *testing.T) {
	ts := time.Date(2026, 11, 2, 15, 30, 0, 0, time.UTC)
	n := &domainnotification.Notification{
		ID:        42,
		CreatedAt: ts,
	}

	sseID := n.SSEEventID()
	expectedPrefix := fmt.Sprintf("%d:42", ts.UnixMilli())
	if sseID != expectedPrefix {
		t.Fatalf("expected %s, got %s", expectedPrefix, sseID)
	}

	parsedTime, parsedID, err := domainnotification.ParseSSEEventID(sseID)
	if err != nil {
		t.Fatalf("unexpected error parsing SSEEventID: %v", err)
	}
	if parsedID != 42 {
		t.Fatalf("expected id 42, got %d", parsedID)
	}
	if parsedTime.UnixMilli() != ts.UnixMilli() {
		t.Fatalf("expected time %v, got %v", ts, parsedTime)
	}

	// Invalid inputs
	_, _, err = domainnotification.ParseSSEEventID("")
	if err != domainnotification.ErrInvalidEventID {
		t.Fatalf("expected ErrInvalidEventID, got %v", err)
	}

	_, _, err = domainnotification.ParseSSEEventID("abc:def")
	if err != domainnotification.ErrInvalidEventID {
		t.Fatalf("expected ErrInvalidEventID, got %v", err)
	}

	_, _, err = domainnotification.ParseSSEEventID("12345")
	if err != domainnotification.ErrInvalidEventID {
		t.Fatalf("expected ErrInvalidEventID, got %v", err)
	}
}

