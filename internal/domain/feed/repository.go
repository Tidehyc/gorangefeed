package feed

import (
	"context"
	"time"
)

// TimelineRepository manages the L2 Redis sorted-set global timeline.
type TimelineRepository interface {
	// GetFloor returns the lowest video ID currently retained in the timeline.
	GetFloor(ctx context.Context) (int64, error)
	// SetFloor sets the floor watermark explicitly.
	SetFloor(ctx context.Context, floor int64) error
	// Append adds a new video ID to the timeline using its ID as the score.
	Append(ctx context.Context, videoID int64) error
	// Trim removes all entries with scores strictly below floor and sets the floor key atomically.
	Trim(ctx context.Context, floor int64) error
	// ListByCursor queries the timeline strictly below cursor (or from top if cursor=0) with limit.
	ListByCursor(ctx context.Context, cursor int64, limit int) ([]int64, error)
	// Rebuild repopulates the timeline with a list of video IDs and resets the floor.
	Rebuild(ctx context.Context, videoIDs []int64, floor int64) error
}

// PageCacheRepository handles L1 logical-expiration page caching.
type PageCacheRepository interface {
	// GetPage retrieves a cached page from L1.
	GetPage(ctx context.Context, key string) (*CachedPage, bool, error)
	// SetPage stores a cached page into L1 with physical TTL.
	SetPage(ctx context.Context, key string, page *CachedPage, ttl time.Duration) error
	// DeletePage evicts a cached page from L1.
	DeletePage(ctx context.Context, key string) error
}

// CardCacheRepository manages L3 static video card caching.
type CardCacheRepository interface {
	// GetCards fetches video cards by IDs in batch. Returns only found cards.
	GetCards(ctx context.Context, videoIDs []int64) (map[int64]*FeedCard, error)
	// SetCards writes video cards to cache with TTL.
	SetCards(ctx context.Context, cards []*FeedCard, ttl time.Duration) error
}

// StatCacheRepository manages L4 volatile video engagement metrics caching.
type StatCacheRepository interface {
	// GetStats fetches video stats by IDs in batch. Returns only found stats.
	GetStats(ctx context.Context, videoIDs []int64) (map[int64]*FeedStat, error)
	// SetStats writes video stats to cache with TTL.
	SetStats(ctx context.Context, stats []*FeedStat, ttl time.Duration) error
	// InvalidateStat evicts or invalidates a video's stat cache.
	InvalidateStat(ctx context.Context, videoID int64) error
}

// FeedDBRepository queries MySQL for persistent fallback and cold data.
type FeedDBRepository interface {
	// ListColdVideos queries published videos older than cursor from MySQL.
	ListColdVideos(ctx context.Context, cursor int64, limit int) ([]*FeedCard, []*FeedStat, int64, bool, error)
	// GetCardsByIDs queries static video information along with author profiles.
	GetCardsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*FeedCard, error)
	// GetStatsByIDs queries engagement counters for a batch of videos.
	GetStatsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*FeedStat, error)
	// GetVideosStatusByIDs checks publication status for video IDs to filter hidden entries.
	GetVideosStatusByIDs(ctx context.Context, videoIDs []int64) (map[int64]int8, error)
	// CalculateFloor computes the minimum published video ID within the given duration window.
	CalculateFloor(ctx context.Context, duration time.Duration) (int64, error)
	// IncrViewCount increments the view count for a video atomically in MySQL.
	IncrViewCount(ctx context.Context, videoID int64) error
}

// ExposureRepository stores client exposure impressions to prevent repeated recommendations.
type ExposureRepository interface {
	// RecordExposures records impression of video IDs for a viewer with 72h expiration.
	RecordExposures(ctx context.Context, viewerID int64, videoIDs []int64) error
	// IsExposed checks if a viewer has already been exposed to a video.
	IsExposed(ctx context.Context, viewerID int64, videoID int64) (bool, error)
}
