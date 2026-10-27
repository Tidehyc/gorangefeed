package feed

import (
	"context"
	"fmt"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	"gorm.io/gorm"
)

type feedDBRepository struct {
	db *gorm.DB
}

// NewFeedDBRepository creates a GORM-backed FeedDBRepository instance.
func NewFeedDBRepository(db *gorm.DB) domainfeed.FeedDBRepository {
	return &feedDBRepository{db: db}
}

type videoRow struct {
	ID          int64     `gorm:"column:id"`
	AuthorID    int64     `gorm:"column:author_id"`
	Title       string    `gorm:"column:title"`
	Description string    `gorm:"column:description"`
	CoverURL    string    `gorm:"column:cover_url"`
	PlayURL     string    `gorm:"column:play_url"`
	DurationMs  int       `gorm:"column:duration_ms"`
	Status      int8      `gorm:"column:status"`
	PublishedAt time.Time `gorm:"column:published_at"`
}

func (r *feedDBRepository) ListColdVideos(ctx context.Context, cursor int64, limit int) ([]*domainfeed.FeedCard, []*domainfeed.FeedStat, int64, bool, error) {
	if limit <= 0 {
		limit = domainfeed.DefaultLimit
	}
	if limit > domainfeed.MaxLimit {
		limit = domainfeed.MaxLimit
	}

	query := r.db.WithContext(ctx).Table("videos").
		Where("status = ?", 1)
	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}

	var rows []videoRow
	err := query.Order("id DESC").Limit(limit + 1).Find(&rows).Error
	if err != nil {
		return nil, nil, 0, false, fmt.Errorf("list cold videos error: %w", err)
	}

	if len(rows) == 0 {
		return []*domainfeed.FeedCard{}, []*domainfeed.FeedStat{}, 0, false, nil
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	nextCursor := rows[len(rows)-1].ID

	authorIDs := make([]int64, 0, len(rows))
	videoIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		authorIDs = append(authorIDs, row.AuthorID)
		videoIDs = append(videoIDs, row.ID)
	}

	authorMap := r.fetchAuthors(ctx, authorIDs)
	statMap := r.fetchStats(ctx, videoIDs)

	cards := make([]*domainfeed.FeedCard, len(rows))
	stats := make([]*domainfeed.FeedStat, len(rows))

	for i, row := range rows {
		cards[i] = &domainfeed.FeedCard{
			VideoID:     row.ID,
			AuthorID:    row.AuthorID,
			Title:       row.Title,
			Description: row.Description,
			CoverURL:    row.CoverURL,
			PlayURL:     row.PlayURL,
			DurationMs:  row.DurationMs,
			PublishedAt: row.PublishedAt,
			Author:      authorMap[row.AuthorID],
		}
		if s, ok := statMap[row.ID]; ok {
			stats[i] = s
		} else {
			stats[i] = &domainfeed.FeedStat{VideoID: row.ID}
		}
	}

	return cards, stats, nextCursor, hasMore, nil
}

func (r *feedDBRepository) GetCardsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	if len(videoIDs) == 0 {
		return map[int64]*domainfeed.FeedCard{}, nil
	}

	var rows []videoRow
	err := r.db.WithContext(ctx).Table("videos").
		Where("id IN (?) AND status = 1", videoIDs).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get cards by ids error: %w", err)
	}

	if len(rows) == 0 {
		return map[int64]*domainfeed.FeedCard{}, nil
	}

	authorIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		authorIDs = append(authorIDs, row.AuthorID)
	}
	authorMap := r.fetchAuthors(ctx, authorIDs)

	result := make(map[int64]*domainfeed.FeedCard, len(rows))
	for _, row := range rows {
		result[row.ID] = &domainfeed.FeedCard{
			VideoID:     row.ID,
			AuthorID:    row.AuthorID,
			Title:       row.Title,
			Description: row.Description,
			CoverURL:    row.CoverURL,
			PlayURL:     row.PlayURL,
			DurationMs:  row.DurationMs,
			PublishedAt: row.PublishedAt,
			Author:      authorMap[row.AuthorID],
		}
	}
	return result, nil
}

func (r *feedDBRepository) GetStatsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	if len(videoIDs) == 0 {
		return map[int64]*domainfeed.FeedStat{}, nil
	}
	return r.fetchStats(ctx, videoIDs), nil
}

func (r *feedDBRepository) GetVideosStatusByIDs(ctx context.Context, videoIDs []int64) (map[int64]int8, error) {
	if len(videoIDs) == 0 {
		return map[int64]int8{}, nil
	}

	type statusRow struct {
		ID     int64 `gorm:"column:id"`
		Status int8  `gorm:"column:status"`
	}

	var rows []statusRow
	err := r.db.WithContext(ctx).Table("videos").
		Select("id, status").
		Where("id IN (?)", videoIDs).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get videos status error: %w", err)
	}

	res := make(map[int64]int8, len(rows))
	for _, row := range rows {
		res[row.ID] = row.Status
	}
	return res, nil
}

func (r *feedDBRepository) CalculateFloor(ctx context.Context, duration time.Duration) (int64, error) {
	threshold := time.Now().Add(-duration)
	var minID *int64
	err := r.db.WithContext(ctx).Table("videos").
		Select("MIN(id)").
		Where("status = ? AND published_at >= ?", 1, threshold).
		Row().Scan(&minID)
	if err != nil {
		return 0, fmt.Errorf("calculate floor error: %w", err)
	}
	if minID == nil {
		return 0, nil
	}
	return *minID, nil
}

func (r *feedDBRepository) IncrViewCount(ctx context.Context, videoID int64) error {
	res := r.db.WithContext(ctx).Table("video_stats").
		Where("video_id = ?", videoID).
		Update("view_count", gorm.Expr("view_count + 1"))
	if res.Error != nil {
		return fmt.Errorf("increment view count error: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domainfeed.ErrVideoNotFound
	}
	return nil
}

func (r *feedDBRepository) fetchAuthors(ctx context.Context, authorIDs []int64) map[int64]domainfeed.FeedAuthor {
	authorMap := make(map[int64]domainfeed.FeedAuthor, len(authorIDs))
	if len(authorIDs) == 0 {
		return authorMap
	}

	type userRow struct {
		ID        int64  `gorm:"column:id"`
		Username  string `gorm:"column:username"`
		Nickname  string `gorm:"column:nickname"`
		AvatarURL string `gorm:"column:avatar_url"`
	}

	var users []userRow
	if err := r.db.WithContext(ctx).Table("users").
		Select("id, username, nickname, avatar_url").
		Where("id IN (?)", authorIDs).
		Find(&users).Error; err == nil {
		for _, u := range users {
			authorMap[u.ID] = domainfeed.FeedAuthor{
				ID:        u.ID,
				Username:  u.Username,
				Nickname:  u.Nickname,
				AvatarURL: u.AvatarURL,
			}
		}
	}
	return authorMap
}

func (r *feedDBRepository) fetchStats(ctx context.Context, videoIDs []int64) map[int64]*domainfeed.FeedStat {
	statMap := make(map[int64]*domainfeed.FeedStat, len(videoIDs))
	if len(videoIDs) == 0 {
		return statMap
	}

	type statRow struct {
		VideoID      int64 `gorm:"column:video_id"`
		LikeCount    int64 `gorm:"column:like_count"`
		CommentCount int64 `gorm:"column:comment_count"`
		ViewCount    int64 `gorm:"column:view_count"`
	}

	var stats []statRow
	if err := r.db.WithContext(ctx).Table("video_stats").
		Select("video_id, like_count, comment_count, view_count").
		Where("video_id IN (?)", videoIDs).
		Find(&stats).Error; err == nil {
		for _, s := range stats {
			statMap[s.VideoID] = &domainfeed.FeedStat{
				VideoID:      s.VideoID,
				LikeCount:    s.LikeCount,
				CommentCount: s.CommentCount,
				ViewCount:    s.ViewCount,
			}
		}
	}
	return statMap
}
