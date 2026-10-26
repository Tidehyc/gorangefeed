package video

import (
	"time"

	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
)

// VideoModel maps to the `videos` database table.
type VideoModel struct {
	ID             int64     `gorm:"primaryKey;autoIncrement"`
	AuthorID       int64     `gorm:"column:author_id;not null"`
	Title          string    `gorm:"column:title;type:varchar(128);not null"`
	Description    string    `gorm:"column:description;type:varchar(1000);not null;default:''"`
	CoverURL       string    `gorm:"column:cover_url;type:varchar(512);not null;default:''"`
	PlayURL        string    `gorm:"column:play_url;type:varchar(512);not null"`
	DurationMs     int       `gorm:"column:duration_ms;not null;default:0"`
	Status         int8      `gorm:"column:status;not null;default:1"`
	IdempotencyKey *string   `gorm:"column:idempotency_key;type:varchar(128);default:null"`
	PublishedAt    time.Time `gorm:"column:published_at;not null;default:CURRENT_TIMESTAMP(3)"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the table name for VideoModel.
func (VideoModel) TableName() string {
	return "videos"
}

// VideoStatModel maps to the `video_stats` database table.
type VideoStatModel struct {
	VideoID      int64     `gorm:"column:video_id;primaryKey"`
	LikeCount    int64     `gorm:"column:like_count;not null;default:0"`
	CommentCount int64     `gorm:"column:comment_count;not null;default:0"`
	ViewCount    int64     `gorm:"column:view_count;not null;default:0"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the table name for VideoStatModel.
func (VideoStatModel) TableName() string {
	return "video_stats"
}

func toDomainVideo(m *VideoModel) *domainvideo.Video {
	if m == nil {
		return nil
	}
	return &domainvideo.Video{
		ID:             m.ID,
		AuthorID:       m.AuthorID,
		Title:          m.Title,
		Description:    m.Description,
		CoverURL:       m.CoverURL,
		PlayURL:        m.PlayURL,
		DurationMs:     m.DurationMs,
		Status:         domainvideo.VideoStatus(m.Status),
		IdempotencyKey: m.IdempotencyKey,
		PublishedAt:    m.PublishedAt,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func toDomainStats(m *VideoStatModel) *domainvideo.VideoStats {
	if m == nil {
		return nil
	}
	return &domainvideo.VideoStats{
		VideoID:      m.VideoID,
		LikeCount:    m.LikeCount,
		CommentCount: m.CommentCount,
		ViewCount:    m.ViewCount,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func fromDomainVideo(v *domainvideo.Video) *VideoModel {
	if v == nil {
		return nil
	}
	pubAt := v.PublishedAt
	if pubAt.IsZero() {
		pubAt = time.Now()
	}
	return &VideoModel{
		ID:             v.ID,
		AuthorID:       v.AuthorID,
		Title:          v.Title,
		Description:    v.Description,
		CoverURL:       v.CoverURL,
		PlayURL:        v.PlayURL,
		DurationMs:     v.DurationMs,
		Status:         int8(v.Status),
		IdempotencyKey: v.IdempotencyKey,
		PublishedAt:    pubAt,
	}
}
