package asset

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

// Service defines application contracts for asset HTTP controllers.
type Service interface {
	FavoriteVideo(ctx context.Context, userID, videoID int64, folderID *int64) (*domainasset.FavoriteItem, bool, error)
	UnfavoriteVideo(ctx context.Context, userID, videoID int64, folderID *int64) (bool, error)

	CreateFolder(ctx context.Context, userID int64, name, description string, isPublic bool) (*domainasset.FavoriteFolder, error)
	ListFolders(ctx context.Context, userID int64) ([]*domainasset.FavoriteFolder, error)
	DeleteFolder(ctx context.Context, userID, folderID int64) error
	ListFolderItems(ctx context.Context, viewerID, folderID int64, cursor int64, limit int) ([]*domainasset.FavoriteItem, int64, bool, error)

	IsFavorited(ctx context.Context, userID, videoID int64) (bool, error)
	BatchIsFavorited(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error)

	RecordWatchProgress(ctx context.Context, userID, videoID int64, positionMs, durationMs int) (*domainasset.WatchHistory, error)
	GetWatchProgress(ctx context.Context, userID, videoID int64) (*domainasset.WatchHistory, error)
	ListWatchHistory(ctx context.Context, userID int64, limit int) ([]*domainasset.WatchHistory, error)
	ClearWatchHistory(ctx context.Context, userID int64) error
}

// Handler handles user asset HTTP requests (favorites and watch history).
type Handler struct {
	svc Service
}

// NewHandler creates a new Handler with injected asset service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// --- Request / Response DTOs ---

// FavoriteVideoRequest represents input payload for favoriting a video.
type FavoriteVideoRequest struct {
	FolderID *int64 `json:"folder_id"`
}

// FavoriteVideoResponse returns favorite relationship details.
type FavoriteVideoResponse struct {
	ID         int64 `json:"id"`
	FavoriteID int64 `json:"favorite_id"`
	VideoID    int64 `json:"video_id"`
	UserID     int64 `json:"user_id"`
	IsNew      bool  `json:"is_new"`
}

// CreateFolderRequest represents input payload for creating a custom folder.
type CreateFolderRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	IsPublic    *bool  `json:"is_public"`
}

// ListFoldersResponse returns user's favorite folders.
type ListFoldersResponse struct {
	Folders []*domainasset.FavoriteFolder `json:"folders"`
}

// ListFolderItemsResponse returns paginated favorite items in a folder.
type ListFolderItemsResponse struct {
	Items      []*domainasset.FavoriteItem `json:"items"`
	NextCursor int64                       `json:"next_cursor"`
	HasMore    bool                        `json:"has_more"`
}

// RecordProgressRequest represents input payload for reporting watch progress.
type RecordProgressRequest struct {
	VideoID        int64 `json:"video_id"`
	LastPositionMs int   `json:"last_position_ms"`
	DurationMs     int   `json:"duration_ms"`
}

// ListHistoryResponse returns user's playback history list.
type ListHistoryResponse struct {
	Items []*domainasset.WatchHistory `json:"items"`
}

// --- Video Favorite Handlers ---

// FavoriteVideo handles POST /api/v1/videos/:id/favorite.
func (h *Handler) FavoriteVideo(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	videoID, err := parseIDParam(c, "id", "videoId")
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid video ID"))
		return
	}

	var req FavoriteVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, context.Canceled) && c.Request.ContentLength > 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid request payload: "+err.Error()))
		return
	}

	// Also allow folder_id from query if body not specified
	if req.FolderID == nil {
		if qFolder := c.Query("folder_id"); qFolder != "" {
			if fid, err := strconv.ParseInt(qFolder, 10, 64); err == nil && fid > 0 {
				req.FolderID = &fid
			}
		}
	}

	item, isNew, err := h.svc.FavoriteVideo(c.Request.Context(), userID, videoID, req.FolderID)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, FavoriteVideoResponse{
		ID:         item.ID,
		FavoriteID: item.FavoriteID,
		VideoID:    item.VideoID,
		UserID:     item.UserID,
		IsNew:      isNew,
	})
}

// UnfavoriteVideo handles DELETE /api/v1/videos/:id/favorite.
func (h *Handler) UnfavoriteVideo(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	videoID, err := parseIDParam(c, "id", "videoId")
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid video ID"))
		return
	}

	var folderID *int64
	if qFolder := c.Query("folder_id"); qFolder != "" {
		if fid, err := strconv.ParseInt(qFolder, 10, 64); err == nil && fid > 0 {
			folderID = &fid
		}
	}
	if folderID == nil && c.Request.ContentLength > 0 {
		var req FavoriteVideoRequest
		if err := c.ShouldBindJSON(&req); err == nil && req.FolderID != nil {
			folderID = req.FolderID
		}
	}

	removed, err := h.svc.UnfavoriteVideo(c.Request.Context(), userID, videoID, folderID)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, gin.H{
		"success": true,
		"removed": removed,
	})
}

// --- Favorite Folder Handlers ---

// CreateFolder handles POST /api/v1/user/favorites.
func (h *Handler) CreateFolder(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	var req CreateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request payload: "+err.Error()))
		return
	}

	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}

	folder, err := h.svc.CreateFolder(c.Request.Context(), userID, strings.TrimSpace(req.Name), req.Description, isPublic)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.Success(c, http.StatusCreated, folder)
}

// ListFolders handles GET /api/v1/user/favorites.
func (h *Handler) ListFolders(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	folders, err := h.svc.ListFolders(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, ListFoldersResponse{
		Folders: folders,
	})
}

// DeleteFolder handles DELETE /api/v1/user/favorites/:id.
func (h *Handler) DeleteFolder(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	folderID, err := parseIDParam(c, "id")
	if err != nil || folderID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid folder ID"))
		return
	}

	if err := h.svc.DeleteFolder(c.Request.Context(), userID, folderID); err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, gin.H{"success": true})
}

// ListFolderItems handles GET /api/v1/user/favorites/:id/items.
func (h *Handler) ListFolderItems(c *gin.Context) {
	viewerID, _ := middleware.GetUserID(c)

	folderID, err := parseIDParam(c, "id")
	if err != nil || folderID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid folder ID"))
		return
	}

	cursor, _ := strconv.ParseInt(c.DefaultQuery("cursor", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	items, nextCursor, hasMore, err := h.svc.ListFolderItems(c.Request.Context(), viewerID, folderID, cursor, limit)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, ListFolderItemsResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// --- Watch History Handlers ---

// RecordHistory handles POST /api/v1/user/history.
func (h *Handler) RecordHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	var req RecordProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request payload: "+err.Error()))
		return
	}

	if req.VideoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid video_id"))
		return
	}

	hist, err := h.svc.RecordWatchProgress(c.Request.Context(), userID, req.VideoID, req.LastPositionMs, req.DurationMs)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, hist)
}

// RecordWatchProgress handles POST /api/v1/videos/:id/history.
func (h *Handler) RecordWatchProgress(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	videoID, err := parseIDParam(c, "id", "videoId")
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid video ID"))
		return
	}

	var req struct {
		LastPositionMs int `json:"last_position_ms"`
		DurationMs     int `json:"duration_ms"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request payload: "+err.Error()))
		return
	}

	hist, err := h.svc.RecordWatchProgress(c.Request.Context(), userID, videoID, req.LastPositionMs, req.DurationMs)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, hist)
}

// GetWatchProgress handles GET /api/v1/videos/:id/history (breakpoint resumption).
func (h *Handler) GetWatchProgress(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	videoID, err := parseIDParam(c, "id", "videoId")
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid video ID"))
		return
	}

	progress, err := h.svc.GetWatchProgress(c.Request.Context(), userID, videoID)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	if progress == nil {
		response.OK(c, gin.H{
			"video_id":         videoID,
			"last_position_ms": 0,
			"duration_ms":      0,
			"is_finished":      false,
		})
		return
	}

	response.OK(c, progress)
}

// ListHistory handles GET /api/v1/user/history.
func (h *Handler) ListHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	items, err := h.svc.ListWatchHistory(c.Request.Context(), userID, limit)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, ListHistoryResponse{
		Items: items,
	})
}

// ClearHistory handles DELETE /api/v1/user/history.
func (h *Handler) ClearHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	if err := h.svc.ClearWatchHistory(c.Request.Context(), userID); err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, gin.H{"success": true})
}

// --- Helpers ---

func parseIDParam(c *gin.Context, paramNames ...string) (int64, error) {
	for _, p := range paramNames {
		val := c.Param(p)
		if val != "" {
			return strconv.ParseInt(val, 10, 64)
		}
	}
	return 0, errors.New("param not found")
}

func (h *Handler) mapError(err error) *response.AppError {
	if err == nil {
		return nil
	}
	var appErr *response.AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	switch {
	case errors.Is(err, domainvideo.ErrVideoNotFound),
		errors.Is(err, domainasset.ErrFolderNotFound),
		errors.Is(err, domainasset.ErrFavoriteItemNotFound):
		return response.ErrNotFound(err.Error())

	case errors.Is(err, domainasset.ErrFolderAccessDenied):
		return response.ErrForbidden(err.Error())

	case errors.Is(err, domainasset.ErrInvalidUserID),
		errors.Is(err, domainasset.ErrInvalidVideoID),
		errors.Is(err, domainasset.ErrInvalidFolderID),
		errors.Is(err, domainasset.ErrEmptyFolderName),
		errors.Is(err, domainasset.ErrFolderNameTooLong),
		errors.Is(err, domainasset.ErrFolderDescTooLong),
		errors.Is(err, domainasset.ErrCannotDeleteDefaultFolder),
		errors.Is(err, domainasset.ErrInvalidPlaybackPosition),
		errors.Is(err, domainasset.ErrInvalidDuration),
		errors.Is(err, domainasset.ErrInvalidRetentionLimit):
		return response.ErrInvalidParam(err.Error())

	default:
		return response.ErrInternal(err.Error())
	}
}
