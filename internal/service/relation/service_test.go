package relation_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceaccount "github.com/Tidehyc/gorangefeed/internal/infra/persistence/account"
	persistencerelation "github.com/Tidehyc/gorangefeed/internal/infra/persistence/relation"
	servicerelation "github.com/Tidehyc/gorangefeed/internal/service/relation"
	"gorm.io/gorm"
)

func setupRelationServiceTest(t *testing.T) (*servicerelation.Service, domainaccount.Repository, *gorm.DB) {
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
		t.Skipf("skipping service test: config load failed (%v)", err)
		return nil, nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping service test: MySQL connection failed (%v)", err)
		return nil, nil, nil
	}

	relationRepo := persistencerelation.NewRepository(db)
	accountRepo := persistenceaccount.NewRepository(db)

	svc := servicerelation.NewService(relationRepo, accountRepo)
	return svc, accountRepo, db
}

func createTestUser(t *testing.T, accountRepo domainaccount.Repository) *domainaccount.User {
	t.Helper()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	u := &domainaccount.User{
		Username:     fmt.Sprintf("user_%d", suffix),
		PasswordHash: "hashed_secret",
		Nickname:     fmt.Sprintf("Nick_%d", suffix),
		Status:       domainaccount.UserStatusActive,
	}
	if err := accountRepo.Create(ctx, u); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	return u
}

func TestRelationService_FollowLifecycle(t *testing.T) {
	svc, accountRepo, db := setupRelationServiceTest(t)
	if db == nil {
		return
	}
	ctx := context.Background()

	userA := createTestUser(t, accountRepo)
	userB := createTestUser(t, accountRepo)

	// 1. Self follow is forbidden
	err := svc.Follow(ctx, userA.ID, userA.ID)
	if !errors.Is(err, domainrelation.ErrSelfFollow) {
		t.Errorf("expected ErrSelfFollow, got %v", err)
	}

	// 2. Follow non-existent user
	err = svc.Follow(ctx, userA.ID, 99999999)
	if !errors.Is(err, domainrelation.ErrTargetUserNotFound) {
		t.Errorf("expected ErrTargetUserNotFound, got %v", err)
	}

	// 3. User A follows User B
	if err := svc.Follow(ctx, userA.ID, userB.ID); err != nil {
		t.Fatalf("Follow failed: %v", err)
	}

	// 4. Verify relation
	rel, err := svc.GetRelation(ctx, userA.ID, userB.ID)
	if err != nil || !rel.IsFollowing || rel.IsFollower || rel.IsFriend {
		t.Fatalf("unexpected relation state: %+v (err: %v)", rel, err)
	}

	// 5. User B follows User A => Friendship
	if err := svc.Follow(ctx, userB.ID, userA.ID); err != nil {
		t.Fatalf("User B Follow User A failed: %v", err)
	}

	relMutual, err := svc.GetRelation(ctx, userA.ID, userB.ID)
	if err != nil || !relMutual.IsFriend {
		t.Fatalf("expected mutual friend true, got %+v (err: %v)", relMutual, err)
	}

	// 6. ListFollowings and ListFollowers
	followings, _, _, err := svc.ListFollowings(ctx, userA.ID, userA.ID, 0, 10)
	if err != nil {
		t.Fatalf("ListFollowings failed: %v", err)
	}
	if len(followings) != 1 || followings[0].ID != userB.ID || !followings[0].IsFriend {
		t.Errorf("expected userB in followings with IsFriend=true, got %+v", followings)
	}

	followers, _, _, err := svc.ListFollowers(ctx, userB.ID, userB.ID, 0, 10)
	if err != nil {
		t.Fatalf("ListFollowers failed: %v", err)
	}
	if len(followers) != 1 || followers[0].ID != userA.ID || !followers[0].IsFriend {
		t.Errorf("expected userA in followers with IsFriend=true, got %+v", followers)
	}

	// 7. GetStats
	statsA, err := svc.GetStats(ctx, userA.ID)
	if err != nil || statsA.FollowingCount != 1 || statsA.FollowerCount != 1 {
		t.Fatalf("unexpected statsA: %+v (err: %v)", statsA, err)
	}

	// 8. Unfollow
	if err := svc.Unfollow(ctx, userA.ID, userB.ID); err != nil {
		t.Fatalf("Unfollow failed: %v", err)
	}

	relAfterUnfollow, err := svc.GetRelation(ctx, userA.ID, userB.ID)
	if err != nil || relAfterUnfollow.IsFollowing || relAfterUnfollow.IsFriend {
		t.Fatalf("expected not following and not friend, got %+v", relAfterUnfollow)
	}
}
