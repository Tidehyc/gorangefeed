package feed

import (
	"context"
	"fmt"
	"strconv"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const (
	keyFollowingInbox        = "feed:following:inbox:%d"
	keyFollowingAuthorOutbox = "feed:following:author:%d"
	followingRetentionTTL    = 30 * 24 * time.Hour
)

type followingStore struct {
	rdb *redis.Client
	db  *gorm.DB
}

// NewFollowingStore creates a new FollowingFeedRepository backed by Redis and MySQL.
func NewFollowingStore(rdb *redis.Client, db *gorm.DB) domainfeed.FollowingFeedRepository {
	return &followingStore{
		rdb: rdb,
		db:  db,
	}
}

func (s *followingStore) PushToInboxes(ctx context.Context, followerIDs []int64, videoID int64, maxCapacity int) error {
	if len(followerIDs) == 0 || videoID <= 0 {
		return nil
	}
	if maxCapacity <= 0 {
		maxCapacity = 800
	}

	// Process in batches of 500 to keep pipeline memory safe
	batchSize := 500
	for i := 0; i < len(followerIDs); i += batchSize {
		end := i + batchSize
		if end > len(followerIDs) {
			end = len(followerIDs)
		}
		chunk := followerIDs[i:end]

		pipe := s.rdb.Pipeline()
		for _, fid := range chunk {
			key := fmt.Sprintf(keyFollowingInbox, fid)
			pipe.ZAdd(ctx, key, redis.Z{Score: float64(videoID), Member: videoID})
			pipe.ZRemRangeByRank(ctx, key, 0, -int64(maxCapacity+1))
			pipe.Expire(ctx, key, followingRetentionTTL)
		}

		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("failed to push to follower inboxes pipeline: %w", err)
		}
	}

	return nil
}

func (s *followingStore) PushToAuthorOutbox(ctx context.Context, authorID int64, videoID int64, maxCapacity int) error {
	if authorID <= 0 || videoID <= 0 {
		return nil
	}
	if maxCapacity <= 0 {
		maxCapacity = 800
	}

	key := fmt.Sprintf(keyFollowingAuthorOutbox, authorID)
	pipe := s.rdb.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(videoID), Member: videoID})
	pipe.ZRemRangeByRank(ctx, key, 0, -int64(maxCapacity+1))
	pipe.Expire(ctx, key, followingRetentionTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to push to author outbox: %w", err)
	}

	return nil
}

func (s *followingStore) GetInboxVideoIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, error) {
	if userID <= 0 {
		return nil, nil
	}
	key := fmt.Sprintf(keyFollowingInbox, userID)
	return s.getZSetVideoIDs(ctx, key, cursor, limit)
}

func (s *followingStore) GetAuthorOutboxVideoIDs(ctx context.Context, authorID int64, cursor int64, limit int) ([]int64, error) {
	if authorID <= 0 {
		return nil, nil
	}
	key := fmt.Sprintf(keyFollowingAuthorOutbox, authorID)
	return s.getZSetVideoIDs(ctx, key, cursor, limit)
}

func (s *followingStore) getZSetVideoIDs(ctx context.Context, key string, cursor int64, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 10
	}

	maxScore := "+inf"
	if cursor > 0 {
		maxScore = fmt.Sprintf("(%d", cursor)
	}

	vals, err := s.rdb.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
		Max:    maxScore,
		Min:    "-inf",
		Offset: 0,
		Count:  int64(limit),
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query zset %s: %w", key, err)
	}

	ids := make([]int64, 0, len(vals))
	for _, v := range vals {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}

	return ids, nil
}

func (s *followingStore) CheckInboxExists(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	key := fmt.Sprintf(keyFollowingInbox, userID)
	n, err := s.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check inbox existence for %d: %w", userID, err)
	}
	return n > 0, nil
}

func (s *followingStore) PrewarmInbox(ctx context.Context, userID int64, videoIDs []int64, maxCapacity int) error {
	if userID <= 0 || len(videoIDs) == 0 {
		return nil
	}
	if maxCapacity <= 0 {
		maxCapacity = 800
	}

	key := fmt.Sprintf(keyFollowingInbox, userID)
	pipe := s.rdb.Pipeline()
	for _, vid := range videoIDs {
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(vid), Member: vid})
	}
	pipe.ZRemRangeByRank(ctx, key, 0, -int64(maxCapacity+1))
	pipe.Expire(ctx, key, followingRetentionTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to prewarm inbox for user %d: %w", userID, err)
	}

	return nil
}

func (s *followingStore) ListFollowingVideosFromDB(ctx context.Context, authorIDs []int64, cursor int64, limit int) ([]int64, error) {
	if len(authorIDs) == 0 {
		return []int64{}, nil
	}
	if limit <= 0 {
		limit = 10
	}

	query := s.db.WithContext(ctx).Table("videos").
		Select("id").
		Where("author_id IN ? AND status = ?", authorIDs, 1)

	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}

	var ids []int64
	if err := query.Order("id DESC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("failed to query following videos from db: %w", err)
	}

	return ids, nil
}
