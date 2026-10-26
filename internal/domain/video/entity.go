package video

import (
	"strings"
	"time"
)

// VideoStatus represents the publication status of a video.
type VideoStatus int8

const (
	// VideoStatusPublished represents an active, published video visible to everyone.
	VideoStatusPublished VideoStatus = 1
	// VideoStatusHidden represents a video taken down or deleted by author.
	VideoStatusHidden VideoStatus = 2
)

const (
	MaxTitleLength          = 128
	MaxDescriptionLength    = 1000
	MaxIdempotencyKeyLength = 128
)

// Video represents the core domain entity for a short video.
type Video struct {
	ID             int64       `json:"id"`
	AuthorID       int64       `json:"author_id"`
	Title          string      `json:"title"`
	Description    string      `json:"description"`
	CoverURL       string      `json:"cover_url"`
	PlayURL        string      `json:"play_url"`
	DurationMs     int         `json:"duration_ms"`
	Status         VideoStatus `json:"status"`
	IdempotencyKey *string     `json:"idempotency_key,omitempty"`
	PublishedAt    time.Time   `json:"published_at"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// IsPublished checks if video is in published state.
func (v *Video) IsPublished() bool {
	return v.Status == VideoStatusPublished
}

// IsHidden checks if video is hidden/taken down.
func (v *Video) IsHidden() bool {
	return v.Status == VideoStatusHidden
}

// Hide marks the video as hidden.
func (v *Video) Hide() {
	v.Status = VideoStatusHidden
}

// Validate performs basic domain invariant checks on video fields.
func (v *Video) Validate() error {
	if v.AuthorID <= 0 {
		return ErrInvalidAuthorID
	}
	title := strings.TrimSpace(v.Title)
	if title == "" {
		return ErrEmptyTitle
	}
	if len(title) > MaxTitleLength {
		return ErrTitleTooLong
	}
	if len(v.Description) > MaxDescriptionLength {
		return ErrDescriptionTooLong
	}
	if strings.TrimSpace(v.PlayURL) == "" {
		return ErrEmptyPlayURL
	}
	if v.IdempotencyKey != nil && len(*v.IdempotencyKey) > MaxIdempotencyKeyLength {
		return ErrIdempotencyKeyTooLong
	}
	return nil
}

// VideoStats represents engagement metrics for a video, decoupled from the video entity
// to isolate high-frequency write contention.
type VideoStats struct {
	VideoID      int64     `json:"video_id"`
	LikeCount    int64     `json:"like_count"`
	CommentCount int64     `json:"comment_count"`
	ViewCount    int64     `json:"view_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AuthorProfile provides public author identity details embedded in video responses.
type AuthorProfile struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// VideoDetail aggregates the video entity, engagement metrics, author info, and viewer state.
type VideoDetail struct {
	Video   *Video         `json:"video"`
	Stats   *VideoStats    `json:"stats"`
	Author  *AuthorProfile `json:"author"`
	IsLiked bool           `json:"is_liked"`
}
