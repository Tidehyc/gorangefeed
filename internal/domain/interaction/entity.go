package interaction

import (
	"strings"
	"time"
)

const (
	MaxCommentContentLength = 500
	MaxIdempotencyKeyLength = 64
)

// CommentStatus represents comment publication status.
type CommentStatus int8

const (
	CommentStatusPublished CommentStatus = 1
	CommentStatusDeleted   CommentStatus = 2
)

// Like represents a video like record.
type Like struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	VideoID   int64     `json:"video_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Comment represents a comment on a video (supports 2-level nested replies).
type Comment struct {
	ID             int64         `json:"id"`
	VideoID        int64         `json:"video_id"`
	UserID         int64         `json:"user_id"`
	ParentID       int64         `json:"parent_id"` // 0 for root comment, >0 for reply
	Content        string        `json:"content"`
	LikeCount      int64         `json:"like_count"`
	IdempotencyKey string        `json:"idempotency_key"`
	Status         CommentStatus `json:"status"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// Validate checks domain invariants for Comment.
func (c *Comment) Validate() error {
	if c.UserID <= 0 {
		return ErrInvalidUserID
	}
	if c.VideoID <= 0 {
		return ErrInvalidVideoID
	}
	content := strings.TrimSpace(c.Content)
	if content == "" {
		return ErrEmptyCommentContent
	}
	if len([]rune(content)) > MaxCommentContentLength {
		return ErrCommentContentTooLong
	}
	key := strings.TrimSpace(c.IdempotencyKey)
	if key == "" {
		return ErrEmptyIdempotencyKey
	}
	if len(key) > MaxIdempotencyKeyLength {
		return ErrIdempotencyKeyTooLong
	}
	if c.ParentID < 0 {
		return ErrInvalidParentComment
	}
	return nil
}

// IsPublished checks if the comment is published.
func (c *Comment) IsPublished() bool {
	return c.Status == CommentStatusPublished
}

// CommentLike represents a like on a comment.
type CommentLike struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	CommentID int64     `json:"comment_id"`
	CreatedAt time.Time `json:"created_at"`
}

// CommentAuthor provides author public identity info.
type CommentAuthor struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

// CommentDetail aggregates comment entity, author profile, and viewer's like status.
type CommentDetail struct {
	Comment *Comment       `json:"comment"`
	Author  *CommentAuthor `json:"author"`
	IsLiked bool           `json:"is_liked"`
}
