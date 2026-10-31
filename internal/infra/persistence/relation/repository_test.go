package relation_test

import (
	"context"
	"os"
	"testing"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistencerelation "github.com/Tidehyc/gorangefeed/internal/infra/persistence/relation"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, domainrelation.Repository) {
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
		t.Skipf("skipping persistence test: config load failed (%v)", err)
		return nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping persistence test: MySQL connection failed (%v)", err)
		return nil, nil
	}

	repo := persistencerelation.NewRepository(db)
	return db, repo
}

func TestGormRelationRepository_FollowLifecycleAndFriendship(t *testing.T) {
	db, repo := setupTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()

	userA := time.Now().UnixNano()
	userB := userA + 1

	// 1. Self-follow must be rejected
	_, err := repo.Follow(ctx, userA, userA, nil)
	if err != domainrelation.ErrSelfFollow {
		t.Fatalf("expected ErrSelfFollow, got %v", err)
	}

	// 2. User A follows User B
	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeRelationFollowed, map[string]any{
		"follower_id": userA,
		"vlogger_id":  userB,
	})
	if err != nil {
		t.Fatalf("failed to create outbox event: %v", err)
	}

	followed, err := repo.Follow(ctx, userA, userB, outboxEvent)
	if err != nil {
		t.Fatalf("Follow failed: %v", err)
	}
	if !followed {
		t.Errorf("expected followed true on first follow")
	}

	// Check stats for A and B
	statsA, err := repo.GetStats(ctx, userA)
	if err != nil || statsA.FollowingCount != 1 || statsA.FollowerCount != 0 {
		t.Fatalf("expected statsA following=1 follower=0, got following=%d follower=%d (err: %v)",
			statsA.FollowingCount, statsA.FollowerCount, err)
	}

	statsB, err := repo.GetStats(ctx, userB)
	if err != nil || statsB.FollowingCount != 0 || statsB.FollowerCount != 1 {
		t.Fatalf("expected statsB following=0 follower=1, got following=%d follower=%d (err: %v)",
			statsB.FollowingCount, statsB.FollowerCount, err)
	}

	// 3. Check Friendship before mutual follow
	relAB, err := repo.GetRelation(ctx, userA, userB)
	if err != nil {
		t.Fatalf("GetRelation failed: %v", err)
	}
	if !relAB.IsFollowing || relAB.IsFollower || relAB.IsFriend {
		t.Errorf("expected A->B isFollowing=true isFollower=false isFriend=false, got %+v", relAB)
	}

	// 4. Duplicate follow must be idempotent
	followedAgain, err := repo.Follow(ctx, userA, userB, nil)
	if err != nil {
		t.Fatalf("duplicate Follow failed: %v", err)
	}
	if followedAgain {
		t.Errorf("expected duplicate follow to return false")
	}

	// 5. User B follows User A (Mutual Follow => Friendship)
	followedB, err := repo.Follow(ctx, userB, userA, nil)
	if err != nil || !followedB {
		t.Fatalf("User B follow User A failed: %v", err)
	}

	relABMutual, err := repo.GetRelation(ctx, userA, userB)
	if err != nil {
		t.Fatalf("GetRelation mutual failed: %v", err)
	}
	if !relABMutual.IsFollowing || !relABMutual.IsFollower || !relABMutual.IsFriend {
		t.Errorf("expected mutual friendship true, got %+v", relABMutual)
	}

	// 6. BatchIsFollowed
	followMap, err := repo.BatchIsFollowed(ctx, userA, []int64{userB, 9999999})
	if err != nil {
		t.Fatalf("BatchIsFollowed failed: %v", err)
	}
	if !followMap[userB] || followMap[9999999] {
		t.Errorf("expected userB true and 9999999 false, got %v", followMap)
	}

	// 7. ListFollowingIDs and ListFollowerIDs
	followingsA, nextCursor, hasMore, err := repo.ListFollowingIDs(ctx, userA, 0, 10)
	if err != nil {
		t.Fatalf("ListFollowingIDs failed: %v", err)
	}
	if len(followingsA) != 1 || followingsA[0] != userB {
		t.Errorf("expected followingsA to contain userB, got %v", followingsA)
	}
	if nextCursor <= 0 || hasMore {
		t.Errorf("unexpected cursor=%d hasMore=%v", nextCursor, hasMore)
	}

	followersB, _, _, err := repo.ListFollowerIDs(ctx, userB, 0, 10)
	if err != nil {
		t.Fatalf("ListFollowerIDs failed: %v", err)
	}
	if len(followersB) != 1 || followersB[0] != userA {
		t.Errorf("expected followersB to contain userA, got %v", followersB)
	}

	// 8. Unfollow
	unfollowed, err := repo.Unfollow(ctx, userA, userB, nil)
	if err != nil || !unfollowed {
		t.Fatalf("Unfollow failed: %v (unfollowed: %v)", err, unfollowed)
	}

	relABAfterUnfollow, err := repo.GetRelation(ctx, userA, userB)
	if err != nil {
		t.Fatalf("GetRelation after unfollow failed: %v", err)
	}
	if relABAfterUnfollow.IsFollowing || !relABAfterUnfollow.IsFollower || relABAfterUnfollow.IsFriend {
		t.Errorf("expected A not following B, B still following A, isFriend=false, got %+v", relABAfterUnfollow)
	}

	// 9. Duplicate unfollow must be idempotent
	unfollowedAgain, err := repo.Unfollow(ctx, userA, userB, nil)
	if err != nil {
		t.Fatalf("duplicate Unfollow failed: %v", err)
	}
	if unfollowedAgain {
		t.Errorf("expected duplicate unfollow to return false")
	}
}
