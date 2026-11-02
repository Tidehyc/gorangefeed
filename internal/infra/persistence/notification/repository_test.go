package notification_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistencenotification "github.com/Tidehyc/gorangefeed/internal/infra/persistence/notification"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, domainnotification.Repository) {
	t.Helper()

	var configPath string
	for _, candidate := range []string{
		"../../../../configs/config.yaml",
		"../../../configs/config.yaml",
		"../../configs/config.yaml",
		"configs/config.yaml",
	} {
		if _, err := os.Stat(candidate); err == nil {
			configPath = candidate
			break
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Skipf("skipping notification persistence test: config load failed (%v)", err)
		return nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping notification persistence test: MySQL connection failed (%v)", err)
		return nil, nil
	}

	repo := persistencenotification.NewRepository(db)
	return db, repo
}

func TestGormNotificationRepository_LifecycleAndIdempotency(t *testing.T) {
	db, repo := setupTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()

	uniqueSeed := time.Now().UnixNano()
	recipientID := uniqueSeed
	actorID := uniqueSeed + 1

	// Insert test actor user in `users` table for actor hydration assertion
	username := fmt.Sprintf("actor_%d", uniqueSeed)
	if err := db.Exec("INSERT INTO users (id, username, password_hash, nickname, avatar_url, bio, status) VALUES (?, ?, 'hash', 'Test Actor', 'https://avatar.png', 'bio', 'active')",
		actorID, username).Error; err != nil {
		t.Fatalf("failed to insert test actor: %v", err)
	}
	defer func() {
		_ = db.Exec("DELETE FROM users WHERE id = ?", actorID)
		_ = db.Exec("DELETE FROM notifications WHERE recipient_id = ?", recipientID)
	}()

	// 1. Create single notification
	n1, err := domainnotification.NewNotification(
		recipientID, actorID,
		domainnotification.TypeLike, domainnotification.TargetVideo, 100,
		"点赞了你的视频", "视频很棒", fmt.Sprintf("evt-like-%d", uniqueSeed),
	)
	if err != nil {
		t.Fatalf("unexpected NewNotification error: %v", err)
	}

	if err := repo.Create(ctx, n1); err != nil {
		t.Fatalf("failed to create notification: %v", err)
	}
	if n1.ID <= 0 {
		t.Fatalf("expected positive notification ID, got %d", n1.ID)
	}

	// 2. Duplicate Create with same eventID must be idempotent
	nDup, _ := domainnotification.NewNotification(
		recipientID, actorID,
		domainnotification.TypeLike, domainnotification.TargetVideo, 100,
		"点赞了你的视频", "视频很棒", n1.EventID,
	)
	if err := repo.Create(ctx, nDup); err != nil {
		t.Fatalf("duplicate create should succeed idempotently, got error: %v", err)
	}
	if nDup.ID != n1.ID {
		t.Fatalf("expected duplicate to resolve to existing id %d, got %d", n1.ID, nDup.ID)
	}

	// 3. BatchCreate multiple notifications
	n2, _ := domainnotification.NewNotification(
		recipientID, actorID,
		domainnotification.TypeComment, domainnotification.TargetComment, 200,
		"评论了你的视频", "写得很好", fmt.Sprintf("evt-cmt-%d", uniqueSeed),
	)
	n3, _ := domainnotification.NewNotification(
		recipientID, actorID,
		domainnotification.TypeMention, domainnotification.TargetComment, 300,
		"在评论中提及了你", "@你 快来看", fmt.Sprintf("evt-men-%d", uniqueSeed),
	)
	n4, _ := domainnotification.NewNotification(
		recipientID, actorID,
		domainnotification.TypeFollow, domainnotification.TargetUser, actorID,
		"关注了你", "", fmt.Sprintf("evt-fol-%d", uniqueSeed),
	)

	if err := repo.BatchCreate(ctx, []*domainnotification.Notification{n2, n3, n4}); err != nil {
		t.Fatalf("failed to batch create notifications: %v", err)
	}

	// 4. CountUnread
	unreadCount, err := repo.CountUnread(ctx, recipientID)
	if err != nil {
		t.Fatalf("failed to count unread: %v", err)
	}
	if unreadCount != 4 {
		t.Fatalf("expected 4 unread notifications, got %d", unreadCount)
	}

	// 5. ListByRecipient with cursor pagination
	page1, nextCursor, hasMore, err := repo.ListByRecipient(ctx, recipientID, nil, 2, "")
	if err != nil {
		t.Fatalf("failed to list page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 items on page 1, got %d", len(page1))
	}
	if !hasMore {
		t.Fatalf("expected hasMore=true on page 1")
	}
	if nextCursor == nil {
		t.Fatalf("expected valid nextCursor")
	}
	// Verify Actor hydration
	if page1[0].Actor == nil || page1[0].Actor.Nickname != "Test Actor" {
		t.Fatalf("expected hydrated actor with nickname 'Test Actor', got %v", page1[0].Actor)
	}

	page2, nextCursor2, hasMore2, err := repo.ListByRecipient(ctx, recipientID, nextCursor, 2, "")
	if err != nil {
		t.Fatalf("failed to list page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(page2))
	}
	if hasMore2 {
		t.Fatalf("expected hasMore2=false on page 2")
	}
	_ = nextCursor2

	// 6. Filter by type
	likePage, _, _, err := repo.ListByRecipient(ctx, recipientID, nil, 10, domainnotification.TypeLike)
	if err != nil {
		t.Fatalf("failed to list filtered likes: %v", err)
	}
	if len(likePage) != 1 || likePage[0].Type != domainnotification.TypeLike {
		t.Fatalf("expected exactly 1 like notification, got %d", len(likePage))
	}

	// 7. Mark single notification as read
	marked, err := repo.MarkAsRead(ctx, recipientID, []int64{n1.ID})
	if err != nil {
		t.Fatalf("failed to mark as read: %v", err)
	}
	if marked != 1 {
		t.Fatalf("expected 1 record marked as read, got %d", marked)
	}

	unreadCountAfterSingle, err := repo.CountUnread(ctx, recipientID)
	if err != nil {
		t.Fatalf("failed to count unread: %v", err)
	}
	if unreadCountAfterSingle != 3 {
		t.Fatalf("expected 3 unread notifications, got %d", unreadCountAfterSingle)
	}

	// 8. ListAfter (Catch-up replay test)
	// Query unread after n1.CreatedAt - 1 second
	catchUpItems, err := repo.ListAfter(ctx, recipientID, n1.CreatedAt.Add(-time.Second), 0, 10)
	if err != nil {
		t.Fatalf("failed to list after: %v", err)
	}
	// n1 was marked as read, so only remaining 3 unread items should be returned
	if len(catchUpItems) != 3 {
		t.Fatalf("expected 3 unread catch-up items, got %d", len(catchUpItems))
	}

	// 9. MarkAllAsRead
	allMarked, err := repo.MarkAllAsRead(ctx, recipientID)
	if err != nil {
		t.Fatalf("failed to mark all as read: %v", err)
	}
	if allMarked != 3 {
		t.Fatalf("expected 3 records marked as read, got %d", allMarked)
	}

	finalUnread, err := repo.CountUnread(ctx, recipientID)
	if err != nil {
		t.Fatalf("failed to count unread: %v", err)
	}
	if finalUnread != 0 {
		t.Fatalf("expected 0 unread notifications, got %d", finalUnread)
	}
}
