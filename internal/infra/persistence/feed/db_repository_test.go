package feed_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistencefeed "github.com/Tidehyc/gorangefeed/internal/infra/persistence/feed"
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

func TestFeedDBRepository(t *testing.T) {
	db, cleanup := setupTestMySQL(t)
	if db == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	repo := persistencefeed.NewFeedDBRepository(db)

	uniqueSuffix := time.Now().UnixNano()
	authorID := uniqueSuffix % 1000000000
	username := fmt.Sprintf("feed_u_%d", authorID)

	// Create test author in users table
	err := db.Exec("INSERT INTO users (id, username, password_hash, nickname, avatar_url, created_at, updated_at) VALUES (?, ?, 'hash', 'Nick', '/media/avatar.png', NOW(), NOW())", authorID, username).Error
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}
	defer func() {
		_ = db.Exec("DELETE FROM users WHERE id = ?", authorID).Error
	}()

	// Insert 3 test videos: v1 (status=1), v2 (status=2 hidden), v3 (status=1)
	type insertVideo struct {
		title  string
		status int8
	}
	videos := []insertVideo{
		{"Feed Video 1", 1},
		{"Feed Video 2 Hidden", 2},
		{"Feed Video 3", 1},
	}

	createdIDs := make([]int64, 0, len(videos))
	for _, iv := range videos {
		res := db.Exec("INSERT INTO videos (author_id, title, play_url, status, published_at, created_at, updated_at) VALUES (?, ?, '/media/v.mp4', ?, NOW(), NOW(), NOW())", authorID, iv.title, iv.status)
		if res.Error != nil {
			t.Fatalf("failed to insert video: %v", res.Error)
		}
		var insertedID int64
		_ = db.Raw("SELECT LAST_INSERT_ID()").Scan(&insertedID)
		createdIDs = append(createdIDs, insertedID)

		// Insert video_stats
		_ = db.Exec("INSERT INTO video_stats (video_id, like_count, comment_count, view_count, created_at, updated_at) VALUES (?, 10, 2, 5, NOW(), NOW())", insertedID)
	}

	defer func() {
		_ = db.Exec("DELETE FROM video_stats WHERE video_id IN (?)", createdIDs).Error
		_ = db.Exec("DELETE FROM videos WHERE id IN (?)", createdIDs).Error
	}()

	v1ID := createdIDs[0]
	v2HiddenID := createdIDs[1]
	v3ID := createdIDs[2]

	// 1. Test GetVideosStatusByIDs
	statusMap, err := repo.GetVideosStatusByIDs(ctx, []int64{v1ID, v2HiddenID, v3ID})
	if err != nil {
		t.Fatalf("GetVideosStatusByIDs failed: %v", err)
	}
	if statusMap[v1ID] != 1 || statusMap[v2HiddenID] != 2 || statusMap[v3ID] != 1 {
		t.Fatalf("mismatched statusMap: %+v", statusMap)
	}

	// 2. Test GetCardsByIDs (hidden v2 shouldn't be returned since status=1 filter)
	cardsMap, err := repo.GetCardsByIDs(ctx, []int64{v1ID, v2HiddenID, v3ID})
	if err != nil {
		t.Fatalf("GetCardsByIDs failed: %v", err)
	}
	if len(cardsMap) != 2 {
		t.Fatalf("expected 2 published cards, got %d", len(cardsMap))
	}
	if cardsMap[v1ID].Title != "Feed Video 1" || cardsMap[v1ID].Author.Username != username {
		t.Fatalf("mismatched card v1: %+v", cardsMap[v1ID])
	}
	if _, ok := cardsMap[v2HiddenID]; ok {
		t.Fatalf("hidden video should not be in cardsMap")
	}

	// 3. Test GetStatsByIDs
	statsMap, err := repo.GetStatsByIDs(ctx, []int64{v1ID, v3ID})
	if err != nil {
		t.Fatalf("GetStatsByIDs failed: %v", err)
	}
	if statsMap[v1ID].ViewCount != 5 || statsMap[v1ID].LikeCount != 10 {
		t.Fatalf("mismatched stats: %+v", statsMap[v1ID])
	}

	// 4. Test IncrViewCount
	if err := repo.IncrViewCount(ctx, v1ID); err != nil {
		t.Fatalf("IncrViewCount failed: %v", err)
	}
	statsAfterIncr, _ := repo.GetStatsByIDs(ctx, []int64{v1ID})
	if statsAfterIncr[v1ID].ViewCount != 6 {
		t.Fatalf("expected view_count 6, got %d", statsAfterIncr[v1ID].ViewCount)
	}

	// 5. Test CalculateFloor (72h duration)
	floorID, err := repo.CalculateFloor(ctx, 72*time.Hour)
	if err != nil {
		t.Fatalf("CalculateFloor failed: %v", err)
	}
	if floorID <= 0 {
		t.Fatalf("expected floorID > 0, got %d", floorID)
	}

	// 6. Test ListColdVideos
	cards, stats, nextCursor, _, err := repo.ListColdVideos(ctx, 0, 10)
	if err != nil {
		t.Fatalf("ListColdVideos failed: %v", err)
	}
	if len(cards) == 0 || len(stats) != len(cards) {
		t.Fatalf("expected cold videos returned, got %d cards, %d stats", len(cards), len(stats))
	}
	if nextCursor <= 0 {
		t.Fatalf("expected nextCursor > 0, got %d", nextCursor)
	}
}
