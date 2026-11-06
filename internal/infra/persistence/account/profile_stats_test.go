package account_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceaccount "github.com/Tidehyc/gorangefeed/internal/infra/persistence/account"
	"gorm.io/gorm"
)

func setupTestMySQL(t *testing.T) (*gorm.DB, func()) {
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
		t.Skipf("skipping db test: config load failed (%v)", err)
		return nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping db test: MySQL connection failed (%v)", err)
		return nil, nil
	}

	cleanup := func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	return db, cleanup
}

func TestProfileStatsRepository_AggregatesCounters(t *testing.T) {
	db, cleanup := setupTestMySQL(t)
	if db == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	repo := persistenceaccount.NewProfileStatsRepository(db)

	base := time.Now().UnixNano() % 1000000000
	authorID := base
	bystanderID := base + 1
	username := fmt.Sprintf("stats_u_%d", authorID)

	if err := db.Exec("INSERT INTO users (id, username, password_hash, nickname, avatar_url, created_at, updated_at) VALUES (?, ?, 'hash', 'Stats Author', '', NOW(), NOW())", authorID, username).Error; err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}
	defer func() {
		_ = db.Exec("DELETE FROM users WHERE id = ?", authorID).Error
	}()

	// relation_stats is the source of both follow counters; the repository must not recompute
	// them from the follows table.
	if err := db.Exec("INSERT INTO relation_stats (user_id, following_count, follower_count, created_at, updated_at) VALUES (?, 3, 5, NOW(), NOW())", authorID).Error; err != nil {
		t.Fatalf("failed to insert relation stats: %v", err)
	}
	defer func() {
		_ = db.Exec("DELETE FROM relation_stats WHERE user_id = ?", authorID).Error
	}()

	// Two published videos (10 + 5 likes) and one hidden video with a large like count. The
	// hidden one must be excluded from both work_count and total_likes.
	type seededVideo struct {
		status int8
		likes  int64
	}
	seeded := []seededVideo{{1, 10}, {1, 5}, {2, 100}}
	for _, v := range seeded {
		res := db.Exec("INSERT INTO videos (author_id, title, play_url, status, published_at, created_at, updated_at) VALUES (?, 'Stats video', '/media/v.mp4', ?, NOW(), NOW(), NOW())", authorID, v.status)
		if res.Error != nil {
			t.Fatalf("failed to insert video: %v", res.Error)
		}
		var videoID int64
		if err := db.Raw("SELECT LAST_INSERT_ID()").Scan(&videoID).Error; err != nil {
			t.Fatalf("failed to read inserted video id: %v", err)
		}
		if err := db.Exec("INSERT INTO video_stats (video_id, like_count, comment_count, view_count, favorite_count, created_at, updated_at) VALUES (?, ?, 0, 0, 0, NOW(), NOW())", videoID, v.likes).Error; err != nil {
			t.Fatalf("failed to insert video stats: %v", err)
		}
		vid := videoID
		defer func() {
			_ = db.Exec("DELETE FROM video_stats WHERE video_id = ?", vid).Error
			_ = db.Exec("DELETE FROM videos WHERE id = ?", vid).Error
		}()
	}

	got, err := repo.GetProfileStats(ctx, authorID)
	if err != nil {
		t.Fatalf("GetProfileStats failed: %v", err)
	}
	want := domainaccount.ProfileStats{FollowingCount: 3, FollowerCount: 5, WorkCount: 2, TotalLikes: 15}
	if got != want {
		t.Fatalf("expected %+v, got %+v", want, got)
	}

	// A user with no relation row and no videos must report zeroes, not an error.
	bystander, err := repo.GetProfileStats(ctx, bystanderID)
	if err != nil {
		t.Fatalf("GetProfileStats for a bare user failed: %v", err)
	}
	if bystander != (domainaccount.ProfileStats{}) {
		t.Fatalf("expected zeroes for a bare user, got %+v", bystander)
	}
}
