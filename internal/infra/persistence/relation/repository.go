package relation

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	persistenceoutbox "github.com/Tidehyc/gorangefeed/internal/infra/persistence/outbox"
	"gorm.io/gorm"
)

type gormRepository struct {
	db *gorm.DB
}

// NewRepository creates a new relation Repository backed by GORM.
func NewRepository(db *gorm.DB) domainrelation.Repository {
	return &gormRepository{db: db}
}

// Follow atomically creates a follow relationship and updates following/follower counts using RowsAffected == 1.
func (r *gormRepository) Follow(ctx context.Context, followerID, vloggerID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	if followerID <= 0 || vloggerID <= 0 {
		return false, domainrelation.ErrInvalidFollowerID
	}
	if followerID == vloggerID {
		return false, domainrelation.ErrSelfFollow
	}

	var followed bool
	now := time.Now()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Insert follow relationship, ignore duplicate
		res := tx.Exec("INSERT INTO follows (follower_id, vlogger_id, created_at) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE id = id",
			followerID, vloggerID, now)
		if res.Error != nil {
			return fmt.Errorf("failed to insert follow: %w", res.Error)
		}

		// 2. Only update stats when actual insertion occurred
		if res.RowsAffected == 1 {
			followed = true

			// Update follower's following_count
			if err := tx.Exec(`
				INSERT INTO relation_stats (user_id, following_count, follower_count, created_at, updated_at)
				VALUES (?, 1, 0, ?, ?)
				ON DUPLICATE KEY UPDATE following_count = following_count + 1, updated_at = ?
			`, followerID, now, now, now).Error; err != nil {
				return fmt.Errorf("failed to increment follower following_count: %w", err)
			}

			// Update vlogger's follower_count
			if err := tx.Exec(`
				INSERT INTO relation_stats (user_id, following_count, follower_count, created_at, updated_at)
				VALUES (?, 0, 1, ?, ?)
				ON DUPLICATE KEY UPDATE follower_count = follower_count + 1, updated_at = ?
			`, vloggerID, now, now, now).Error; err != nil {
				return fmt.Errorf("failed to increment vlogger follower_count: %w", err)
			}

			// Persist outbox event if present
			if outboxEvent != nil {
				if err := persistenceoutbox.SaveEventInTx(ctx, tx, outboxEvent); err != nil {
					return err
				}
			}
		}
		return nil
	})

	return followed, err
}

// Unfollow atomically deletes a follow relationship and decrements counts using RowsAffected == 1.
func (r *gormRepository) Unfollow(ctx context.Context, followerID, vloggerID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	if followerID <= 0 || vloggerID <= 0 {
		return false, domainrelation.ErrInvalidFollowerID
	}
	if followerID == vloggerID {
		return false, domainrelation.ErrSelfUnfollow
	}

	var unfollowed bool
	now := time.Now()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM follows WHERE follower_id = ? AND vlogger_id = ?", followerID, vloggerID)
		if res.Error != nil {
			return fmt.Errorf("failed to delete follow: %w", res.Error)
		}

		if res.RowsAffected == 1 {
			unfollowed = true

			// Decrement follower's following_count with GREATEST
			if err := tx.Exec(`
				UPDATE relation_stats 
				SET following_count = GREATEST(following_count - 1, 0), updated_at = ? 
				WHERE user_id = ?
			`, now, followerID).Error; err != nil {
				return fmt.Errorf("failed to decrement following_count: %w", err)
			}

			// Decrement vlogger's follower_count with GREATEST
			if err := tx.Exec(`
				UPDATE relation_stats 
				SET follower_count = GREATEST(follower_count - 1, 0), updated_at = ? 
				WHERE user_id = ?
			`, now, vloggerID).Error; err != nil {
				return fmt.Errorf("failed to decrement follower_count: %w", err)
			}

			if outboxEvent != nil {
				if err := persistenceoutbox.SaveEventInTx(ctx, tx, outboxEvent); err != nil {
					return err
				}
			}
		}
		return nil
	})

	return unfollowed, err
}

// GetRelation evaluates the bidirectional friendship state between current user and target user.
func (r *gormRepository) GetRelation(ctx context.Context, userID, targetID int64) (*domainrelation.Friendship, error) {
	if userID <= 0 || targetID <= 0 {
		return &domainrelation.Friendship{}, nil
	}
	if userID == targetID {
		return &domainrelation.Friendship{}, nil
	}

	var countFollowing int64
	if err := r.db.WithContext(ctx).Model(&FollowModel{}).
		Where("follower_id = ? AND vlogger_id = ?", userID, targetID).
		Count(&countFollowing).Error; err != nil {
		return nil, fmt.Errorf("failed to check following relation: %w", err)
	}

	var countFollower int64
	if err := r.db.WithContext(ctx).Model(&FollowModel{}).
		Where("follower_id = ? AND vlogger_id = ?", targetID, userID).
		Count(&countFollower).Error; err != nil {
		return nil, fmt.Errorf("failed to check follower relation: %w", err)
	}

	isFollowing := countFollowing > 0
	isFollower := countFollower > 0

	return &domainrelation.Friendship{
		IsFollowing: isFollowing,
		IsFollower:  isFollower,
		IsFriend:    isFollowing && isFollower,
	}, nil
}

// BatchIsFollowed returns a map of authorID -> isFollowed for the given follower.
func (r *gormRepository) BatchIsFollowed(ctx context.Context, followerID int64, authorIDs []int64) (map[int64]bool, error) {
	res := make(map[int64]bool, len(authorIDs))
	if followerID <= 0 || len(authorIDs) == 0 {
		return res, nil
	}

	var follows []FollowModel
	err := r.db.WithContext(ctx).
		Where("follower_id = ? AND vlogger_id IN ?", followerID, authorIDs).
		Find(&follows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to batch get follow status: %w", err)
	}

	for _, f := range follows {
		res[f.VloggerID] = true
	}
	return res, nil
}

// ListFollowingIDs retrieves vlogger IDs that the user follows, ordered by follows.id DESC.
func (r *gormRepository) ListFollowingIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	query := r.db.WithContext(ctx).Model(&FollowModel{}).
		Where("follower_id = ?", userID)

	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}

	var follows []FollowModel
	err := query.Order("id DESC").Limit(limit + 1).Find(&follows).Error
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to list followings: %w", err)
	}

	hasMore := len(follows) > limit
	if hasMore {
		follows = follows[:limit]
	}

	vloggerIDs := make([]int64, len(follows))
	var nextCursor int64
	for i, f := range follows {
		vloggerIDs[i] = f.VloggerID
		nextCursor = f.ID
	}

	return vloggerIDs, nextCursor, hasMore, nil
}

// ListFollowerIDs retrieves follower IDs that follow the vlogger, ordered by follows.id DESC.
func (r *gormRepository) ListFollowerIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	query := r.db.WithContext(ctx).Model(&FollowModel{}).
		Where("vlogger_id = ?", userID)

	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}

	var follows []FollowModel
	err := query.Order("id DESC").Limit(limit + 1).Find(&follows).Error
	if err != nil {
		return nil, 0, false, fmt.Errorf("failed to list followers: %w", err)
	}

	hasMore := len(follows) > limit
	if hasMore {
		follows = follows[:limit]
	}

	followerIDs := make([]int64, len(follows))
	var nextCursor int64
	for i, f := range follows {
		followerIDs[i] = f.FollowerID
		nextCursor = f.ID
	}

	return followerIDs, nextCursor, hasMore, nil
}

// GetStats retrieves relation statistics for a user.
func (r *gormRepository) GetStats(ctx context.Context, userID int64) (*domainrelation.RelationStats, error) {
	var m RelationStatsModel
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &domainrelation.RelationStats{
				UserID:         userID,
				FollowingCount: 0,
				FollowerCount:  0,
			}, nil
		}
		return nil, fmt.Errorf("failed to get relation stats: %w", err)
	}
	return toDomainStats(&m), nil
}
