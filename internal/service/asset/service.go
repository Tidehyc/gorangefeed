package asset

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
)

// Service provides user asset orchestration across favorite folders and playback history.
type Service struct {
	favoriteRepo domainasset.FavoriteRepository
	historyRepo  domainasset.HistoryRepository
	videoRepo    domainvideo.Repository
}

// NewService constructs an asset service with required persistence repositories.
func NewService(
	favoriteRepo domainasset.FavoriteRepository,
	historyRepo domainasset.HistoryRepository,
	videoRepo domainvideo.Repository,
) (*Service, error) {
	if favoriteRepo == nil {
		return nil, errors.New("favorite repository cannot be nil")
	}
	if historyRepo == nil {
		return nil, errors.New("history repository cannot be nil")
	}
	if videoRepo == nil {
		return nil, errors.New("video repository cannot be nil")
	}

	return &Service{
		favoriteRepo: favoriteRepo,
		historyRepo:  historyRepo,
		videoRepo:    videoRepo,
	}, nil
}

// FavoriteVideo adds a video to a user's favorite folder (default folder if folderID is nil/0).
// Creates an outbox event (interaction.favorited) to drive hot rank weighting.
func (s *Service) FavoriteVideo(ctx context.Context, userID, videoID int64, folderID *int64) (*domainasset.FavoriteItem, bool, error) {
	if userID <= 0 {
		return nil, false, domainasset.ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, false, domainasset.ErrInvalidVideoID
	}

	// 1. Verify target video exists and is in published state
	v, err := s.videoRepo.FindByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return nil, false, domainvideo.ErrVideoNotFound
		}
		return nil, false, fmt.Errorf("find video error: %w", err)
	}
	if !v.IsPublished() {
		return nil, false, domainvideo.ErrVideoNotFound
	}

	// 2. Resolve destination favorite folder
	var targetFolderID int64
	if folderID == nil || *folderID <= 0 {
		defaultFolder, err := s.favoriteRepo.GetOrCreateDefaultFolder(ctx, userID)
		if err != nil {
			return nil, false, fmt.Errorf("resolve default folder error: %w", err)
		}
		targetFolderID = defaultFolder.ID
	} else {
		folder, err := s.favoriteRepo.FindFolderByID(ctx, *folderID)
		if err != nil {
			return nil, false, err
		}
		if folder.UserID != userID {
			return nil, false, domainasset.ErrFolderAccessDenied
		}
		targetFolderID = folder.ID
	}

	// 3. Create Outbox event for real-time engagement and hot score weighting
	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeInteractionFavorited, map[string]any{
		"user_id":     userID,
		"video_id":    videoID,
		"author_id":   v.AuthorID,
		"favorite_id": targetFolderID,
		"occurred_at": time.Now().UTC(),
	})
	if err != nil {
		return nil, false, fmt.Errorf("create outbox event error: %w", err)
	}

	// 4. Save item and outbox event atomically
	item := &domainasset.FavoriteItem{
		FavoriteID: targetFolderID,
		VideoID:    videoID,
		UserID:     userID,
	}

	inserted, err := s.favoriteRepo.AddItem(ctx, item, outboxEvent)
	if err != nil {
		return nil, false, fmt.Errorf("add favorite item error: %w", err)
	}

	return item, inserted, nil
}

// UnfavoriteVideo removes a video from a user's favorite folder and emits interaction.unfavorited outbox event.
func (s *Service) UnfavoriteVideo(ctx context.Context, userID, videoID int64, folderID *int64) (bool, error) {
	if userID <= 0 {
		return false, domainasset.ErrInvalidUserID
	}
	if videoID <= 0 {
		return false, domainasset.ErrInvalidVideoID
	}

	// 1. Verify target video
	v, err := s.videoRepo.FindByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return false, domainvideo.ErrVideoNotFound
		}
		return false, fmt.Errorf("find video error: %w", err)
	}

	// 2. Resolve folder
	var targetFolderID int64
	if folderID == nil || *folderID <= 0 {
		defaultFolder, err := s.favoriteRepo.GetOrCreateDefaultFolder(ctx, userID)
		if err != nil {
			return false, fmt.Errorf("resolve default folder error: %w", err)
		}
		targetFolderID = defaultFolder.ID
	} else {
		targetFolderID = *folderID
	}

	// 3. Create Outbox event
	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeInteractionUnfavorited, map[string]any{
		"user_id":     userID,
		"video_id":    videoID,
		"author_id":   v.AuthorID,
		"favorite_id": targetFolderID,
		"occurred_at": time.Now().UTC(),
	})
	if err != nil {
		return false, fmt.Errorf("create outbox event error: %w", err)
	}

	// 4. Remove item and persist outbox event
	removed, err := s.favoriteRepo.RemoveItem(ctx, targetFolderID, videoID, outboxEvent)
	if err != nil {
		return false, fmt.Errorf("remove favorite item error: %w", err)
	}

	return removed, nil
}

// CreateFolder creates a new custom favorite folder for a user.
func (s *Service) CreateFolder(ctx context.Context, userID int64, name, description string, isPublic bool) (*domainasset.FavoriteFolder, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}

	folder := &domainasset.FavoriteFolder{
		UserID:      userID,
		Name:        name,
		Description: description,
		IsPublic:    isPublic,
		IsDefault:   false,
	}

	if err := folder.Validate(); err != nil {
		return nil, err
	}

	if err := s.favoriteRepo.CreateFolder(ctx, folder); err != nil {
		return nil, fmt.Errorf("create folder error: %w", err)
	}

	return folder, nil
}

// ListFolders returns all favorite folders of a user, ensuring the default folder exists and is first.
func (s *Service) ListFolders(ctx context.Context, userID int64) ([]*domainasset.FavoriteFolder, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}

	// Ensure default folder is initialized
	if _, err := s.favoriteRepo.GetOrCreateDefaultFolder(ctx, userID); err != nil {
		return nil, fmt.Errorf("ensure default folder error: %w", err)
	}

	return s.favoriteRepo.ListFolders(ctx, userID)
}

// DeleteFolder deletes a custom favorite folder, cascading items and updating video_stats.
func (s *Service) DeleteFolder(ctx context.Context, userID, folderID int64) error {
	if userID <= 0 {
		return domainasset.ErrInvalidUserID
	}
	if folderID <= 0 {
		return domainasset.ErrInvalidFolderID
	}

	return s.favoriteRepo.DeleteFolder(ctx, userID, folderID)
}

// ListFolderItems retrieves items inside a favorite folder with cursor pagination.
func (s *Service) ListFolderItems(ctx context.Context, viewerID, folderID int64, cursor int64, limit int) ([]*domainasset.FavoriteItem, int64, bool, error) {
	if folderID <= 0 {
		return nil, 0, false, domainasset.ErrInvalidFolderID
	}

	folder, err := s.favoriteRepo.FindFolderByID(ctx, folderID)
	if err != nil {
		return nil, 0, false, err
	}

	// Privacy check: private folder is only viewable by its owner
	if !folder.IsPublic && folder.UserID != viewerID {
		return nil, 0, false, domainasset.ErrFolderAccessDenied
	}

	return s.favoriteRepo.ListItems(ctx, folderID, cursor, limit)
}

// IsFavorited checks whether the user has favorited a video across any folder.
func (s *Service) IsFavorited(ctx context.Context, userID, videoID int64) (bool, error) {
	if userID <= 0 || videoID <= 0 {
		return false, nil
	}
	return s.favoriteRepo.IsFavorited(ctx, userID, videoID)
}

// BatchIsFavorited batch checks favorite status for multiple videos.
func (s *Service) BatchIsFavorited(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error) {
	return s.favoriteRepo.BatchIsFavorited(ctx, userID, videoIDs)
}

// RecordWatchProgress records or updates playback progress for a video.
func (s *Service) RecordWatchProgress(ctx context.Context, userID, videoID int64, positionMs, durationMs int) (*domainasset.WatchHistory, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, domainasset.ErrInvalidVideoID
	}
	if positionMs < 0 {
		return nil, domainasset.ErrInvalidPlaybackPosition
	}
	if durationMs < 0 {
		return nil, domainasset.ErrInvalidDuration
	}

	h := &domainasset.WatchHistory{
		UserID:    userID,
		VideoID:   videoID,
		WatchedAt: time.Now().UTC(),
	}
	h.UpdateProgress(positionMs, durationMs)

	if err := s.historyRepo.RecordProgress(ctx, h); err != nil {
		return nil, fmt.Errorf("record watch progress error: %w", err)
	}

	return h, nil
}

// RecordWatchProgressSync synchronously records progress to both Redis and MySQL.
func (s *Service) RecordWatchProgressSync(ctx context.Context, userID, videoID int64, positionMs, durationMs int) (*domainasset.WatchHistory, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, domainasset.ErrInvalidVideoID
	}
	if positionMs < 0 {
		return nil, domainasset.ErrInvalidPlaybackPosition
	}
	if durationMs < 0 {
		return nil, domainasset.ErrInvalidDuration
	}

	h := &domainasset.WatchHistory{
		UserID:    userID,
		VideoID:   videoID,
		WatchedAt: time.Now().UTC(),
	}
	h.UpdateProgress(positionMs, durationMs)

	if err := s.historyRepo.RecordProgressSync(ctx, h); err != nil {
		return nil, fmt.Errorf("record watch progress sync error: %w", err)
	}

	return h, nil
}

// GetWatchProgress retrieves current watch progress for breakpoint resumption.
func (s *Service) GetWatchProgress(ctx context.Context, userID, videoID int64) (*domainasset.WatchHistory, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, domainasset.ErrInvalidVideoID
	}

	return s.historyRepo.GetProgress(ctx, userID, videoID)
}

// ListWatchHistory retrieves the user's latest watch history up to limit entries.
func (s *Service) ListWatchHistory(ctx context.Context, userID int64, limit int) ([]*domainasset.WatchHistory, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}
	return s.historyRepo.ListHistory(ctx, userID, limit)
}

// ClearWatchHistory wipes all watch history for the user across Redis and MySQL.
func (s *Service) ClearWatchHistory(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return domainasset.ErrInvalidUserID
	}
	return s.historyRepo.ClearHistory(ctx, userID)
}
