package feed

import (
	"errors"
	"strconv"
	"strings"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for feed retrieval and view telemetry.
type Handler struct {
	svc servicefeed.Service
}

// NewHandler creates a new feed Handler instance.
func NewHandler(svc servicefeed.Service) *Handler {
	return &Handler{svc: svc}
}

// RecordViewRequest represents the payload for POST /api/v1/videos/:videoId/view.
type RecordViewRequest struct {
	EventType       string `json:"event_type"`
	WatchDurationMs int    `json:"watch_duration_ms"`
}

// GetGlobalFeed handles GET /api/v1/feed/global.
// Supports soft auth (extracts viewer identity if authenticated, otherwise 0 for anon).
func (h *Handler) GetGlobalFeed(c *gin.Context) {
	viewerID := c.GetInt64(middleware.ContextKeyUserID)

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
	if limit > domainfeed.MaxLimit {
		limit = domainfeed.MaxLimit
	}

	page, err := h.svc.GetGlobalFeed(c.Request.Context(), viewerID, cursor, limit)
	if err != nil {
		if errors.Is(err, domainfeed.ErrInvalidCursor) || errors.Is(err, domainfeed.ErrInvalidLimit) {
			response.Fail(c, response.ErrInvalidParam(err.Error()))
			return
		}
		response.Fail(c, err)
		return
	}

	response.OK(c, page)
}

// RecordView handles POST /api/v1/videos/:videoId/view.
// Reports video playback or view impression, driving view_count increment and exposure tracking.
func (h *Handler) RecordView(c *gin.Context) {
	videoIDStr := c.Param("videoId")
	videoID, err := strconv.ParseInt(videoIDStr, 10, 64)
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid videoId"))
		return
	}

	viewerID := c.GetInt64(middleware.ContextKeyUserID)

	var req RecordViewRequest
	if c.Request.Body != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, response.ErrInvalidParam("Invalid request body"))
			return
		}
	}

	eventType := domainfeed.ViewEventType(strings.TrimSpace(req.EventType))
	if eventType == "" {
		eventType = domainfeed.ViewEventPlay
	}

	event := domainfeed.ViewEvent{
		VideoID:         videoID,
		ViewerID:        viewerID,
		EventType:       eventType,
		WatchDurationMs: req.WatchDurationMs,
		Timestamp:       time.Now(),
	}

	if err := h.svc.RecordView(c.Request.Context(), viewerID, videoID, event); err != nil {
		if errors.Is(err, domainfeed.ErrVideoNotFound) {
			response.Fail(c, response.ErrNotFound("Video not found"))
			return
		}
		if errors.Is(err, domainfeed.ErrInvalidVideoID) || errors.Is(err, domainfeed.ErrInvalidEventType) {
			response.Fail(c, response.ErrInvalidParam(err.Error()))
			return
		}
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"message": "view recorded successfully"})
}
