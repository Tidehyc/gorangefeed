package interaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	persistenceoutbox "github.com/Tidehyc/gorangefeed/internal/infra/persistence/outbox"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type gormRepository struct {
	db *gorm.DB
}

// NewRepository creates a new interaction Repository backed by GORM.
func NewRepository(db *gorm.DB) domaininteraction.Repository {
	return &gormRepository{db: db}
}

// LikeVideo atomically inserts a like record and increments video_stats.like_count using RowsAffected == 1.
func (r *gormRepository) LikeVideo(ctx context.Context, userID, videoID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	var liked bool
	now := time.Now()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Attempt insert, on duplicate do nothing (no-op update)
		res := tx.Exec("INSERT INTO likes (user_id, video_id, created_at) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE id = id",
			userID, videoID, now)
		if res.Error != nil {
			return fmt.Errorf("failed to insert like: %w", res.Error)
		}

		// 2. Only increment count when actual insertion occurred
		if res.RowsAffected == 1 {
			liked = true
			if err := tx.Exec("UPDATE video_stats SET like_count = like_count + 1 WHERE video_id = ?", videoID).Error; err != nil {
				return fmt.Errorf("failed to increment video like_count: %w", err)
			}
			if outboxEvent != nil {
				if err := persistenceoutbox.SaveEventInTx(ctx, tx, outboxEvent); err != nil {
					return err
				}
			}
		}
		return nil
	})

	return liked, err
}

// UnlikeVideo atomically deletes the like record and decrements video_stats.like_count using RowsAffected == 1.
func (r *gormRepository) UnlikeVideo(ctx context.Context, userID, videoID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	var unliked bool

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM likes WHERE user_id = ? AND video_id = ?", userID, videoID)
		if res.Error != nil {
			return fmt.Errorf("failed to delete like: %w", res.Error)
		}

		// Only decrement count when actual row was deleted
		if res.RowsAffected == 1 {
			unliked = true
			// GREATEST prevents count from dropping below 0
			if err := tx.Exec("UPDATE video_stats SET like_count = GREATEST(like_count - 1, 0) WHERE video_id = ?", videoID).Error; err != nil {
				return fmt.Errorf("failed to decrement video like_count: %w", err)
			}
			if outboxEvent != nil {
				if err := persistenceoutbox.SaveEventInTx(ctx, tx, outboxEvent); err != nil {
					return err
				}
			}
		}
		return nil
	})

	return unliked, err
}

// IsLiked checks whether a user has liked a video.
func (r *gormRepository) IsLiked(ctx context.Context, userID, videoID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&LikeModel{}).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check like status: %w", err)
	}
	return count > 0, nil
}

// BatchIsLiked returns a map of videoID -> isLiked for the given user.
func (r *gormRepository) BatchIsLiked(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error) {
	res := make(map[int64]bool, len(videoIDs))
	if userID <= 0 || len(videoIDs) == 0 {
		return res, nil
	}

	var likes []LikeModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id IN ?", userID, videoIDs).
		Find(&likes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to batch get like status: %w", err)
	}

	for _, l := range likes {
		res[l.VideoID] = true
	}
	return res, nil
}

// ListMyLikedVideoIDs returns video IDs liked by the user ordered by like time DESC (likes.id < cursor).
func (r *gormRepository) ListMyLikedVideoIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	query := r.db.WithContext(ctx).Model(&LikeModel{}).
		Where("user_id = ?", userID)

	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}

	var likes []LikeModel
	// Fetch limit + 1 to detect hasMore
	err := query.Order("id DESC").Limit(limit + 1).Find(&likes).Error
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to list my liked video ids: %w", err)
	}

	hasMore := len(likes) > limit
	if hasMore {
		likes = likes[:limit]
	}

	videoIDs := make([]int64, len(likes))
	var nextCursor int64
	for i, l := range likes {
		videoIDs[i] = l.VideoID
		nextCursor = l.ID
	}

	return videoIDs, nextCursor, hasMore, nil
}

// CreateComment inserts a comment with idempotency protection.
func (r *gormRepository) CreateComment(ctx context.Context, comment *domaininteraction.Comment, outboxEvent *domainoutbox.Event) (*domaininteraction.Comment, bool, error) {
	if comment == nil {
		return nil, false, errors.New("comment cannot be nil")
	}

	var resultComment *domaininteraction.Comment
	var created bool

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m := fromDomainComment(comment)
		err := tx.Create(m).Error
		if err != nil {
			var mysqlErr *mysql.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
				// Duplicate idempotency key detected: return existing comment
				var existing CommentModel
				if findErr := tx.Where("user_id = ? AND idempotency_key = ?", comment.UserID, comment.IdempotencyKey).
					First(&existing).Error; findErr != nil {
					return fmt.Errorf("failed to find existing idempotent comment: %w", findErr)
				}
				resultComment = toDomainComment(&existing)
				created = false
				return nil
			}
			return fmt.Errorf("failed to insert comment: %w", err)
		}

		// Newly created comment
		created = true
		resultComment = toDomainComment(m)
		comment.ID = m.ID
		comment.CreatedAt = m.CreatedAt
		comment.UpdatedAt = m.UpdatedAt

		// Increment video_stats.comment_count
		if err := tx.Exec("UPDATE video_stats SET comment_count = comment_count + 1 WHERE video_id = ?", comment.VideoID).Error; err != nil {
			return fmt.Errorf("failed to increment video comment_count: %w", err)
		}

		// Persist outbox event
		if outboxEvent != nil {
			if err := persistenceoutbox.SaveEventInTx(ctx, tx, outboxEvent); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, false, err
	}
	return resultComment, created, nil
}

// FindCommentByIdempotencyKey retrieves an existing comment by user ID and idempotency key.
func (r *gormRepository) FindCommentByIdempotencyKey(ctx context.Context, userID int64, key string) (*domaininteraction.Comment, error) {
	var m CommentModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND idempotency_key = ?", userID, key).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domaininteraction.ErrCommentNotFound
		}
		return nil, fmt.Errorf("failed to find comment by idempotency key: %w", err)
	}
	return toDomainComment(&m), nil
}

// FindCommentByID retrieves a comment by its ID.
func (r *gormRepository) FindCommentByID(ctx context.Context, commentID int64) (*domaininteraction.Comment, error) {
	var m CommentModel
	err := r.db.WithContext(ctx).Where("id = ?", commentID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domaininteraction.ErrCommentNotFound
		}
		return nil, fmt.Errorf("failed to find comment by id: %w", err)
	}
	return toDomainComment(&m), nil
}

// DeleteComment soft-deletes a comment (status = 2) and decrements video_stats.comment_count atomically.
func (r *gormRepository) DeleteComment(ctx context.Context, commentID, userID int64, outboxEvent *domainoutbox.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var comment CommentModel
		if err := tx.Where("id = ?", commentID).First(&comment).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domaininteraction.ErrCommentNotFound
			}
			return err
		}

		if comment.UserID != userID {
			return domaininteraction.ErrCommentForbidden
		}

		if comment.Status == int8(domaininteraction.CommentStatusDeleted) {
			return nil // Already deleted, idempotent
		}

		res := tx.Model(&CommentModel{}).
			Where("id = ? AND user_id = ? AND status = ?", commentID, userID, int8(domaininteraction.CommentStatusPublished)).
			Update("status", int8(domaininteraction.CommentStatusDeleted))
		if res.Error != nil {
			return fmt.Errorf("failed to soft-delete comment: %w", res.Error)
		}

		if res.RowsAffected == 1 {
			if err := tx.Exec("UPDATE video_stats SET comment_count = GREATEST(comment_count - 1, 0) WHERE video_id = ?", comment.VideoID).Error; err != nil {
				return fmt.Errorf("failed to decrement video comment_count: %w", err)
			}
			if outboxEvent != nil {
				if err := persistenceoutbox.SaveEventInTx(ctx, tx, outboxEvent); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// ListCommentsByTime retrieves published comments for a video ordered by id DESC using cursor pagination.
func (r *gormRepository) ListCommentsByTime(ctx context.Context, videoID int64, cursor int64, limit int) ([]*domaininteraction.Comment, int64, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	query := r.db.WithContext(ctx).Model(&CommentModel{}).
		Where("video_id = ? AND status = ?", videoID, int8(domaininteraction.CommentStatusPublished))

	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}

	var models []CommentModel
	err := query.Order("id DESC").Limit(limit + 1).Find(&models).Error
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to list comments by time: %w", err)
	}

	hasMore := len(models) > limit
	if hasMore {
		models = models[:limit]
	}

	comments := make([]*domaininteraction.Comment, len(models))
	var nextCursor int64
	for i := range models {
		comments[i] = toDomainComment(&models[i])
		nextCursor = models[i].ID
	}

	return comments, nextCursor, hasMore, nil
}

// ListCommentsByHot retrieves published comments for a video ordered by like_count DESC, id DESC.
func (r *gormRepository) ListCommentsByHot(ctx context.Context, videoID int64, cursorLikeCount int64, cursorID int64, limit int) ([]*domaininteraction.Comment, int64, int64, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	query := r.db.WithContext(ctx).Model(&CommentModel{}).
		Where("video_id = ? AND status = ?", videoID, int8(domaininteraction.CommentStatusPublished))

	if cursorLikeCount > 0 || cursorID > 0 {
		query = query.Where("(like_count < ?) OR (like_count = ? AND id < ?)", cursorLikeCount, cursorLikeCount, cursorID)
	}

	var models []CommentModel
	err := query.Order("like_count DESC, id DESC").Limit(limit + 1).Find(&models).Error
	if err != nil {
		return nil, 0, 0, false, fmt.Errorf("failed to list comments by hot: %w", err)
	}

	hasMore := len(models) > limit
	if hasMore {
		models = models[:limit]
	}

	comments := make([]*domaininteraction.Comment, len(models))
	var nextLikeCount, nextID int64
	for i := range models {
		comments[i] = toDomainComment(&models[i])
		nextLikeCount = models[i].LikeCount
		nextID = models[i].ID
	}

	return comments, nextLikeCount, nextID, hasMore, nil
}

// LikeComment atomically increments comments.like_count using RowsAffected == 1.
func (r *gormRepository) LikeComment(ctx context.Context, userID, commentID int64) (bool, error) {
	var liked bool
	now := time.Now()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec("INSERT INTO comment_likes (user_id, comment_id, created_at) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE id = id",
			userID, commentID, now)
		if res.Error != nil {
			return fmt.Errorf("failed to insert comment like: %w", res.Error)
		}

		if res.RowsAffected == 1 {
			liked = true
			if err := tx.Exec("UPDATE comments SET like_count = like_count + 1 WHERE id = ?", commentID).Error; err != nil {
				return fmt.Errorf("failed to increment comment like_count: %w", err)
			}
		}
		return nil
	})

	return liked, err
}

// UnlikeComment atomically decrements comments.like_count using RowsAffected == 1.
func (r *gormRepository) UnlikeComment(ctx context.Context, userID, commentID int64) (bool, error) {
	var unliked bool

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM comment_likes WHERE user_id = ? AND comment_id = ?", userID, commentID)
		if res.Error != nil {
			return fmt.Errorf("failed to delete comment like: %w", res.Error)
		}

		if res.RowsAffected == 1 {
			unliked = true
			if err := tx.Exec("UPDATE comments SET like_count = GREATEST(like_count - 1, 0) WHERE id = ?", commentID).Error; err != nil {
				return fmt.Errorf("failed to decrement comment like_count: %w", err)
			}
		}
		return nil
	})

	return unliked, err
}

// BatchIsCommentLiked returns a map of commentID -> isLiked for the given user.
func (r *gormRepository) BatchIsCommentLiked(ctx context.Context, userID int64, commentIDs []int64) (map[int64]bool, error) {
	res := make(map[int64]bool, len(commentIDs))
	if userID <= 0 || len(commentIDs) == 0 {
		return res, nil
	}

	var likes []CommentLikeModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND comment_id IN ?", userID, commentIDs).
		Find(&likes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to batch get comment like status: %w", err)
	}

	for _, l := range likes {
		res[l.CommentID] = true
	}
	return res, nil
}
