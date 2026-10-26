package video

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
	servicevideo "github.com/Tidehyc/gorangefeed/internal/service/video"
	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for video domain.
type Handler struct {
	svc          servicevideo.Service
	videoStorage *storage.VideoStorage
}

// NewHandler creates a new video Handler instance.
func NewHandler(svc servicevideo.Service, videoStorage *storage.VideoStorage) *Handler {
	return &Handler{
		svc:          svc,
		videoStorage: videoStorage,
	}
}

// UploadVideo handles POST /api/v1/videos/upload.
// Supports single-file MP4 upload with body truncation (MaxBytesReader), magic number inspection,
// path isolation, and clean-on-failure.
func (h *Handler) UploadVideo(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	// Truncate request body to protect memory against oversized payload
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, storage.MaxVideoSize)

	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, response.ErrInvalidParam("Missing or invalid video file: "+err.Error()))
		return
	}

	playURL, size, err := h.videoStorage.SaveVideo(userID, file)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.Success(c, http.StatusCreated, UploadVideoResponse{
		PlayURL: playURL,
		Size:    size,
	})
}

// UploadCover handles POST /api/v1/videos/cover.
// Supports image upload (.jpg, .jpeg, .png, .webp) ≤10MB for video covers.
func (h *Handler) UploadCover(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, storage.MaxCoverSize)

	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, response.ErrInvalidParam("Missing or invalid cover file: "+err.Error()))
		return
	}

	coverURL, err := h.videoStorage.SaveCover(userID, file)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.Success(c, http.StatusCreated, UploadCoverResponse{
		CoverURL: coverURL,
	})
}

// Publish handles POST /api/v1/videos.
// Validates video metadata, handles idempotency keys, and transactionally publishes the video.
func (h *Handler) Publish(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	var req PublishVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request body: "+err.Error()))
		return
	}

	// Idempotency key can be passed via Header "Idempotency-Key" or JSON body "idempotency_key"
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	}

	detail, err := h.svc.Publish(
		c.Request.Context(),
		userID,
		req.Title,
		req.Description,
		req.PlayURL,
		req.CoverURL,
		req.DurationMs,
		idempotencyKey,
	)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.Success(c, http.StatusCreated, detail)
}

// GetDetail handles GET /api/v1/videos/:videoId.
// Supports soft auth (extracts viewer identity if provided to determine like status).
func (h *Handler) GetDetail(c *gin.Context) {
	videoIDStr := c.Param("videoId")
	videoID, err := strconv.ParseInt(videoIDStr, 10, 64)
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid videoId"))
		return
	}

	currentUserID := c.GetInt64(middleware.ContextKeyUserID)

	detail, err := h.svc.GetDetail(c.Request.Context(), videoID, currentUserID)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, detail)
}

// ListAuthorVideos handles GET /api/v1/users/:userId/videos.
// Supports cursor-based pagination (query params cursor & limit).
func (h *Handler) ListAuthorVideos(c *gin.Context) {
	authorIDStr := c.Param("userId")
	authorID, err := strconv.ParseInt(authorIDStr, 10, 64)
	if err != nil || authorID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid userId"))
		return
	}

	cursorStr := c.DefaultQuery("cursor", "0")
	cursor, err := strconv.ParseInt(cursorStr, 10, 64)
	if err != nil || cursor < 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid cursor"))
		return
	}

	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid limit"))
		return
	}

	videos, nextCursor, hasMore, err := h.svc.ListAuthorVideos(c.Request.Context(), authorID, cursor, limit)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, ListAuthorVideosResponse{
		Videos:     videos,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// DeleteVideo handles DELETE /api/v1/videos/:videoId.
// Allows only the video author to take down their video.
func (h *Handler) DeleteVideo(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	videoIDStr := c.Param("videoId")
	videoID, err := strconv.ParseInt(videoIDStr, 10, 64)
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid videoId"))
		return
	}

	if err := h.svc.DeleteVideo(c.Request.Context(), videoID, userID); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"message": "video deleted successfully"})
}
