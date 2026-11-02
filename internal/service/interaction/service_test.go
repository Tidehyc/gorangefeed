package interaction_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceaccount "github.com/Tidehyc/gorangefeed/internal/infra/persistence/account"
	persistenceinteraction "github.com/Tidehyc/gorangefeed/internal/infra/persistence/interaction"
	persistencevideo "github.com/Tidehyc/gorangefeed/internal/infra/persistence/video"
	serviceinteraction "github.com/Tidehyc/gorangefeed/internal/service/interaction"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupServiceTest(t *testing.T) (*serviceinteraction.Service, domainvideo.Repository, *gorm.DB) {
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

	interactionRepo := persistenceinteraction.NewRepository(db)
	videoRepo := persistencevideo.NewRepository(db)
	accountRepo := persistenceaccount.NewRepository(db)

	svc := serviceinteraction.NewService(interactionRepo, videoRepo, accountRepo)
	return svc, videoRepo, db
}

func createTestVideoForService(t *testing.T, videoRepo domainvideo.Repository) int64 {
	t.Helper()
	ctx := context.Background()
	authorID := time.Now().UnixNano()
	key := uuid.New().String()
	v := &domainvideo.Video{
		AuthorID:       authorID,
		Title:          "Service Test Video",
		PlayURL:        "http://example.com/test.mp4",
		Status:         domainvideo.VideoStatusPublished,
		IdempotencyKey: &key,
	}
	stats := &domainvideo.VideoStats{}
	if err := videoRepo.CreateWithStats(ctx, v, stats); err != nil {
		t.Fatalf("failed to create test video: %v", err)
	}
	return v.ID
}

func TestService_LikeAndCommentFlow(t *testing.T) {
	svc, videoRepo, db := setupServiceTest(t)
	if db == nil {
		return
	}
	ctx := context.Background()
	videoID := createTestVideoForService(t, videoRepo)
	userID := time.Now().UnixNano()

	// 1. Like video
	if err := svc.LikeVideo(ctx, userID, videoID); err != nil {
		t.Fatalf("LikeVideo failed: %v", err)
	}

	// 2. List my liked videos
	likedVideos, _, _, err := svc.ListMyLikedVideos(ctx, userID, 0, 10)
	if err != nil {
		t.Fatalf("ListMyLikedVideos failed: %v", err)
	}
	if len(likedVideos) != 1 || likedVideos[0].Video.ID != videoID {
		t.Fatalf("expected videoID %d in my liked list, got %v", videoID, likedVideos)
	}

	// 3. Create comment
	idemKey := fmt.Sprintf("service-test-%d", userID)
	detail, created, err := svc.CreateComment(ctx, userID, videoID, 0, "Great video!", idemKey)
	if err != nil || !created {
		t.Fatalf("CreateComment failed: %v (created: %v)", err, created)
	}
	if detail.Comment.Content != "Great video!" {
		t.Errorf("expected comment content, got %s", detail.Comment.Content)
	}

	// 4. Like comment
	if err := svc.LikeComment(ctx, userID, detail.Comment.ID); err != nil {
		t.Fatalf("LikeComment failed: %v", err)
	}

	// 5. List comments
	comments, _, _, _, err := svc.ListComments(ctx, userID, videoID, "latest", 0, 0, 0, 10)
	if err != nil {
		t.Fatalf("ListComments failed: %v", err)
	}
	if len(comments) != 1 || comments[0].Comment.ID != detail.Comment.ID {
		t.Fatalf("expected 1 comment, got %v", comments)
	}
	if !comments[0].IsLiked {
		t.Errorf("expected comment IsLiked to be true")
	}

	// 6. Delete comment
	if err := svc.DeleteComment(ctx, userID, detail.Comment.ID); err != nil {
		t.Fatalf("DeleteComment failed: %v", err)
	}

	// 7. Unlike video
	if err := svc.UnlikeVideo(ctx, userID, videoID); err != nil {
		t.Fatalf("UnlikeVideo failed: %v", err)
	}
}

func TestService_InvalidInputs(t *testing.T) {
	svc, videoRepo, db := setupServiceTest(t)
	if db == nil {
		return
	}
	ctx := context.Background()
	videoID := createTestVideoForService(t, videoRepo)

	// Like non-existent video
	err := svc.LikeVideo(ctx, 1, 99999999)
	if !errors.Is(err, domaininteraction.ErrVideoNotFound) {
		t.Errorf("expected ErrVideoNotFound, got %v", err)
	}

	// Comment on non-existent video
	_, _, err = svc.CreateComment(ctx, 1, 99999999, 0, "Valid content", "key-nonexist")
	if !errors.Is(err, domaininteraction.ErrVideoNotFound) {
		t.Errorf("expected ErrVideoNotFound for non-existent video, got %v", err)
	}

	// Create comment with empty content
	_, _, err = svc.CreateComment(ctx, 1, videoID, 0, "   ", "key")
	if !errors.Is(err, domaininteraction.ErrEmptyCommentContent) {
		t.Errorf("expected ErrEmptyCommentContent, got %v", err)
	}
}
