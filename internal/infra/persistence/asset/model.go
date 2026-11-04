package asset

import (
	"time"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
)

// FavoriteModel maps to the `favorites` table in MySQL.
type FavoriteModel struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID      int64     `gorm:"column:user_id;not null;index:idx_user_id;uniqueIndex:uk_user_default,priority:1"`
	Name        string    `gorm:"column:name;type:varchar(64);not null;default:''"`
	Description string    `gorm:"column:description;type:varchar(255);not null;default:''"`
	IsPublic    *int8     `gorm:"column:is_public;not null;default:1"`
	IsDefault   *int8     `gorm:"column:is_default;uniqueIndex:uk_user_default,priority:2;default:null"`
	ItemCount   int64     `gorm:"column:item_count;not null;default:0"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the table name for FavoriteModel.
func (FavoriteModel) TableName() string {
	return "favorites"
}

// FavoriteItemModel maps to the `favorite_items` table in MySQL.
type FavoriteItemModel struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	FavoriteID int64     `gorm:"column:favorite_id;not null;uniqueIndex:uk_favorite_video,priority:1;index:idx_favorite_created"`
	VideoID    int64     `gorm:"column:video_id;not null;uniqueIndex:uk_favorite_video,priority:2;index:idx_user_video,priority:2;index:idx_video_id"`
	UserID     int64     `gorm:"column:user_id;not null;index:idx_user_video,priority:1"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName returns the table name for FavoriteItemModel.
func (FavoriteItemModel) TableName() string {
	return "favorite_items"
}

// WatchHistoryModel maps to the `watch_history` table in MySQL.
type WatchHistoryModel struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int64     `gorm:"column:user_id;not null;uniqueIndex:uk_user_video,priority:1;index:idx_user_watched,priority:1"`
	VideoID        int64     `gorm:"column:video_id;not null;uniqueIndex:uk_user_video,priority:2"`
	LastPositionMs int       `gorm:"column:last_position_ms;not null;default:0"`
	DurationMs     int       `gorm:"column:duration_ms;not null;default:0"`
	IsFinished     int8      `gorm:"column:is_finished;not null;default:0"`
	WatchedAt      time.Time `gorm:"column:watched_at;not null;default:CURRENT_TIMESTAMP(3);index:idx_user_watched,priority:2"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the table name for WatchHistoryModel.
func (WatchHistoryModel) TableName() string {
	return "watch_history"
}

// HistoryDetailPayload is stored as JSON in Redis Hash for fast O(1) progress lookup.
type HistoryDetailPayload struct {
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	LastPositionMs int       `json:"last_position_ms"`
	DurationMs     int       `json:"duration_ms"`
	IsFinished     bool      `json:"is_finished"`
	WatchedAt      time.Time `json:"watched_at"`
}

func toDomainFolder(m *FavoriteModel) *domainasset.FavoriteFolder {
	if m == nil {
		return nil
	}
	return &domainasset.FavoriteFolder{
		ID:          m.ID,
		UserID:      m.UserID,
		Name:        m.Name,
		Description: m.Description,
		IsPublic:    m.IsPublic != nil && *m.IsPublic == 1,
		IsDefault:   m.IsDefault != nil && *m.IsDefault == 1,
		ItemCount:   m.ItemCount,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func fromDomainFolder(f *domainasset.FavoriteFolder) *FavoriteModel {
	if f == nil {
		return nil
	}
	var isPub int8 = 0
	if f.IsPublic {
		isPub = 1
	}
	var isDef *int8
	if f.IsDefault {
		val := int8(1)
		isDef = &val
	}
	return &FavoriteModel{
		ID:          f.ID,
		UserID:      f.UserID,
		Name:        f.Name,
		Description: f.Description,
		IsPublic:    &isPub,
		IsDefault:   isDef,
		ItemCount:   f.ItemCount,
		CreatedAt:   f.CreatedAt,
		UpdatedAt:   f.UpdatedAt,
	}
}

func toDomainItem(m *FavoriteItemModel) *domainasset.FavoriteItem {
	if m == nil {
		return nil
	}
	return &domainasset.FavoriteItem{
		ID:         m.ID,
		FavoriteID: m.FavoriteID,
		VideoID:    m.VideoID,
		UserID:     m.UserID,
		CreatedAt:  m.CreatedAt,
	}
}

func fromDomainItem(item *domainasset.FavoriteItem) *FavoriteItemModel {
	if item == nil {
		return nil
	}
	return &FavoriteItemModel{
		ID:         item.ID,
		FavoriteID: item.FavoriteID,
		VideoID:    item.VideoID,
		UserID:     item.UserID,
		CreatedAt:  item.CreatedAt,
	}
}

func toDomainHistory(m *WatchHistoryModel) *domainasset.WatchHistory {
	if m == nil {
		return nil
	}
	return &domainasset.WatchHistory{
		ID:             m.ID,
		UserID:         m.UserID,
		VideoID:        m.VideoID,
		LastPositionMs: m.LastPositionMs,
		DurationMs:     m.DurationMs,
		IsFinished:     m.IsFinished == 1,
		WatchedAt:      m.WatchedAt,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func fromDomainHistory(h *domainasset.WatchHistory) *WatchHistoryModel {
	if h == nil {
		return nil
	}
	var isFin int8 = 0
	if h.IsFinished {
		isFin = 1
	}
	watAt := h.WatchedAt
	if watAt.IsZero() {
		watAt = time.Now()
	}
	return &WatchHistoryModel{
		ID:             h.ID,
		UserID:         h.UserID,
		VideoID:        h.VideoID,
		LastPositionMs: h.LastPositionMs,
		DurationMs:     h.DurationMs,
		IsFinished:     isFin,
		WatchedAt:      watAt,
		CreatedAt:      h.CreatedAt,
		UpdatedAt:      h.UpdatedAt,
	}
}
