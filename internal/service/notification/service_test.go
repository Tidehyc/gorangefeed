package notification_test

import (
	"context"
	"testing"
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	infraredis "github.com/Tidehyc/gorangefeed/internal/infra/redis"
	servicenotification "github.com/Tidehyc/gorangefeed/internal/service/notification"
)

type mockNotificationRepo struct {
	items       map[int64]*domainnotification.Notification
	listItems   []*domainnotification.Notification
	unreadCount int64
	markedIDs   []int64
	allMarked   bool
	createErr   error
}

func newMockRepo() *mockNotificationRepo {
	return &mockNotificationRepo{
		items:     make(map[int64]*domainnotification.Notification),
		listItems: make([]*domainnotification.Notification, 0),
	}
}

func (m *mockNotificationRepo) Create(ctx context.Context, n *domainnotification.Notification) error {
	if m.createErr != nil {
		return m.createErr
	}
	if n.ID == 0 {
		n.ID = int64(len(m.items) + 1)
	}
	m.items[n.ID] = n
	return nil
}

func (m *mockNotificationRepo) BatchCreate(ctx context.Context, notifications []*domainnotification.Notification) error {
	for _, n := range notifications {
		_ = m.Create(ctx, n)
	}
	return nil
}

func (m *mockNotificationRepo) ListByRecipient(
	ctx context.Context,
	recipientID int64,
	cursor *domainnotification.NotificationCursor,
	limit int,
	filterType string,
) ([]*domainnotification.Notification, *domainnotification.NotificationCursor, bool, error) {
	result := make([]*domainnotification.Notification, 0)
	for _, item := range m.listItems {
		if filterType != "" && item.Type != filterType {
			continue
		}
		if cursor != nil {
			if !item.CreatedAt.Before(cursor.CreatedAt) && !(item.CreatedAt.Equal(cursor.CreatedAt) && item.ID < cursor.ID) {
				continue
			}
		}
		result = append(result, item)
		if len(result) >= limit+1 {
			break
		}
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}

	var nextCursor *domainnotification.NotificationCursor
	if len(result) > 0 {
		last := result[len(result)-1]
		nextCursor = &domainnotification.NotificationCursor{
			CreatedAt: last.CreatedAt,
			ID:        last.ID,
		}
	}

	return result, nextCursor, hasMore, nil
}

func (m *mockNotificationRepo) CountUnread(ctx context.Context, recipientID int64) (int64, error) {
	return m.unreadCount, nil
}

func (m *mockNotificationRepo) MarkAsRead(ctx context.Context, recipientID int64, notificationIDs []int64) (int64, error) {
	m.markedIDs = append(m.markedIDs, notificationIDs...)
	for _, id := range notificationIDs {
		if n, ok := m.items[id]; ok {
			n.IsRead = true
		}
	}
	return int64(len(notificationIDs)), nil
}

func (m *mockNotificationRepo) MarkAllAsRead(ctx context.Context, recipientID int64) (int64, error) {
	m.allMarked = true
	count := m.unreadCount
	m.unreadCount = 0
	return count, nil
}

func (m *mockNotificationRepo) ListAfter(ctx context.Context, recipientID int64, after time.Time, minID int64, limit int) ([]*domainnotification.Notification, error) {
	return nil, nil
}

func TestService_ListNotifications_Pagination(t *testing.T) {
	repo := newMockRepo()
	mb := infraredis.NewMemoryBroadcaster()
	defer mb.Close()

	svc, err := servicenotification.NewService(repo, mb)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	baseTime := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	// Seed 25 notifications in reverse chronological order
	for i := 25; i >= 1; i-- {
		n := &domainnotification.Notification{
			ID:          int64(i),
			RecipientID: 100,
			ActorID:     200,
			Type:        domainnotification.TypeLike,
			TargetType:  domainnotification.TargetVideo,
			TargetID:    300,
			Title:       "点赞",
			Content:     "赞了你的视频",
			CreatedAt:   baseTime.Add(time.Duration(i) * time.Minute),
		}
		repo.listItems = append(repo.listItems, n)
	}

	ctx := context.Background()

	// 1. Fetch first page (limit 10)
	page1, cursor1, hasMore1, err := svc.ListNotifications(ctx, 100, "", "", 10)
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(page1) != 10 {
		t.Fatalf("expected 10 items on page 1, got %d", len(page1))
	}
	if !hasMore1 {
		t.Fatalf("expected hasMore true on page 1")
	}
	if cursor1 == "" {
		t.Fatalf("expected non-empty cursor on page 1")
	}

	// 2. Fetch second page (limit 10)
	page2, cursor2, hasMore2, err := svc.ListNotifications(ctx, 100, "", cursor1, 10)
	if err != nil {
		t.Fatalf("page 2 failed: %v", err)
	}
	if len(page2) != 10 {
		t.Fatalf("expected 10 items on page 2, got %d", len(page2))
	}
	if !hasMore2 {
		t.Fatalf("expected hasMore true on page 2")
	}
	if page2[0].ID != page1[9].ID-1 {
		t.Fatalf("expected item ID %d following page 1, got %d", page1[9].ID-1, page2[0].ID)
	}

	// 3. Fetch third page (limit 10) -> should have remaining 5
	page3, _, hasMore3, err := svc.ListNotifications(ctx, 100, "", cursor2, 10)
	if err != nil {
		t.Fatalf("page 3 failed: %v", err)
	}
	if len(page3) != 5 {
		t.Fatalf("expected 5 items on page 3, got %d", len(page3))
	}
	if hasMore3 {
		t.Fatalf("expected hasMore false on page 3")
	}
}

func TestService_UnreadAndMarkAsRead(t *testing.T) {
	repo := newMockRepo()
	repo.unreadCount = 5
	svc, err := servicenotification.NewService(repo, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Unread count
	count, err := svc.GetUnreadCount(ctx, 100)
	if err != nil || count != 5 {
		t.Fatalf("expected 5 unread, got %d, err %v", count, err)
	}

	// Mark single
	if err := svc.MarkAsRead(ctx, 100, 10); err != nil {
		t.Fatalf("mark as read failed: %v", err)
	}
	if len(repo.markedIDs) != 1 || repo.markedIDs[0] != 10 {
		t.Fatalf("expected marked ID 10, got %v", repo.markedIDs)
	}

	// Mark batch
	if _, err := svc.MarkBatchAsRead(ctx, 100, []int64{11, 12}); err != nil {
		t.Fatalf("mark batch failed: %v", err)
	}
	if len(repo.markedIDs) != 3 {
		t.Fatalf("expected 3 total marked IDs, got %v", repo.markedIDs)
	}

	// Mark all
	markedCount, err := svc.MarkAllAsRead(ctx, 100)
	if err != nil || markedCount != 5 {
		t.Fatalf("expected marked count 5, got %d, err %v", markedCount, err)
	}
	if !repo.allMarked {
		t.Fatalf("expected allMarked to be true")
	}

	// Invalid inputs
	if _, err := svc.GetUnreadCount(ctx, 0); err != domainnotification.ErrInvalidRecipientID {
		t.Fatalf("expected ErrInvalidRecipientID, got %v", err)
	}
	if err := svc.MarkAsRead(ctx, 0, 10); err != domainnotification.ErrInvalidRecipientID {
		t.Fatalf("expected ErrInvalidRecipientID, got %v", err)
	}
	if err := svc.MarkAsRead(ctx, 100, 0); err != domainnotification.ErrInvalidNotificationID {
		t.Fatalf("expected ErrInvalidNotificationID, got %v", err)
	}
}

func TestService_CreateAndBroadcast(t *testing.T) {
	repo := newMockRepo()
	mb := infraredis.NewMemoryBroadcaster()
	defer mb.Close()

	svc, err := servicenotification.NewService(repo, mb)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sub, err := mb.Subscribe(ctx)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	notif := &domainnotification.Notification{
		RecipientID: 100,
		ActorID:     200,
		Type:        domainnotification.TypeFollow,
		TargetType:  domainnotification.TargetUser,
		TargetID:    200,
		Title:       "关注通知",
		Content:     "关注了你",
		CreatedAt:   time.Now().UTC(),
	}

	if err := svc.CreateNotification(ctx, notif); err != nil {
		t.Fatalf("failed to create notification: %v", err)
	}

	select {
	case received := <-sub:
		if received.RecipientID != 100 || received.Type != domainnotification.TypeFollow {
			t.Fatalf("unexpected broadcast notification: %v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for broadcast")
	}
}
