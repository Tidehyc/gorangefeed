package account

import (
	"context"
	"errors"
	"fmt"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"gorm.io/gorm"
)

// videoStatusPublished mirrors domainvideo.VideoStatusPublished. It is duplicated here as a
// constant rather than imported so this file stays free of the video package: the whole point
// of ProfileStatsProvider is that account does not depend on the content domain.
const videoStatusPublished = 1

type profileStatsRepository struct {
	db *gorm.DB
}

// NewProfileStatsRepository creates the aggregating read model behind /users/me and /users/:id.
func NewProfileStatsRepository(db *gorm.DB) domainaccount.ProfileStatsProvider {
	return &profileStatsRepository{db: db}
}

// GetProfileStats collects the four counters shown on a profile page.
//
// Missing rows are not errors: a brand-new account has no relation_stats row and someone with
// no published video has no video_stats rows, both of which mean "0" rather than "failed".
func (r *profileStatsRepository) GetProfileStats(ctx context.Context, userID int64) (domainaccount.ProfileStats, error) {
	stats := domainaccount.ProfileStats{}

	var relation struct {
		FollowingCount int64 `gorm:"column:following_count"`
		FollowerCount  int64 `gorm:"column:follower_count"`
	}
	err := r.db.WithContext(ctx).Table("relation_stats").
		Select("following_count, follower_count").
		Where("user_id = ?", userID).
		Take(&relation).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return stats, fmt.Errorf("query relation stats error: %w", err)
	}
	stats.FollowingCount = relation.FollowingCount
	stats.FollowerCount = relation.FollowerCount

	// LEFT JOIN keeps a video whose stats row is somehow missing in the work count; SUM over
	// the missing side contributes 0 through COALESCE, so the two numbers stay consistent.
	var content struct {
		WorkCount  int64 `gorm:"column:work_count"`
		TotalLikes int64 `gorm:"column:total_likes"`
	}
	err = r.db.WithContext(ctx).Table("videos AS v").
		Select("COUNT(*) AS work_count, COALESCE(SUM(s.like_count), 0) AS total_likes").
		Joins("LEFT JOIN video_stats AS s ON s.video_id = v.id").
		Where("v.author_id = ? AND v.status = ?", userID, videoStatusPublished).
		Take(&content).Error
	if err != nil {
		return stats, fmt.Errorf("query content stats error: %w", err)
	}
	stats.WorkCount = content.WorkCount
	stats.TotalLikes = content.TotalLikes

	return stats, nil
}
