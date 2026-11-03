package asset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	zsetKeyPrefix   = "asset:history:zset:"
	detailKeyPrefix = "asset:history:detail:"
	defaultAsyncTimeout = 5 * time.Second
)

// Lua script to atomically update ZSet and detail Hash, then trim entries beyond maxEntries.
const historySlidingWindowLua = `
local zsetKey = KEYS[1]
local detailKey = KEYS[2]
local score = tonumber(ARGV[1])
local member = ARGV[2]
local detail = ARGV[3]
local maxEntries = tonumber(ARGV[4])

redis.call('ZADD', zsetKey, score, member)
redis.call('HSET', detailKey, member, detail)

local total = redis.call('ZCARD', zsetKey)
if total > maxEntries then
    local excess = total - maxEntries
    local toRemove = redis.call('ZRANGE', zsetKey, 0, excess - 1)
    if #toRemove > 0 then
        redis.call('ZREMRANGEBYRANK', zsetKey, 0, excess - 1)
        for i = 1, #toRemove do
            redis.call('HDEL', detailKey, toRemove[i])
        end
    end
end
return total
`

type historyRepository struct {
	db          *gorm.DB
	rdb         redis.Cmdable
	retention   int
	slideScript *redis.Script
}

// NewHistoryRepository creates a watch history repository backed by Redis and MySQL.
func NewHistoryRepository(db *gorm.DB, rdb redis.Cmdable) domainasset.HistoryRepository {
	return NewHistoryRepositoryWithRetention(db, rdb, domainasset.DefaultMaxHistoryEntries)
}

// NewHistoryRepositoryWithRetention creates a repository with customizable retention window.
func NewHistoryRepositoryWithRetention(db *gorm.DB, rdb redis.Cmdable, retention int) domainasset.HistoryRepository {
	if retention <= 0 {
		retention = domainasset.DefaultMaxHistoryEntries
	}
	return &historyRepository{
		db:          db,
		rdb:         rdb,
		retention:   retention,
		slideScript: redis.NewScript(historySlidingWindowLua),
	}
}

func (r *historyRepository) zsetKey(userID int64) string {
	return fmt.Sprintf("%s%d", zsetKeyPrefix, userID)
}

func (r *historyRepository) detailKey(userID int64) string {
	return fmt.Sprintf("%s%d", detailKeyPrefix, userID)
}

func (r *historyRepository) RecordProgress(ctx context.Context, history *domainasset.WatchHistory) error {
	if history == nil {
		return errors.New("watch history cannot be nil")
	}
	if err := history.Validate(); err != nil {
		return err
	}
	if history.WatchedAt.IsZero() {
		history.WatchedAt = time.Now()
	}

	// 1. Maintain Redis sliding window synchronously for low-latency breakpoint tracking
	if err := r.recordInRedis(ctx, history); err != nil {
		// Log or fallback: proceed to MySQL write even if Redis transiently fails
		return r.saveToMySQL(ctx, history)
	}

	// 2. Asynchronously persist to MySQL to isolate DB write contention
	copyHistory := *history
	go func(rec domainasset.WatchHistory) {
		bgCtx, cancel := context.WithTimeout(context.Background(), defaultAsyncTimeout)
		defer cancel()
		_ = r.saveToMySQL(bgCtx, &rec)
	}(copyHistory)

	return nil
}

func (r *historyRepository) RecordProgressSync(ctx context.Context, history *domainasset.WatchHistory) error {
	if history == nil {
		return errors.New("watch history cannot be nil")
	}
	if err := history.Validate(); err != nil {
		return err
	}
	if history.WatchedAt.IsZero() {
		history.WatchedAt = time.Now()
	}

	// 1. Write to Redis
	if err := r.recordInRedis(ctx, history); err != nil {
		return err
	}

	// 2. Synchronously write to MySQL
	return r.saveToMySQL(ctx, history)
}

func (r *historyRepository) recordInRedis(ctx context.Context, history *domainasset.WatchHistory) error {
	payload := HistoryDetailPayload{
		UserID:         history.UserID,
		VideoID:        history.VideoID,
		LastPositionMs: history.LastPositionMs,
		DurationMs:     history.DurationMs,
		IsFinished:     history.IsFinished,
		WatchedAt:      history.WatchedAt,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal history payload error: %w", err)
	}

	zKey := r.zsetKey(history.UserID)
	dKey := r.detailKey(history.UserID)
	memberStr := strconv.FormatInt(history.VideoID, 10)
	scoreMs := history.WatchedAt.UnixMilli()

	_, err = r.slideScript.Run(ctx, r.rdb, []string{zKey, dKey}, scoreMs, memberStr, string(jsonBytes), r.retention).Result()
	if err != nil {
		return fmt.Errorf("redis slide script error: %w", err)
	}

	return nil
}

func (r *historyRepository) saveToMySQL(ctx context.Context, history *domainasset.WatchHistory) error {
	m := fromDomainHistory(history)
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "video_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"last_position_ms", "duration_ms", "is_finished", "watched_at", "updated_at",
		}),
	}).Create(m).Error

	if err != nil {
		return fmt.Errorf("upsert watch history error: %w", err)
	}

	history.ID = m.ID
	history.CreatedAt = m.CreatedAt
	history.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *historyRepository) GetProgress(ctx context.Context, userID, videoID int64) (*domainasset.WatchHistory, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, domainasset.ErrInvalidVideoID
	}

	// 1. Fast path: check Redis detail Hash
	dKey := r.detailKey(userID)
	memberStr := strconv.FormatInt(videoID, 10)

	val, err := r.rdb.HGet(ctx, dKey, memberStr).Result()
	if err == nil && val != "" {
		var payload HistoryDetailPayload
		if jsonErr := json.Unmarshal([]byte(val), &payload); jsonErr == nil {
			return &domainasset.WatchHistory{
				UserID:         payload.UserID,
				VideoID:        payload.VideoID,
				LastPositionMs: payload.LastPositionMs,
				DurationMs:     payload.DurationMs,
				IsFinished:     payload.IsFinished,
				WatchedAt:      payload.WatchedAt,
			}, nil
		}
	}

	// 2. Fallback: query MySQL
	var m WatchHistoryModel
	dbErr := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		First(&m).Error

	if dbErr != nil {
		if errors.Is(dbErr, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("query watch history from db error: %w", dbErr)
	}

	domainHist := toDomainHistory(&m)
	// Warm up Redis cache in background
	_ = r.recordInRedis(ctx, domainHist)

	return domainHist, nil
}

func (r *historyRepository) ListHistory(ctx context.Context, userID int64, limit int) ([]*domainasset.WatchHistory, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	zKey := r.zsetKey(userID)
	dKey := r.detailKey(userID)

	// 1. Try Redis ZSet
	members, err := r.rdb.ZRevRange(ctx, zKey, 0, int64(limit-1)).Result()
	if err == nil && len(members) > 0 {
		vals, hashErr := r.rdb.HMGet(ctx, dKey, members...).Result()
		if hashErr == nil && len(vals) == len(members) {
			histories := make([]*domainasset.WatchHistory, 0, len(members))
			allParsed := true
			for _, v := range vals {
				str, ok := v.(string)
				if !ok || str == "" {
					allParsed = false
					break
				}
				var payload HistoryDetailPayload
				if err := json.Unmarshal([]byte(str), &payload); err != nil {
					allParsed = false
					break
				}
				histories = append(histories, &domainasset.WatchHistory{
					UserID:         payload.UserID,
					VideoID:        payload.VideoID,
					LastPositionMs: payload.LastPositionMs,
					DurationMs:     payload.DurationMs,
					IsFinished:     payload.IsFinished,
					WatchedAt:      payload.WatchedAt,
				})
			}
			if allParsed {
				return histories, nil
			}
		}
	}

	// 2. Fallback to MySQL
	var models []WatchHistoryModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("watched_at DESC, id DESC").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list watch history from db error: %w", err)
	}

	histories := make([]*domainasset.WatchHistory, len(models))
	for i := range models {
		histories[i] = toDomainHistory(&models[i])
		// Warm up Redis
		_ = r.recordInRedis(ctx, histories[i])
	}

	return histories, nil
}

func (r *historyRepository) ClearHistory(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return domainasset.ErrInvalidUserID
	}

	zKey := r.zsetKey(userID)
	dKey := r.detailKey(userID)

	// 1. Clear Redis keys
	_ = r.rdb.Del(ctx, zKey, dKey).Err()

	// 2. Clear MySQL records
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&WatchHistoryModel{}).Error; err != nil {
		return fmt.Errorf("delete watch history from db error: %w", err)
	}

	return nil
}
