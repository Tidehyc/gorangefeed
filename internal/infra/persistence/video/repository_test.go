package video_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistencevideo "github.com/Tidehyc/gorangefeed/internal/infra/persistence/video"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, domainvideo.Repository) {
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

	repo := persistencevideo.NewRepository(db)
	return db, repo
}

func TestGormRepository_CreateWithStatsAndIdempotency(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	authorID := time.Now().UnixNano()
	idempotencyKey := fmt.Sprintf("test_idem_%d", authorID)

	video := &domainvideo.Video{
		AuthorID:       authorID,
		Title:          "Test Video 1",
		Description:    "Test Video Description",
		CoverURL:       "http://example.com/media/covers/1.jpg",
		PlayURL:        "http://example.com/media/videos/1.mp4",
		DurationMs:     12000,
		Status:         domainvideo.VideoStatusPublished,
		IdempotencyKey: &idempotencyKey,
	}
	stats := &domainvideo.VideoStats{
		LikeCount:    0,
		CommentCount: 0,
		ViewCount:    0,
	}

	// 1. First creation must succeed
	if err := repo.CreateWithStats(ctx, video, stats); err != nil {
		t.Fatalf("CreateWithStats failed: %v", err)
	}
	if video.ID <= 0 {
		t.Fatalf("expected positive video.ID, got %d", video.ID)
	}
	if stats.VideoID != video.ID {
		t.Fatalf("expected stats.VideoID %d, got %d", video.ID, stats.VideoID)
	}

	// 2. Query by idempotency key must return the existing video
	found, foundStats, err := repo.FindByAuthorAndIdempotencyKey(ctx, authorID, idempotencyKey)
	if err != nil {
		t.Fatalf("FindByAuthorAndIdempotencyKey failed: %v", err)
	}
	if found.ID != video.ID {
		t.Fatalf("expected found ID %d, got %d", video.ID, found.ID)
	}
	if foundStats.VideoID != video.ID {
		t.Fatalf("expected stats VideoID %d, got %d", video.ID, foundStats.VideoID)
	}

	// 3. Second creation with identical (authorID, idempotencyKey) must conflict
	videoDuplicate := &domainvideo.Video{
		AuthorID:       authorID,
		Title:          "Duplicate Title",
		PlayURL:        "http://example.com/media/videos/dup.mp4",
		Status:         domainvideo.VideoStatusPublished,
		IdempotencyKey: &idempotencyKey,
	}
	err = repo.CreateWithStats(ctx, videoDuplicate, &domainvideo.VideoStats{})
	if err != domainvideo.ErrDuplicateIdempotencyKey {
		t.Fatalf("expected ErrDuplicateIdempotencyKey, got %v", err)
	}
}

func TestGormRepository_FindByIDAndDetail(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	authorID := time.Now().UnixNano()
	video := &domainvideo.Video{
		AuthorID:   authorID,
		Title:      "Detail Query Video",
		PlayURL:    "http://example.com/media/videos/detail.mp4",
		Status:     domainvideo.VideoStatusPublished,
		DurationMs: 60000,
	}
	stats := &domainvideo.VideoStats{
		LikeCount:    10,
		CommentCount: 2,
		ViewCount:    100,
	}

	if err := repo.CreateWithStats(ctx, video, stats); err != nil {
		t.Fatalf("CreateWithStats failed: %v", err)
	}

	// FindByID
	found, err := repo.FindByID(ctx, video.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found.Title != video.Title {
		t.Fatalf("expected title %q, got %q", video.Title, found.Title)
	}

	// FindDetailByID
	foundV, foundS, err := repo.FindDetailByID(ctx, video.ID)
	if err != nil {
		t.Fatalf("FindDetailByID failed: %v", err)
	}
	if foundV.ID != video.ID || foundS.LikeCount != 10 {
		t.Fatalf("unexpected detail: videoID=%d, likes=%d", foundV.ID, foundS.LikeCount)
	}

	// Non-existent ID
	_, err = repo.FindByID(ctx, 999999999999)
	if err != domainvideo.ErrVideoNotFound {
		t.Fatalf("expected ErrVideoNotFound, got %v", err)
	}
}

func TestGormRepository_ListByAuthorCursor(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	authorID := time.Now().UnixNano()

	// Create 5 videos in sequence
	var videoIDs []int64
	for i := 1; i <= 5; i++ {
		v := &domainvideo.Video{
			AuthorID: authorID,
			Title:    fmt.Sprintf("Series Video %d", i),
			PlayURL:  fmt.Sprintf("http://example.com/media/videos/s%d.mp4", i),
			Status:   domainvideo.VideoStatusPublished,
		}
		if err := repo.CreateWithStats(ctx, v, &domainvideo.VideoStats{LikeCount: int64(i * 5)}); err != nil {
			t.Fatalf("CreateWithStats %d failed: %v", i, err)
		}
		videoIDs = append(videoIDs, v.ID)
	}

	// Page 1: limit 2, cursor 0
	videos1, stats1, nextCursor1, hasMore1, err := repo.ListByAuthorCursor(ctx, authorID, 0, 2)
	if err != nil {
		t.Fatalf("Page 1 failed: %v", err)
	}
	if len(videos1) != 2 || len(stats1) != 2 {
		t.Fatalf("expected 2 videos and stats in page 1, got %d", len(videos1))
	}
	if !hasMore1 {
		t.Fatal("expected hasMore to be true for page 1")
	}
	if nextCursor1 != videos1[1].ID {
		t.Fatalf("expected nextCursor %d, got %d", videos1[1].ID, nextCursor1)
	}
	if videos1[0].ID != videoIDs[4] || videos1[1].ID != videoIDs[3] {
		t.Fatalf("expected descending IDs [%d, %d], got [%d, %d]", videoIDs[4], videoIDs[3], videos1[0].ID, videos1[1].ID)
	}

	// Page 2: limit 2, cursor = nextCursor1
	videos2, _, nextCursor2, hasMore2, err := repo.ListByAuthorCursor(ctx, authorID, nextCursor1, 2)
	if err != nil {
		t.Fatalf("Page 2 failed: %v", err)
	}
	if len(videos2) != 2 || !hasMore2 {
		t.Fatalf("expected 2 videos and hasMore=true in page 2, got %d, %v", len(videos2), hasMore2)
	}
	if videos2[0].ID != videoIDs[2] || videos2[1].ID != videoIDs[1] {
		t.Fatalf("expected page 2 IDs [%d, %d], got [%d, %d]", videoIDs[2], videoIDs[1], videos2[0].ID, videos2[1].ID)
	}

	// Page 3: limit 2, cursor = nextCursor2
	videos3, _, _, hasMore3, err := repo.ListByAuthorCursor(ctx, authorID, nextCursor2, 2)
	if err != nil {
		t.Fatalf("Page 3 failed: %v", err)
	}
	if len(videos3) != 1 || hasMore3 {
		t.Fatalf("expected 1 remaining video and hasMore=false in page 3, got %d, %v", len(videos3), hasMore3)
	}
	if videos3[0].ID != videoIDs[0] {
		t.Fatalf("expected last video ID %d, got %d", videoIDs[0], videos3[0].ID)
	}
}

func TestGormRepository_UpdateStatusAndTakeDown(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	authorID := time.Now().UnixNano()
	video := &domainvideo.Video{
		AuthorID: authorID,
		Title:    "Video to be Taken Down",
		PlayURL:  "http://example.com/media/videos/down.mp4",
		Status:   domainvideo.VideoStatusPublished,
	}
	if err := repo.CreateWithStats(ctx, video, &domainvideo.VideoStats{}); err != nil {
		t.Fatalf("CreateWithStats failed: %v", err)
	}

	// Non-author attempt to hide video must be forbidden
	otherAuthorID := authorID + 1
	err := repo.UpdateStatus(ctx, video.ID, otherAuthorID, domainvideo.VideoStatusHidden)
	if err != domainvideo.ErrVideoForbidden {
		t.Fatalf("expected ErrVideoForbidden, got %v", err)
	}

	// Author hides video
	err = repo.UpdateStatus(ctx, video.ID, authorID, domainvideo.VideoStatusHidden)
	if err != nil {
		t.Fatalf("UpdateStatus to hidden failed: %v", err)
	}

	// Repeat hiding (idempotent)
	err = repo.UpdateStatus(ctx, video.ID, authorID, domainvideo.VideoStatusHidden)
	if err != nil {
		t.Fatalf("repeat UpdateStatus failed: %v", err)
	}

	// Hidden video must not be returned by FindByID
	_, err = repo.FindByID(ctx, video.ID)
	if err != domainvideo.ErrVideoNotFound {
		t.Fatalf("expected ErrVideoNotFound for hidden video, got %v", err)
	}

	// Non-existent video update
	err = repo.UpdateStatus(ctx, 999999999999, authorID, domainvideo.VideoStatusHidden)
	if err != domainvideo.ErrVideoNotFound {
		t.Fatalf("expected ErrVideoNotFound for missing video, got %v", err)
	}
}
