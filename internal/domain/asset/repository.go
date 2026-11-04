package asset

import (
	"context"

	"github.com/Tidehyc/gorangefeed/internal/domain/outbox"
)

// FavoriteRepository defines persistence contracts for favorite folders and items.
type FavoriteRepository interface {
	// GetOrCreateDefaultFolder returns the user's default favorite folder, initializing one if none exists.
	GetOrCreateDefaultFolder(ctx context.Context, userID int64) (*FavoriteFolder, error)
	// CreateFolder creates a new custom favorite folder for the user.
	CreateFolder(ctx context.Context, folder *FavoriteFolder) error
	// FindFolderByID retrieves a folder by its ID.
	FindFolderByID(ctx context.Context, folderID int64) (*FavoriteFolder, error)
	// ListFolders retrieves all favorite folders belonging to a user (default folder first).
	ListFolders(ctx context.Context, userID int64) ([]*FavoriteFolder, error)
	// DeleteFolder deletes a custom favorite folder and cascades/removes its items. Default folder cannot be deleted.
	DeleteFolder(ctx context.Context, userID, folderID int64) error

	// AddItem adds a video into a specific favorite folder, atomically updating item_count and video_stats.favorite_count.
	// Optionally writes an outboxEvent within the same transaction. Returns true if newly inserted, false if already present in this folder.
	AddItem(ctx context.Context, item *FavoriteItem, outboxEvent *outbox.Event) (bool, error)
	// RemoveItem removes a video from a specific favorite folder, atomically updating item_count and video_stats.favorite_count.
	// Optionally writes an outboxEvent within the same transaction. Returns true if removed, false if not present in this folder.
	RemoveItem(ctx context.Context, favoriteID, videoID int64, outboxEvent *outbox.Event) (bool, error)
	// IsFavorited checks if a user has favorited a video in any folder.
	IsFavorited(ctx context.Context, userID, videoID int64) (bool, error)
	// BatchIsFavorited checks favorite status for multiple videos for a viewer.
	BatchIsFavorited(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error)
	// ListItems retrieves paginated favorite items from a folder with cursor pagination.
	ListItems(ctx context.Context, favoriteID int64, cursor int64, limit int) ([]*FavoriteItem, int64, bool, error)
}

// HistoryRepository defines persistence contracts for watch history and playback progress.
type HistoryRepository interface {
	// RecordProgress records or updates watch progress in the Redis sliding window (up to 100 entries) and async persists to MySQL.
	RecordProgress(ctx context.Context, history *WatchHistory) error
	// RecordProgressSync synchronously persists to both Redis and MySQL (useful for deterministic tests).
	RecordProgressSync(ctx context.Context, history *WatchHistory) error
	// GetProgress retrieves playback progress for a specific video, supporting breakpoint resumption.
	GetProgress(ctx context.Context, userID, videoID int64) (*WatchHistory, error)
	// ListHistory retrieves the recent watch history for a user up to limit entries (ordered by watched_at DESC).
	ListHistory(ctx context.Context, userID int64, limit int) ([]*WatchHistory, error)
	// ClearHistory removes all watch history for a user across both Redis and MySQL.
	ClearHistory(ctx context.Context, userID int64) error
}
