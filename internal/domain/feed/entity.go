package feed

import (
	"strings"
	"time"
)

// Scene represents a feed delivery scenario.
type Scene string

const (
	SceneGlobal    Scene = "global"
	SceneFollowing Scene = "following"
	SceneHot       Scene = "hot"
	SceneRecommend Scene = "recommend"

	DefaultScene = SceneGlobal
)

const (
	// DefaultLimit is the default page size when none is specified.
	DefaultLimit = 10
	// MaxLimit is the maximum page size allowed per request.
	MaxLimit = 30
	// MaxScanLimit is the upper safety bound of items scanned per request to prevent CPU burn.
	MaxScanLimit = 500

	// StaleCacheTolerance defines the grace window for serving slightly expired L1 pages.
	StaleCacheTolerance = 30 * time.Second
	// L1FirstPageTTL is the L1 cache expiration for the initial page (cursor = 0).
	L1FirstPageTTL = 5 * time.Second
	// L1PaginationTTL is the L1 cache expiration for deep pagination (cursor > 0).
	L1PaginationTTL = 45 * time.Second
	// L3CardTTL is the cache TTL for static video card metadata.
	L3CardTTL = 15 * time.Minute
	// L4StatTTL is the cache TTL for volatile video engagement metrics.
	L4StatTTL = 15 * time.Second
	// ExposureTTL is the retention window for viewer exposure deduplication.
	ExposureTTL = 72 * time.Hour
)

// NormalizeScene sanitizes and normalizes the scene string.
func NormalizeScene(scene string) Scene {
	s := strings.TrimSpace(strings.ToLower(scene))
	switch Scene(s) {
	case SceneGlobal, SceneFollowing, SceneHot, SceneRecommend:
		return Scene(s)
	default:
		return DefaultScene
	}
}

// FeedAuthor provides public author details embedded in a feed item.
type FeedAuthor struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// FeedStat provides video engagement metrics.
type FeedStat struct {
	VideoID      int64 `json:"video_id"`
	LikeCount    int64 `json:"like_count"`
	CommentCount int64 `json:"comment_count"`
	ViewCount    int64 `json:"view_count"`
}

// FeedCard stores static display properties for a video in the feed.
type FeedCard struct {
	VideoID     int64      `json:"video_id"`
	AuthorID    int64      `json:"author_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	CoverURL    string     `json:"cover_url"`
	PlayURL     string     `json:"play_url"`
	DurationMs  int        `json:"duration_ms"`
	PublishedAt time.Time  `json:"published_at"`
	Author      FeedAuthor `json:"author"`
}

// FeedItem represents a fully-hydrated video item presented to the client.
type FeedItem struct {
	ID          int64      `json:"id"`
	AuthorID    int64      `json:"author_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	CoverURL    string     `json:"cover_url"`
	PlayURL     string     `json:"play_url"`
	DurationMs  int        `json:"duration_ms"`
	PublishedAt time.Time  `json:"published_at"`
	Author      FeedAuthor `json:"author"`
	Stats       FeedStat   `json:"stats"`
	IsLiked     bool       `json:"is_liked"`
}

// FeedPage represents a paginated list of feed items.
type FeedPage struct {
	Items      []*FeedItem `json:"items"`
	NextCursor int64       `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

// CachedPage stores lightweight page results in L1 cache for zero-stampede serving.
type CachedPage struct {
	VideoIDs   []int64   `json:"video_ids"`
	NextCursor int64     `json:"next_cursor"`
	HasMore    bool      `json:"has_more"`
	ExpireAt   time.Time `json:"expire_at"`
}

// IsExpired checks whether the cached page has exceeded its soft expiration time.
func (c *CachedPage) IsExpired(now time.Time) bool {
	return now.After(c.ExpireAt)
}

// CanServeStale determines if the expired cached page is within the allowable stale window.
func (c *CachedPage) CanServeStale(now time.Time) bool {
	return now.After(c.ExpireAt) && now.Before(c.ExpireAt.Add(StaleCacheTolerance))
}

// ViewEventType identifies the nature of a playback event.
type ViewEventType string

const (
	ViewEventPlay     ViewEventType = "play"
	ViewEventComplete ViewEventType = "complete"
)

// ViewEvent represents a reported playback telemetry or impression event.
type ViewEvent struct {
	VideoID         int64         `json:"video_id"`
	ViewerID        int64         `json:"viewer_id"`
	EventType       ViewEventType `json:"event_type"`
	WatchDurationMs int           `json:"watch_duration_ms"`
	Timestamp       time.Time     `json:"timestamp"`
}

// Validate checks view event field constraints.
func (e *ViewEvent) Validate() error {
	if e.VideoID <= 0 {
		return ErrInvalidVideoID
	}
	switch e.EventType {
	case ViewEventPlay, ViewEventComplete:
		return nil
	default:
		return ErrInvalidEventType
	}
}
