package feed

import (
	"context"
	"fmt"
	"strconv"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	"github.com/redis/go-redis/v9"
)

const (
	hotMinuteKeyPrefix   = "feed:hot:minute:"
	hotSnapshotKeyPrefix = "feed:hot:window:"
)

type hotStore struct {
	rdb redis.Cmdable
}

// NewHotStore creates an implementation of domainfeed.HotTimelineStore backed by Redis.
func NewHotStore(rdb redis.Cmdable) domainfeed.HotTimelineStore {
	return &hotStore{rdb: rdb}
}

// MinuteBucketKey formats the Redis key for a specific 1-minute bucket.
func MinuteBucketKey(t time.Time) string {
	minute := t.UTC().Truncate(time.Minute)
	return fmt.Sprintf("%s%s", hotMinuteKeyPrefix, minute.Format("200601021504"))
}

// WindowSnapshotKey formats the Redis key for a specific aggregated window snapshot.
func WindowSnapshotKey(asOf time.Time) string {
	minute := asOf.UTC().Truncate(time.Minute)
	return fmt.Sprintf("%s%d", hotSnapshotKeyPrefix, minute.Unix())
}

// IncrScore increments or decrements the engagement score of a video in the target minute bucket.
func (s *hotStore) IncrScore(ctx context.Context, videoID int64, delta float64, occurredAt time.Time) error {
	if videoID <= 0 {
		return domainfeed.ErrInvalidVideoID
	}
	if delta == 0 {
		return nil
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	key := MinuteBucketKey(occurredAt)
	member := strconv.FormatInt(videoID, 10)

	pipe := s.rdb.Pipeline()
	pipe.ZIncrBy(ctx, key, delta, member)
	pipe.Expire(ctx, key, domainfeed.HotMinuteBucketTTL)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis zincrby hot score error: %w", err)
	}
	return nil
}

// MergeWindow aggregates sliding minute buckets into a stable snapshot and purges non-positive scores.
func (s *hotStore) MergeWindow(ctx context.Context, asOf time.Time, windowMinutes int) (string, error) {
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if windowMinutes <= 0 {
		windowMinutes = domainfeed.HotWindowBucketCount
	}

	asOfMinute := asOf.UTC().Truncate(time.Minute)
	snapshotKey := WindowSnapshotKey(asOfMinute)

	exists, err := s.rdb.Exists(ctx, snapshotKey).Result()
	if err == nil && exists > 0 {
		return snapshotKey, nil
	}

	bucketKeys := make([]string, 0, windowMinutes)
	for i := 0; i < windowMinutes; i++ {
		t := asOfMinute.Add(-time.Duration(i) * time.Minute)
		bucketKeys = append(bucketKeys, MinuteBucketKey(t))
	}

	pipe := s.rdb.Pipeline()
	pipe.ZUnionStore(ctx, snapshotKey, &redis.ZStore{
		Keys:      bucketKeys,
		Aggregate: "SUM",
	})
	pipe.ZRemRangeByScore(ctx, snapshotKey, "-inf", "0")
	pipe.Expire(ctx, snapshotKey, domainfeed.HotWindowSnapshotTTL)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return "", fmt.Errorf("redis zunionstore hot snapshot error: %w", err)
	}

	return snapshotKey, nil
}

// ListBySnapshot retrieves video IDs from the specified asOf snapshot using offset and limit.
func (s *hotStore) ListBySnapshot(ctx context.Context, asOf time.Time, offset int, limit int) ([]int64, error) {
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if limit <= 0 {
		limit = domainfeed.DefaultLimit
	}
	if offset < 0 {
		offset = 0
	}

	snapshotKey, err := s.MergeWindow(ctx, asOf, domainfeed.HotWindowBucketCount)
	if err != nil {
		return nil, err
	}

	start := int64(offset)
	stop := start + int64(limit) - 1

	members, err := s.rdb.ZRevRange(ctx, snapshotKey, start, stop).Result()
	if err != nil {
		return nil, fmt.Errorf("redis zrevrange hot snapshot error: %w", err)
	}

	videoIDs := make([]int64, 0, len(members))
	for _, m := range members {
		id, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			continue
		}
		videoIDs = append(videoIDs, id)
	}

	return videoIDs, nil
}

// GetSnapshotTotal returns the total count of valid items in the specified asOf snapshot.
func (s *hotStore) GetSnapshotTotal(ctx context.Context, asOf time.Time) (int64, error) {
	snapshotKey, err := s.MergeWindow(ctx, asOf, domainfeed.HotWindowBucketCount)
	if err != nil {
		return 0, err
	}
	count, err := s.rdb.ZCard(ctx, snapshotKey).Result()
	if err != nil {
		return 0, fmt.Errorf("redis zcard hot snapshot error: %w", err)
	}
	return count, nil
}
