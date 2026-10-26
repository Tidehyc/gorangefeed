package video

import (
	"context"
	"errors"
	"fmt"

	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type gormRepository struct {
	db *gorm.DB
}

// NewRepository creates a GORM-backed Video repository.
func NewRepository(db *gorm.DB) domainvideo.Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) CreateWithStats(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) error {
	if video == nil {
		return errors.New("video cannot be nil")
	}
	if stats == nil {
		stats = &domainvideo.VideoStats{}
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		m := fromDomainVideo(video)
		if err := tx.Create(m).Error; err != nil {
			var mysqlErr *mysql.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
				return domainvideo.ErrDuplicateIdempotencyKey
			}
			return fmt.Errorf("create video error: %w", err)
		}

		// Reflect generated ID and timestamps back to domain video
		video.ID = m.ID
		video.PublishedAt = m.PublishedAt
		video.CreatedAt = m.CreatedAt
		video.UpdatedAt = m.UpdatedAt

		statModel := &VideoStatModel{
			VideoID:      m.ID,
			LikeCount:    stats.LikeCount,
			CommentCount: stats.CommentCount,
			ViewCount:    stats.ViewCount,
		}
		if err := tx.Create(statModel).Error; err != nil {
			return fmt.Errorf("create video_stats error: %w", err)
		}

		stats.VideoID = statModel.VideoID
		stats.CreatedAt = statModel.CreatedAt
		stats.UpdatedAt = statModel.UpdatedAt

		return nil
	})
}

func (r *gormRepository) FindByID(ctx context.Context, id int64) (*domainvideo.Video, error) {
	var m VideoModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", id, int8(domainvideo.VideoStatusPublished)).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainvideo.ErrVideoNotFound
		}
		return nil, fmt.Errorf("find video by id error: %w", err)
	}
	return toDomainVideo(&m), nil
}

func (r *gormRepository) FindDetailByID(ctx context.Context, id int64) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	v, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	var statModel VideoStatModel
	statErr := r.db.WithContext(ctx).Where("video_id = ?", id).First(&statModel).Error
	if statErr != nil {
		if errors.Is(statErr, gorm.ErrRecordNotFound) {
			// Gracefully fallback to initialized zero stats if row missing
			return v, &domainvideo.VideoStats{
				VideoID:   id,
				CreatedAt: v.CreatedAt,
				UpdatedAt: v.UpdatedAt,
			}, nil
		}
		return nil, nil, fmt.Errorf("find video_stats error: %w", statErr)
	}

	return v, toDomainStats(&statModel), nil
}

func (r *gormRepository) FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	if key == "" {
		return nil, nil, domainvideo.ErrVideoNotFound
	}

	var m VideoModel
	err := r.db.WithContext(ctx).
		Where("author_id = ? AND idempotency_key = ?", authorID, key).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, domainvideo.ErrVideoNotFound
		}
		return nil, nil, fmt.Errorf("find video by author and idempotency_key error: %w", err)
	}

	v := toDomainVideo(&m)
	var statModel VideoStatModel
	if statErr := r.db.WithContext(ctx).Where("video_id = ?", m.ID).First(&statModel).Error; statErr == nil {
		return v, toDomainStats(&statModel), nil
	}

	return v, &domainvideo.VideoStats{VideoID: v.ID}, nil
}

func (r *gormRepository) ListByAuthorCursor(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.Video, []*domainvideo.VideoStats, int64, bool, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	query := r.db.WithContext(ctx).Model(&VideoModel{}).
		Where("author_id = ? AND status = ?", authorID, int8(domainvideo.VideoStatusPublished))
	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}

	var models []VideoModel
	if err := query.Order("id DESC").Limit(limit + 1).Find(&models).Error; err != nil {
		return nil, nil, 0, false, fmt.Errorf("list author videos error: %w", err)
	}

	if len(models) == 0 {
		return []*domainvideo.Video{}, []*domainvideo.VideoStats{}, 0, false, nil
	}

	hasMore := len(models) > limit
	if hasMore {
		models = models[:limit]
	}

	var nextCursor int64 = 0
	if hasMore {
		nextCursor = models[len(models)-1].ID
	}

	videoIDs := make([]int64, len(models))
	videos := make([]*domainvideo.Video, len(models))
	for i := range models {
		videoIDs[i] = models[i].ID
		videos[i] = toDomainVideo(&models[i])
	}

	// Batch load stats for returned videos
	var statModels []VideoStatModel
	if err := r.db.WithContext(ctx).Where("video_id IN ?", videoIDs).Find(&statModels).Error; err != nil {
		return nil, nil, 0, false, fmt.Errorf("batch load video stats error: %w", err)
	}

	statsMap := make(map[int64]*domainvideo.VideoStats, len(statModels))
	for i := range statModels {
		statsMap[statModels[i].VideoID] = toDomainStats(&statModels[i])
	}

	stats := make([]*domainvideo.VideoStats, len(videos))
	for i, v := range videos {
		if s, ok := statsMap[v.ID]; ok {
			stats[i] = s
		} else {
			stats[i] = &domainvideo.VideoStats{VideoID: v.ID}
		}
	}

	return videos, stats, nextCursor, hasMore, nil
}

func (r *gormRepository) UpdateStatus(ctx context.Context, id int64, authorID int64, status domainvideo.VideoStatus) error {
	var m VideoModel
	err := r.db.WithContext(ctx).First(&m, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainvideo.ErrVideoNotFound
		}
		return fmt.Errorf("find video for update status error: %w", err)
	}

	if m.AuthorID != authorID {
		return domainvideo.ErrVideoForbidden
	}

	if m.Status == int8(status) {
		return nil // Idempotent: already at target status
	}

	if err := r.db.WithContext(ctx).Model(&VideoModel{}).Where("id = ?", id).Update("status", int8(status)).Error; err != nil {
		return fmt.Errorf("update video status error: %w", err)
	}

	return nil
}
