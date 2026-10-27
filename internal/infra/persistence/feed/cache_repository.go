package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	"github.com/redis/go-redis/v9"
)

// pageCacheRepository implements domainfeed.PageCacheRepository for L1 pages.
type pageCacheRepository struct {
	rdb redis.Cmdable
}

// NewPageCacheRepository creates an L1 PageCacheRepository instance.
func NewPageCacheRepository(rdb redis.Cmdable) domainfeed.PageCacheRepository {
	return &pageCacheRepository{rdb: rdb}
}

func (r *pageCacheRepository) GetPage(ctx context.Context, key string) (*domainfeed.CachedPage, bool, error) {
	bytes, err := r.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get l1 page cache error: %w", err)
	}

	var page domainfeed.CachedPage
	if err := json.Unmarshal(bytes, &page); err != nil {
		return nil, false, fmt.Errorf("unmarshal l1 page cache error: %w", err)
	}
	return &page, true, nil
}

func (r *pageCacheRepository) SetPage(ctx context.Context, key string, page *domainfeed.CachedPage, ttl time.Duration) error {
	if page == nil {
		return nil
	}
	bytes, err := json.Marshal(page)
	if err != nil {
		return fmt.Errorf("marshal l1 page cache error: %w", err)
	}
	if err := r.rdb.Set(ctx, key, bytes, ttl).Err(); err != nil {
		return fmt.Errorf("set l1 page cache error: %w", err)
	}
	return nil
}

func (r *pageCacheRepository) DeletePage(ctx context.Context, key string) error {
	if err := r.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("del l1 page cache error: %w", err)
	}
	return nil
}

// cardCacheRepository implements domainfeed.CardCacheRepository for L3 video cards.
type cardCacheRepository struct {
	rdb redis.Cmdable
}

// NewCardCacheRepository creates an L3 CardCacheRepository instance.
func NewCardCacheRepository(rdb redis.Cmdable) domainfeed.CardCacheRepository {
	return &cardCacheRepository{rdb: rdb}
}

func (r *cardCacheRepository) GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	results := make(map[int64]*domainfeed.FeedCard, len(videoIDs))
	if len(videoIDs) == 0 {
		return results, nil
	}

	keys := make([]string, len(videoIDs))
	for i, id := range videoIDs {
		keys[i] = fmt.Sprintf("feed:card:%d", id)
	}

	vals, err := r.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget l3 card cache error: %w", err)
	}

	for i, val := range vals {
		if val == nil {
			continue
		}
		str, ok := val.(string)
		if !ok {
			continue
		}
		var card domainfeed.FeedCard
		if err := json.Unmarshal([]byte(str), &card); err == nil {
			results[videoIDs[i]] = &card
		}
	}
	return results, nil
}

func (r *cardCacheRepository) SetCards(ctx context.Context, cards []*domainfeed.FeedCard, ttl time.Duration) error {
	if len(cards) == 0 {
		return nil
	}
	pipe := r.rdb.Pipeline()
	for _, card := range cards {
		if card == nil {
			continue
		}
		bytes, err := json.Marshal(card)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("feed:card:%d", card.VideoID)
		pipe.Set(ctx, key, bytes, ttl)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("set l3 card cache error: %w", err)
	}
	return nil
}

// statCacheRepository implements domainfeed.StatCacheRepository for L4 metrics.
type statCacheRepository struct {
	rdb redis.Cmdable
}

// NewStatCacheRepository creates an L4 StatCacheRepository instance.
func NewStatCacheRepository(rdb redis.Cmdable) domainfeed.StatCacheRepository {
	return &statCacheRepository{rdb: rdb}
}

func (r *statCacheRepository) GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	results := make(map[int64]*domainfeed.FeedStat, len(videoIDs))
	if len(videoIDs) == 0 {
		return results, nil
	}

	keys := make([]string, len(videoIDs))
	for i, id := range videoIDs {
		keys[i] = fmt.Sprintf("feed:stat:%d", id)
	}

	vals, err := r.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("mget l4 stat cache error: %w", err)
	}

	for i, val := range vals {
		if val == nil {
			continue
		}
		str, ok := val.(string)
		if !ok {
			continue
		}
		var stat domainfeed.FeedStat
		if err := json.Unmarshal([]byte(str), &stat); err == nil {
			results[videoIDs[i]] = &stat
		}
	}
	return results, nil
}

func (r *statCacheRepository) SetStats(ctx context.Context, stats []*domainfeed.FeedStat, ttl time.Duration) error {
	if len(stats) == 0 {
		return nil
	}
	pipe := r.rdb.Pipeline()
	for _, stat := range stats {
		if stat == nil {
			continue
		}
		bytes, err := json.Marshal(stat)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("feed:stat:%d", stat.VideoID)
		pipe.Set(ctx, key, bytes, ttl)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("set l4 stat cache error: %w", err)
	}
	return nil
}

func (r *statCacheRepository) InvalidateStat(ctx context.Context, videoID int64) error {
	if err := r.rdb.Del(ctx, fmt.Sprintf("feed:stat:%d", videoID)).Err(); err != nil {
		return fmt.Errorf("invalidate l4 stat error: %w", err)
	}
	return nil
}

// exposureRepository implements domainfeed.ExposureRepository for recommendation deduplication.
type exposureRepository struct {
	rdb redis.Cmdable
}

// NewExposureRepository creates an ExposureRepository instance.
func NewExposureRepository(rdb redis.Cmdable) domainfeed.ExposureRepository {
	return &exposureRepository{rdb: rdb}
}

func (r *exposureRepository) RecordExposures(ctx context.Context, viewerID int64, videoIDs []int64) error {
	if viewerID <= 0 || len(videoIDs) == 0 {
		return nil
	}
	key := fmt.Sprintf("reco:exposed:%d", viewerID)
	members := make([]interface{}, len(videoIDs))
	for i, id := range videoIDs {
		members[i] = id
	}

	pipe := r.rdb.Pipeline()
	pipe.SAdd(ctx, key, members...)
	pipe.Expire(ctx, key, domainfeed.ExposureTTL)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("record exposures error: %w", err)
	}
	return nil
}

func (r *exposureRepository) IsExposed(ctx context.Context, viewerID int64, videoID int64) (bool, error) {
	if viewerID <= 0 || videoID <= 0 {
		return false, nil
	}
	key := fmt.Sprintf("reco:exposed:%d", viewerID)
	exposed, err := r.rdb.SIsMember(ctx, key, videoID).Result()
	if err != nil {
		return false, fmt.Errorf("check exposure error: %w", err)
	}
	return exposed, nil
}
