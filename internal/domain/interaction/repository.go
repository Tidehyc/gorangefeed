package interaction

import (
	"context"

	"github.com/Tidehyc/gorangefeed/internal/domain/outbox"
)

// Repository defines data persistence contracts for interaction domain.
type Repository interface {
	// LikeVideo atomically inserts a like record and increments video_stats.like_count using RowsAffected == 1.
	// Optionally writes an outboxEvent within the same transaction. Returns true if newly liked, false if already liked.
	LikeVideo(ctx context.Context, userID, videoID int64, outboxEvent *outbox.Event) (bool, error)

	// UnlikeVideo atomically deletes the like record and decrements video_stats.like_count using RowsAffected == 1.
	// GREATEST(like_count - 1, 0) prevents negative count. Optionally writes an outboxEvent.
	UnlikeVideo(ctx context.Context, userID, videoID int64, outboxEvent *outbox.Event) (bool, error)

	// IsLiked checks whether a user has liked a video.
	IsLiked(ctx context.Context, userID, videoID int64) (bool, error)

	// BatchIsLiked returns a map of videoID -> isLiked for the given user.
	BatchIsLiked(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error)

	// ListMyLikedVideoIDs returns video IDs liked by the user ordered by like time DESC (likes.id < cursor).
	// Returns: videoIDs, nextCursor, hasMore, error.
	ListMyLikedVideoIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error)

	// CreateComment inserts a comment with idempotency protection. If duplicate idempotency key is found,
	// returns the existing comment and created=false. On first creation, increments video_stats.comment_count
	// and writes outboxEvent in the same transaction, returning created=true.
	CreateComment(ctx context.Context, comment *Comment, outboxEvent *outbox.Event) (*Comment, bool, error)

	// FindCommentByIdempotencyKey retrieves an existing comment by user ID and idempotency key.
	FindCommentByIdempotencyKey(ctx context.Context, userID int64, key string) (*Comment, error)

	// FindCommentByID retrieves a comment by its ID.
	FindCommentByID(ctx context.Context, commentID int64) (*Comment, error)

	// DeleteComment soft-deletes a comment (status = 2) and decrements video_stats.comment_count atomically.
	DeleteComment(ctx context.Context, commentID, userID int64, outboxEvent *outbox.Event) error

	// ListCommentsByTime retrieves published comments for a video ordered by id DESC using cursor pagination.
	// Returns: comments, nextCursor, hasMore, error.
	ListCommentsByTime(ctx context.Context, videoID int64, cursor int64, limit int) ([]*Comment, int64, bool, error)

	// ListCommentsByHot retrieves published comments for a video ordered by like_count DESC, id DESC.
	// Uses composite cursor (cursorLikeCount, cursorID).
	// Returns: comments, nextLikeCount, nextID, hasMore, error.
	ListCommentsByHot(ctx context.Context, videoID int64, cursorLikeCount int64, cursorID int64, limit int) ([]*Comment, int64, int64, bool, error)

	// LikeComment atomically increments comments.like_count using RowsAffected == 1.
	LikeComment(ctx context.Context, userID, commentID int64) (bool, error)

	// UnlikeComment atomically decrements comments.like_count using RowsAffected == 1.
	UnlikeComment(ctx context.Context, userID, commentID int64) (bool, error)

	// BatchIsCommentLiked returns a map of commentID -> isLiked for the given user.
	BatchIsCommentLiked(ctx context.Context, userID int64, commentIDs []int64) (map[int64]bool, error)
}
