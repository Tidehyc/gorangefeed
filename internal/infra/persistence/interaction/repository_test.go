package interaction_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceinteraction "github.com/Tidehyc/gorangefeed/internal/infra/persistence/interaction"
	persistencevideo "github.com/Tidehyc/gorangefeed/internal/infra/persistence/video"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, domaininteraction.Repository, domainvideo.Repository) {
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
		return nil, nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping persistence test: MySQL connection failed (%v)", err)
		return nil, nil, nil
	}

	repo := persistenceinteraction.NewRepository(db)
	videoRepo := persistencevideo.NewRepository(db)
	return db, repo, videoRepo
}

func createTestVideo(t *testing.T, videoRepo domainvideo.Repository) int64 {
	t.Helper()
	ctx := context.Background()
	authorID := time.Now().UnixNano()
	key := uuid.New().String()
	v := &domainvideo.Video{
		AuthorID:       authorID,
		Title:          "Interaction Test Video",
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

func TestGormInteractionRepository_LikeLifecycle(t *testing.T) {
	db, repo, videoRepo := setupTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()
	videoID := createTestVideo(t, videoRepo)
	userID := time.Now().UnixNano()

	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeInteractionLiked, map[string]any{
		"user_id":  userID,
		"video_id": videoID,
	})
	if err != nil {
		t.Fatalf("failed to create outbox event: %v", err)
	}

	// 1. First like must succeed and increment count
	liked, err := repo.LikeVideo(ctx, userID, videoID, outboxEvent)
	if err != nil {
		t.Fatalf("LikeVideo failed: %v", err)
	}
	if !liked {
		t.Errorf("expected liked true on first like")
	}

	// Verify is_liked and video_stats
	isLiked, err := repo.IsLiked(ctx, userID, videoID)
	if err != nil || !isLiked {
		t.Fatalf("expected isLiked true, got %v (err: %v)", isLiked, err)
	}

	_, stats, err := videoRepo.FindDetailByID(ctx, videoID)
	if err != nil || stats.LikeCount != 1 {
		t.Fatalf("expected like_count 1, got %d (err: %v)", stats.LikeCount, err)
	}

	// 2. Duplicate like must be idempotent and return false
	likedAgain, err := repo.LikeVideo(ctx, userID, videoID, nil)
	if err != nil {
		t.Fatalf("duplicate LikeVideo failed: %v", err)
	}
	if likedAgain {
		t.Errorf("expected duplicate like to return false")
	}

	_, stats, err = videoRepo.FindDetailByID(ctx, videoID)
	if err != nil || stats.LikeCount != 1 {
		t.Fatalf("expected like_count to remain 1, got %d", stats.LikeCount)
	}

	// 3. BatchIsLiked check
	statusMap, err := repo.BatchIsLiked(ctx, userID, []int64{videoID, 9999999})
	if err != nil {
		t.Fatalf("BatchIsLiked failed: %v", err)
	}
	if !statusMap[videoID] || statusMap[9999999] {
		t.Errorf("expected videoID true and 9999999 false, got %v", statusMap)
	}

	// 4. ListMyLikedVideoIDs
	likedIDs, nextCursor, hasMore, err := repo.ListMyLikedVideoIDs(ctx, userID, 0, 10)
	if err != nil {
		t.Fatalf("ListMyLikedVideoIDs failed: %v", err)
	}
	if len(likedIDs) != 1 || likedIDs[0] != videoID {
		t.Errorf("expected likedIDs [%d], got %v", videoID, likedIDs)
	}
	if nextCursor <= 0 || hasMore {
		t.Errorf("expected nextCursor > 0 and hasMore false, got cursor=%d hasMore=%v", nextCursor, hasMore)
	}

	// 5. Unlike video
	unliked, err := repo.UnlikeVideo(ctx, userID, videoID, nil)
	if err != nil {
		t.Fatalf("UnlikeVideo failed: %v", err)
	}
	if !unliked {
		t.Errorf("expected unliked true")
	}

	_, stats, err = videoRepo.FindDetailByID(ctx, videoID)
	if err != nil || stats.LikeCount != 0 {
		t.Fatalf("expected like_count 0 after unlike, got %d", stats.LikeCount)
	}

	// 6. Duplicate unlike must be idempotent and not drop below 0
	unlikedAgain, err := repo.UnlikeVideo(ctx, userID, videoID, nil)
	if err != nil {
		t.Fatalf("duplicate UnlikeVideo failed: %v", err)
	}
	if unlikedAgain {
		t.Errorf("expected duplicate unlike to return false")
	}

	_, stats, err = videoRepo.FindDetailByID(ctx, videoID)
	if err != nil || stats.LikeCount != 0 {
		t.Fatalf("expected like_count 0, got %d", stats.LikeCount)
	}
}

func TestGormInteractionRepository_CommentLifecycle(t *testing.T) {
	db, repo, videoRepo := setupTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()
	videoID := createTestVideo(t, videoRepo)
	userID := time.Now().UnixNano()
	idempotencyKey := fmt.Sprintf("comment-idem-%d", userID)

	c := &domaininteraction.Comment{
		VideoID:        videoID,
		UserID:         userID,
		ParentID:       0,
		Content:        "First comment!",
		IdempotencyKey: idempotencyKey,
		Status:         domaininteraction.CommentStatusPublished,
	}

	// 1. Create comment with idempotency
	createdComment, created, err := repo.CreateComment(ctx, c, nil)
	if err != nil {
		t.Fatalf("CreateComment failed: %v", err)
	}
	if !created || createdComment.ID <= 0 {
		t.Fatalf("expected created true with positive ID, got created=%v ID=%d", created, createdComment.ID)
	}

	_, stats, err := videoRepo.FindDetailByID(ctx, videoID)
	if err != nil || stats.CommentCount != 1 {
		t.Fatalf("expected comment_count 1, got %d", stats.CommentCount)
	}

	// 2. Retry with same idempotency key must return existing comment without incrementing count
	retryComment, created2, err := repo.CreateComment(ctx, c, nil)
	if err != nil {
		t.Fatalf("retry CreateComment failed: %v", err)
	}
	if created2 {
		t.Errorf("expected created false for retry")
	}
	if retryComment.ID != createdComment.ID {
		t.Errorf("expected same comment ID %d, got %d", createdComment.ID, retryComment.ID)
	}

	_, stats, err = videoRepo.FindDetailByID(ctx, videoID)
	if err != nil || stats.CommentCount != 1 {
		t.Fatalf("expected comment_count to remain 1, got %d", stats.CommentCount)
	}

	// 3. Comment like and unlike
	liked, err := repo.LikeComment(ctx, userID, createdComment.ID)
	if err != nil || !liked {
		t.Fatalf("LikeComment failed: %v (liked: %v)", err, liked)
	}

	cAfterLike, err := repo.FindCommentByID(ctx, createdComment.ID)
	if err != nil || cAfterLike.LikeCount != 1 {
		t.Fatalf("expected comment like_count 1, got %d (err: %v)", cAfterLike.LikeCount, err)
	}

	batchCommentLikes, err := repo.BatchIsCommentLiked(ctx, userID, []int64{createdComment.ID})
	if err != nil || !batchCommentLikes[createdComment.ID] {
		t.Fatalf("BatchIsCommentLiked failed or false: %v", err)
	}

	// 4. List comments by time
	comments, nextCursor, hasMore, err := repo.ListCommentsByTime(ctx, videoID, 0, 10)
	if err != nil {
		t.Fatalf("ListCommentsByTime failed: %v", err)
	}
	if len(comments) != 1 || comments[0].ID != createdComment.ID {
		t.Errorf("expected 1 comment with ID %d, got %v", createdComment.ID, comments)
	}
	if nextCursor <= 0 || hasMore {
		t.Errorf("unexpected cursor=%d hasMore=%v", nextCursor, hasMore)
	}

	// 5. Delete comment
	if err := repo.DeleteComment(ctx, createdComment.ID, userID, nil); err != nil {
		t.Fatalf("DeleteComment failed: %v", err)
	}

	_, stats, err = videoRepo.FindDetailByID(ctx, videoID)
	if err != nil || stats.CommentCount != 0 {
		t.Fatalf("expected comment_count 0 after delete, got %d", stats.CommentCount)
	}

	// Deleted comment should not appear in ListCommentsByTime
	commentsAfterDelete, _, _, err := repo.ListCommentsByTime(ctx, videoID, 0, 10)
	if err != nil {
		t.Fatalf("ListCommentsByTime after delete failed: %v", err)
	}
	if len(commentsAfterDelete) != 0 {
		t.Errorf("expected 0 published comments, got %d", len(commentsAfterDelete))
	}
}
