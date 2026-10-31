package interaction

import (
	"time"

	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
)

// LikeModel maps to the `likes` database table.
type LikeModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	UserID    int64     `gorm:"column:user_id;not null;uniqueIndex:uk_user_video,priority:1;index:idx_user_id,priority:1"`
	VideoID   int64     `gorm:"column:video_id;not null;uniqueIndex:uk_user_video,priority:2;index:idx_video"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

func (LikeModel) TableName() string {
	return "likes"
}

func toDomainLike(m *LikeModel) *domaininteraction.Like {
	if m == nil {
		return nil
	}
	return &domaininteraction.Like{
		ID:        m.ID,
		UserID:    m.UserID,
		VideoID:   m.VideoID,
		CreatedAt: m.CreatedAt,
	}
}

// CommentModel maps to the `comments` database table.
type CommentModel struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	VideoID        int64     `gorm:"column:video_id;not null;index:idx_video_status_id,priority:1;index:idx_video_status_hot,priority:1"`
	UserID         int64     `gorm:"column:user_id;not null;uniqueIndex:uk_user_idempotency,priority:1"`
	ParentID       int64     `gorm:"column:parent_id;not null;default:0"`
	Content        string    `gorm:"column:content;size:500;not null"`
	LikeCount      int64     `gorm:"column:like_count;not null;default:0;index:idx_video_status_hot,priority:3"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:64;not null;uniqueIndex:uk_user_idempotency,priority:2"`
	Status         int8      `gorm:"column:status;not null;default:1;index:idx_video_status_id,priority:2;index:idx_video_status_hot,priority:2"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

func (CommentModel) TableName() string {
	return "comments"
}

func fromDomainComment(c *domaininteraction.Comment) *CommentModel {
	if c == nil {
		return nil
	}
	return &CommentModel{
		ID:             c.ID,
		VideoID:        c.VideoID,
		UserID:         c.UserID,
		ParentID:       c.ParentID,
		Content:        c.Content,
		LikeCount:      c.LikeCount,
		IdempotencyKey: c.IdempotencyKey,
		Status:         int8(c.Status),
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

func toDomainComment(m *CommentModel) *domaininteraction.Comment {
	if m == nil {
		return nil
	}
	return &domaininteraction.Comment{
		ID:             m.ID,
		VideoID:        m.VideoID,
		UserID:         m.UserID,
		ParentID:       m.ParentID,
		Content:        m.Content,
		LikeCount:      m.LikeCount,
		IdempotencyKey: m.IdempotencyKey,
		Status:         domaininteraction.CommentStatus(m.Status),
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

// CommentLikeModel maps to the `comment_likes` database table.
type CommentLikeModel struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	UserID    int64     `gorm:"column:user_id;not null;uniqueIndex:uk_user_comment,priority:1"`
	CommentID int64     `gorm:"column:comment_id;not null;uniqueIndex:uk_user_comment,priority:2;index:idx_comment"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

func (CommentLikeModel) TableName() string {
	return "comment_likes"
}
