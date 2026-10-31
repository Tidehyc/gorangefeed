package relation

import (
	"time"

	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
)

// FollowModel maps to the `follows` database table.
type FollowModel struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	FollowerID int64     `gorm:"column:follower_id;not null;uniqueIndex:uk_follower_vlogger,priority:1;index:idx_follower_id,priority:1"`
	VloggerID  int64     `gorm:"column:vlogger_id;not null;uniqueIndex:uk_follower_vlogger,priority:2;index:idx_vlogger_id,priority:1"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

func (FollowModel) TableName() string {
	return "follows"
}

// RelationStatsModel maps to the `relation_stats` database table.
type RelationStatsModel struct {
	UserID         int64     `gorm:"primaryKey;column:user_id"`
	FollowingCount int64     `gorm:"column:following_count;not null;default:0"`
	FollowerCount  int64     `gorm:"column:follower_count;not null;default:0"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

func (RelationStatsModel) TableName() string {
	return "relation_stats"
}

func toDomainStats(m *RelationStatsModel) *domainrelation.RelationStats {
	if m == nil {
		return nil
	}
	return &domainrelation.RelationStats{
		UserID:         m.UserID,
		FollowingCount: m.FollowingCount,
		FollowerCount:  m.FollowerCount,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}
