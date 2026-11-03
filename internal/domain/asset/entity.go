package asset

import (
	"strings"
	"time"
)

const (
	DefaultFolderName          = "默认收藏夹"
	DefaultFolderDesc          = "系统默认收藏夹"
	MaxFolderNameLength        = 64
	MaxFolderDescriptionLength  = 255
	DefaultMaxHistoryEntries   = 100
	PlaybackFinishThresholdRatio = 0.95
)

// FavoriteFolder represents a user-created or default playlist/folder for favoriting videos.
type FavoriteFolder struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsPublic    bool      `json:"is_public"`
	IsDefault   bool      `json:"is_default"`
	ItemCount   int64     `json:"item_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Validate ensures favorite folder invariants hold true.
func (f *FavoriteFolder) Validate() error {
	if f.UserID <= 0 {
		return ErrInvalidUserID
	}
	name := strings.TrimSpace(f.Name)
	if name == "" {
		return ErrEmptyFolderName
	}
	if len(name) > MaxFolderNameLength {
		return ErrFolderNameTooLong
	}
	if len(f.Description) > MaxFolderDescriptionLength {
		return ErrFolderDescTooLong
	}
	return nil
}

// IsOwnedBy checks whether this folder is owned by the given user.
func (f *FavoriteFolder) IsOwnedBy(userID int64) bool {
	return f.UserID == userID
}

// CanDelete checks whether this folder is eligible for deletion.
func (f *FavoriteFolder) CanDelete() error {
	if f.IsDefault {
		return ErrCannotDeleteDefaultFolder
	}
	return nil
}

// IncrItemCount increments the count of items in this folder.
func (f *FavoriteFolder) IncrItemCount() {
	f.ItemCount++
}

// DecrItemCount decrements the count of items in this folder with a lower bound of zero.
func (f *FavoriteFolder) DecrItemCount() {
	if f.ItemCount > 0 {
		f.ItemCount--
	}
}

// FavoriteItem represents a video favorited into a specific folder.
type FavoriteItem struct {
	ID         int64     `json:"id"`
	FavoriteID int64     `json:"favorite_id"`
	VideoID    int64     `json:"video_id"`
	UserID     int64     `json:"user_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// Validate checks basic identifiers for a favorite item.
func (item *FavoriteItem) Validate() error {
	if item.FavoriteID <= 0 {
		return ErrInvalidFolderID
	}
	if item.VideoID <= 0 {
		return ErrInvalidVideoID
	}
	if item.UserID <= 0 {
		return ErrInvalidUserID
	}
	return nil
}

// WatchHistory represents playback progress and breakpoint state for a video.
type WatchHistory struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	LastPositionMs int       `json:"last_position_ms"`
	DurationMs     int       `json:"duration_ms"`
	IsFinished     bool      `json:"is_finished"`
	WatchedAt      time.Time `json:"watched_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate verifies watch history invariants.
func (h *WatchHistory) Validate() error {
	if h.UserID <= 0 {
		return ErrInvalidUserID
	}
	if h.VideoID <= 0 {
		return ErrInvalidVideoID
	}
	if h.LastPositionMs < 0 {
		return ErrInvalidPlaybackPosition
	}
	if h.DurationMs < 0 {
		return ErrInvalidDuration
	}
	return nil
}

// UpdateProgress updates the playback position, duration, and marks completion if threshold reached.
func (h *WatchHistory) UpdateProgress(positionMs, durationMs int) {
	if positionMs < 0 {
		positionMs = 0
	}
	if durationMs < 0 {
		durationMs = 0
	}
	h.LastPositionMs = positionMs
	h.DurationMs = durationMs
	h.WatchedAt = time.Now()

	if durationMs > 0 && float64(positionMs)/float64(durationMs) >= PlaybackFinishThresholdRatio {
		h.IsFinished = true
	} else if positionMs >= durationMs && durationMs > 0 {
		h.IsFinished = true
	}
}

// MarkFinished marks playback as fully completed.
func (h *WatchHistory) MarkFinished() {
	h.IsFinished = true
	if h.DurationMs > 0 {
		h.LastPositionMs = h.DurationMs
	}
	h.WatchedAt = time.Now()
}

// ProgressRatio calculates the playback completion percentage (0.0 to 1.0).
func (h *WatchHistory) ProgressRatio() float64 {
	if h.DurationMs <= 0 {
		return 0.0
	}
	ratio := float64(h.LastPositionMs) / float64(h.DurationMs)
	if ratio > 1.0 {
		return 1.0
	}
	if ratio < 0.0 {
		return 0.0
	}
	return ratio
}

// HistoryRetentionPolicy is a value object specifying maximum entries retained per user.
type HistoryRetentionPolicy struct {
	MaxEntries int `json:"max_entries"`
}

// DefaultRetentionPolicy provides standard 100-entry sliding window retention.
func DefaultRetentionPolicy() HistoryRetentionPolicy {
	return HistoryRetentionPolicy{
		MaxEntries: DefaultMaxHistoryEntries,
	}
}

// Validate ensures the retention policy specifies a positive entry count.
func (p HistoryRetentionPolicy) Validate() error {
	if p.MaxEntries <= 0 {
		return ErrInvalidRetentionLimit
	}
	return nil
}
