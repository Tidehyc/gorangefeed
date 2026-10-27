package feed

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	"github.com/redis/go-redis/v9"
)

const (
	defaultTimelineGlobalKey = "timeline:global"
	defaultTimelineFloorKey  = "timeline:floor"
)

// trimAndSetFloorLua atomically trims the ZSet timeline strictly below newFloor
// and updates the floor watermark key.
const trimAndSetFloorLua = `
local removed = redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', '(' .. ARGV[1])
redis.call('SET', KEYS[2], ARGV[1])
return removed
`

type timelineStore struct {
	rdb        redis.Cmdable
	globalKey  string
	floorKey   string
	trimScript *redis.Script
}

// NewTimelineStore creates a TimelineRepository connected to Redis.
func NewTimelineStore(rdb redis.Cmdable) domainfeed.TimelineRepository {
	return NewCustomTimelineStore(rdb, defaultTimelineGlobalKey, defaultTimelineFloorKey)
}

// NewCustomTimelineStore creates a TimelineRepository with customizable keys (useful for test isolation).
func NewCustomTimelineStore(rdb redis.Cmdable, globalKey, floorKey string) domainfeed.TimelineRepository {
	return &timelineStore{
		rdb:        rdb,
		globalKey:  globalKey,
		floorKey:   floorKey,
		trimScript: redis.NewScript(trimAndSetFloorLua),
	}
}

func (s *timelineStore) GetFloor(ctx context.Context) (int64, error) {
	val, err := s.rdb.Get(ctx, s.floorKey).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("redis get floor error: %w", err)
	}
	floor, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid floor value %q: %w", val, err)
	}
	return floor, nil
}

func (s *timelineStore) SetFloor(ctx context.Context, floor int64) error {
	if err := s.rdb.Set(ctx, s.floorKey, strconv.FormatInt(floor, 10), 0).Err(); err != nil {
		return fmt.Errorf("redis set floor error: %w", err)
	}
	return nil
}

func (s *timelineStore) Append(ctx context.Context, videoID int64) error {
	if videoID <= 0 {
		return domainfeed.ErrInvalidVideoID
	}
	member := strconv.FormatInt(videoID, 10)
	err := s.rdb.ZAdd(ctx, s.globalKey, redis.Z{
		Score:  float64(videoID),
		Member: member,
	}).Err()
	if err != nil {
		return fmt.Errorf("redis zadd timeline error: %w", err)
	}
	return nil
}

func (s *timelineStore) Trim(ctx context.Context, floor int64) error {
	if floor <= 0 {
		return nil
	}
	keys := []string{s.globalKey, s.floorKey}
	args := []interface{}{strconv.FormatInt(floor, 10)}
	if err := s.trimScript.Run(ctx, s.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("redis trim lua error: %w", err)
	}
	return nil
}

func (s *timelineStore) ListByCursor(ctx context.Context, cursor int64, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = domainfeed.DefaultLimit
	}

	maxScore := "+inf"
	if cursor > 0 {
		maxScore = fmt.Sprintf("(%d", cursor)
	}

	opt := &redis.ZRangeBy{
		Min:    "-inf",
		Max:    maxScore,
		Offset: 0,
		Count:  int64(limit),
	}

	members, err := s.rdb.ZRevRangeByScore(ctx, s.globalKey, opt).Result()
	if err != nil {
		return nil, fmt.Errorf("redis zrevrangebyscore error: %w", err)
	}

	ids := make([]int64, 0, len(members))
	for _, m := range members {
		id, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *timelineStore) Rebuild(ctx context.Context, videoIDs []int64, floor int64) error {
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, s.globalKey)

	if len(videoIDs) > 0 {
		zMembers := make([]redis.Z, len(videoIDs))
		for i, id := range videoIDs {
			zMembers[i] = redis.Z{
				Score:  float64(id),
				Member: strconv.FormatInt(id, 10),
			}
		}
		pipe.ZAdd(ctx, s.globalKey, zMembers...)
	}

	if floor > 0 {
		pipe.Set(ctx, s.floorKey, strconv.FormatInt(floor, 10), 0)
	} else {
		pipe.Del(ctx, s.floorKey)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis rebuild timeline error: %w", err)
	}
	return nil
}
