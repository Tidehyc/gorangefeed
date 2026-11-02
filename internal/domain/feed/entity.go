package feed

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
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
	SceneLikeRank  Scene = "like_rank"

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

	// HotMinuteBucketTTL is the expiration time for single 1-minute hot engagement buckets.
	HotMinuteBucketTTL = 2 * time.Hour
	// HotWindowSnapshotTTL is the expiration time for aggregated 60-minute hot window snapshots.
	HotWindowSnapshotTTL = 2 * time.Minute
	// HotWindowBucketCount is the default number of 1-minute buckets in a sliding hot window (60 minutes).
	HotWindowBucketCount = 60

	// Hot scoring weights for engagement events
	ScoreLikeDelta          = 3.0
	ScoreUnlikeDelta        = -3.0
	ScoreCommentDelta       = 5.0
	ScoreCommentDeleteDelta = -5.0
	ScorePlayDelta          = 0.5
	ScoreCompleteDelta      = 2.0
)

// NormalizeScene sanitizes and normalizes the scene string.
func NormalizeScene(scene string) Scene {
	s := strings.TrimSpace(strings.ToLower(scene))
	switch Scene(s) {
	case SceneGlobal, SceneFollowing, SceneHot, SceneRecommend, SceneLikeRank:
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
	IsFollowed  bool       `json:"is_followed"`
}

// FeedPage represents a paginated list of feed items.
type FeedPage struct {
	Items      []*FeedItem `json:"items"`
	NextCursor int64       `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

// LikeRankCursor represents a composite cursor for like count ranking.
type LikeRankCursor struct {
	LikeCount int64 `json:"like_count"`
	VideoID   int64 `json:"video_id"`
}

// LikeRankPage represents a paginated list of feed items ordered by like count.
type LikeRankPage struct {
	Items      []*FeedItem `json:"items"`
	NextCursor string      `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

// EncodeLikeRankCursor encodes likeCount and videoID into a cursor string.
func EncodeLikeRankCursor(likeCount, videoID int64) string {
	if likeCount <= 0 && videoID <= 0 {
		return ""
	}
	return fmt.Sprintf("%d_%d", likeCount, videoID)
}

// DecodeLikeRankCursor decodes a like rank cursor string into likeCount and videoID.
func DecodeLikeRankCursor(cursor string) (likeCount, videoID int64, err error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" || cursor == "0" {
		return 0, 0, nil
	}
	parts := strings.Split(cursor, "_")
	if len(parts) != 2 {
		return 0, 0, ErrInvalidCursor
	}
	likeCount, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || likeCount < 0 {
		return 0, 0, ErrInvalidCursor
	}
	videoID, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil || videoID < 0 {
		return 0, 0, ErrInvalidCursor
	}
	return likeCount, videoID, nil
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

// HotCursor stores state for stable snapshot-based pagination of the hot feed.
type HotCursor struct {
	Scene  string `json:"scene"`
	AsOf   int64  `json:"as_of"`
	Offset int    `json:"offset"`
}

// EncodeHotCursor encodes asOf unix timestamp and offset into a transparent base64 cursor.
func EncodeHotCursor(asOf int64, offset int) string {
	if asOf <= 0 && offset <= 0 {
		return ""
	}
	hc := HotCursor{
		Scene:  string(SceneHot),
		AsOf:   asOf,
		Offset: offset,
	}
	b, err := json.Marshal(hc)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
}

// DecodeHotCursor decodes a transparent cursor into asOf unix timestamp and offset.
func DecodeHotCursor(cursor string) (asOf int64, offset int, err error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" || cursor == "0" {
		return 0, 0, nil
	}
	b, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		// Fallback for non-url standard base64
		b, err = base64.StdEncoding.DecodeString(cursor)
		if err != nil {
			return 0, 0, ErrInvalidHotCursor
		}
	}
	var hc HotCursor
	if err := json.Unmarshal(b, &hc); err != nil {
		return 0, 0, ErrInvalidHotCursor
	}
	if hc.AsOf < 0 || hc.Offset < 0 {
		return 0, 0, ErrInvalidHotCursor
	}
	return hc.AsOf, hc.Offset, nil
}

// RecommendCursor stores state for diversified multi-source recommendation pagination.
type RecommendCursor struct {
	Scene string `json:"scene"`
	Page  int    `json:"page"`
	Seed  int64  `json:"seed"`
}

// EncodeRecommendCursor encodes recommendation page index and random seed into a base64 cursor.
func EncodeRecommendCursor(page int, seed int64) string {
	if page <= 1 && seed <= 0 {
		return ""
	}
	rc := RecommendCursor{
		Scene: string(SceneRecommend),
		Page:  page,
		Seed:  seed,
	}
	b, err := json.Marshal(rc)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
}

// DecodeRecommendCursor decodes a recommendation cursor into page index and seed.
func DecodeRecommendCursor(cursor string) (page int, seed int64, err error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" || cursor == "0" {
		return 1, 0, nil
	}
	b, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		b, err = base64.StdEncoding.DecodeString(cursor)
		if err != nil {
			return 1, 0, ErrInvalidRecommendCursor
		}
	}
	var rc RecommendCursor
	if err := json.Unmarshal(b, &rc); err != nil {
		return 1, 0, ErrInvalidRecommendCursor
	}
	if rc.Page < 1 {
		return 1, 0, ErrInvalidRecommendCursor
	}
	return rc.Page, rc.Seed, nil
}

// HotScoreAction defines the source action influencing engagement score.
type HotScoreAction string

const (
	ActionLike          HotScoreAction = "like"
	ActionUnlike        HotScoreAction = "unlike"
	ActionComment       HotScoreAction = "comment"
	ActionCommentDelete HotScoreAction = "comment_delete"
	ActionPlay          HotScoreAction = "play"
	ActionComplete      HotScoreAction = "complete"
)

// HotScoreEvent encapsulates an engagement event for hot timeline score calculation.
type HotScoreEvent struct {
	EventID    string         `json:"event_id"`
	VideoID    int64          `json:"video_id"`
	Action     HotScoreAction `json:"action"`
	Delta      float64        `json:"delta"`
	OccurredAt time.Time      `json:"occurred_at"`
}
