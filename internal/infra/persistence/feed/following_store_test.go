package feed_test

import (
	"context"
	"os"
	"testing"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistencefeed "github.com/Tidehyc/gorangefeed/internal/infra/persistence/feed"
	infraredis "github.com/Tidehyc/gorangefeed/internal/infra/redis"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func setupFollowingStore(t *testing.T) (*redis.Client, *gorm.DB, domainfeed.FollowingFeedRepository) {
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
		t.Skipf("skipping following store test: config load failed (%v)", err)
		return nil, nil, nil
	}

	rdb, err := infraredis.New(&cfg.Redis)
	if err != nil {
		t.Skipf("skipping following store test: Redis connection failed (%v)", err)
		return nil, nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping following store test: MySQL connection failed (%v)", err)
		return nil, nil, nil
	}

	store := persistencefeed.NewFollowingStore(rdb, db)
	return rdb, db, store
}

func TestFollowingStore_InboxLifecycle(t *testing.T) {
	rdb, _, store := setupFollowingStore(t)
	if store == nil {
		return
	}
	ctx := context.Background()
	userA := int64(88801)
	userB := int64(88802)

	defer func() {
		rdb.Del(ctx, "feed:following:inbox:88801", "feed:following:inbox:88802")
	}()

	// 1. Initially inbox does not exist
	exists, err := store.CheckInboxExists(ctx, userA)
	if err != nil {
		t.Fatalf("CheckInboxExists failed: %v", err)
	}
	if exists {
		t.Errorf("expected inbox not to exist initially")
	}

	// 2. Push to inboxes
	followerIDs := []int64{userA, userB}
	if err := store.PushToInboxes(ctx, followerIDs, 1001, 800); err != nil {
		t.Fatalf("PushToInboxes 1001 failed: %v", err)
	}
	if err := store.PushToInboxes(ctx, followerIDs, 1002, 800); err != nil {
		t.Fatalf("PushToInboxes 1002 failed: %v", err)
	}
	if err := store.PushToInboxes(ctx, followerIDs, 1003, 800); err != nil {
		t.Fatalf("PushToInboxes 1003 failed: %v", err)
	}

	// 3. Check inbox exists
	exists, err = store.CheckInboxExists(ctx, userA)
	if err != nil {
		t.Fatalf("CheckInboxExists failed: %v", err)
	}
	if !exists {
		t.Errorf("expected inbox to exist after push")
	}

	// 4. Query inbox with cursor pagination (cursor=0 -> newest first)
	ids, err := store.GetInboxVideoIDs(ctx, userA, 0, 2)
	if err != nil {
		t.Fatalf("GetInboxVideoIDs failed: %v", err)
	}
	if len(ids) != 2 || ids[0] != 1003 || ids[1] != 1002 {
		t.Fatalf("unexpected ids: %v", ids)
	}

	// Cursor query: cursor=1002 -> strictly < 1002
	ids, err = store.GetInboxVideoIDs(ctx, userA, 1002, 2)
	if err != nil {
		t.Fatalf("GetInboxVideoIDs cursor failed: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1001 {
		t.Fatalf("unexpected ids after cursor 1002: %v", ids)
	}

	// 5. Prewarm inbox test
	newIDs := []int64{2001, 2002, 2003}
	if err := store.PrewarmInbox(ctx, userA, newIDs, 800); err != nil {
		t.Fatalf("PrewarmInbox failed: %v", err)
	}
	warmedIDs, err := store.GetInboxVideoIDs(ctx, userA, 0, 10)
	if err != nil {
		t.Fatalf("GetInboxVideoIDs after prewarm failed: %v", err)
	}
	if len(warmedIDs) != 6 { // 1001, 1002, 1003, 2001, 2002, 2003
		t.Errorf("expected 6 video IDs, got %d: %v", len(warmedIDs), warmedIDs)
	}
}

func TestFollowingStore_AuthorOutbox(t *testing.T) {
	rdb, _, store := setupFollowingStore(t)
	if store == nil {
		return
	}
	ctx := context.Background()
	authorID := int64(99901)

	defer func() {
		rdb.Del(ctx, "feed:following:author:99901")
	}()

	// 1. Push to author outbox
	if err := store.PushToAuthorOutbox(ctx, authorID, 5001, 800); err != nil {
		t.Fatalf("PushToAuthorOutbox 5001 failed: %v", err)
	}
	if err := store.PushToAuthorOutbox(ctx, authorID, 5002, 800); err != nil {
		t.Fatalf("PushToAuthorOutbox 5002 failed: %v", err)
	}

	// 2. Query author outbox
	ids, err := store.GetAuthorOutboxVideoIDs(ctx, authorID, 0, 10)
	if err != nil {
		t.Fatalf("GetAuthorOutboxVideoIDs failed: %v", err)
	}
	if len(ids) != 2 || ids[0] != 5002 || ids[1] != 5001 {
		t.Fatalf("unexpected author outbox ids: %v", ids)
	}
}
