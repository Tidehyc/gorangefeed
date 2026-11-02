package playback

import (
	"context"
	"time"

	domainplayback "github.com/Tidehyc/gorangefeed/internal/domain/playback"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

// Service defines application contracts for playback configuration and QoS aggregation.
type Service interface {
	GetPlaybackConfig(ctx context.Context, networkType string) *domainplayback.PlaybackConfig
	ReportQualityEvents(ctx context.Context, userID int64, events []*domainplayback.PlaybackQualityEvent) (int, error)
}

// Handler handles HTTP requests for playback configuration and QoS telemetry.
type Handler struct {
	svc Service
}

// NewHandler creates a new playback Handler instance.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// ReportEventsRequest represents the payload for POST /api/v1/playback/events.
// It flexibly supports both a batch of events and a single event JSON payload.
type ReportEventsRequest struct {
	Events []*domainplayback.PlaybackQualityEvent `json:"events"`

	// Flat fields for single event telemetry submission
	VideoID           int64     `json:"video_id"`
	FirstFrameMs      int       `json:"first_frame_ms"`
	StutterCount      int       `json:"stutter_count"`
	StutterDurationMs int       `json:"stutter_duration_ms"`
	PlayDurationMs    int       `json:"play_duration_ms"`
	VideoDurationMs   int       `json:"video_duration_ms"`
	IsCompleted       bool      `json:"is_completed"`
	NetworkType       string    `json:"network_type"`
	ErrorCode         string    `json:"error_code,omitempty"`
	ReportedAt        time.Time `json:"reported_at"`
}

// GetConfig handles GET /api/v1/playback/config.
func (h *Handler) GetConfig(c *gin.Context) {
	networkType := c.Query("network_type")
	cfg := h.svc.GetPlaybackConfig(c.Request.Context(), networkType)
	response.OK(c, cfg)
}

// ReportEvents handles POST /api/v1/playback/events.
func (h *Handler) ReportEvents(c *gin.Context) {
	var req ReportEventsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("invalid playback events payload"))
		return
	}

	var events []*domainplayback.PlaybackQualityEvent
	if len(req.Events) > 0 {
		events = req.Events
	} else if req.VideoID > 0 {
		events = []*domainplayback.PlaybackQualityEvent{
			{
				VideoID:           req.VideoID,
				FirstFrameMs:      req.FirstFrameMs,
				StutterCount:      req.StutterCount,
				StutterDurationMs: req.StutterDurationMs,
				PlayDurationMs:    req.PlayDurationMs,
				VideoDurationMs:   req.VideoDurationMs,
				IsCompleted:       req.IsCompleted,
				NetworkType:       req.NetworkType,
				ErrorCode:         req.ErrorCode,
				ReportedAt:        req.ReportedAt,
			},
		}
	} else {
		response.Fail(c, response.ErrInvalidParam("at least one event or valid video_id required"))
		return
	}

	var userID int64
	if uid, ok := middleware.GetUserID(c); ok {
		userID = uid
	}

	processed, err := h.svc.ReportQualityEvents(c.Request.Context(), userID, events)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam(err.Error()))
		return
	}

	response.OK(c, gin.H{
		"success":         true,
		"processed_count": processed,
	})
}
