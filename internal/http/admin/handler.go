package admin

import (
	"strconv"

	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	servicemoderation "github.com/Tidehyc/gorangefeed/internal/service/moderation"
	"github.com/gin-gonic/gin"
)

// Handler handles administrative content compliance and user moderation endpoints.
type Handler struct {
	svc servicemoderation.Service
}

// NewHandler constructs an admin Handler.
func NewHandler(svc servicemoderation.Service) *Handler {
	return &Handler{svc: svc}
}

// ListReviewsRequest represents pagination query params for review tasks.
type ListReviewsRequest struct {
	Status int8  `form:"status"`
	Cursor int64 `form:"cursor"`
	Limit  int   `form:"limit"`
}

// ListReviews handles GET /api/v1/admin/reviews.
func (h *Handler) ListReviews(c *gin.Context) {
	var req ListReviewsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("invalid query parameters"))
		return
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	tasks, nextCursor, hasMore, err := h.svc.ListReviewTasks(c.Request.Context(), domainmoderation.ReviewStatus(req.Status), req.Cursor, limit)
	if err != nil {
		response.Fail(c, response.ErrInternal(err.Error()))
		return
	}

	response.OK(c, gin.H{
		"items":       tasks,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

// ReviewVideoRequest represents body for reviewing a video.
type ReviewVideoRequest struct {
	Action     string `json:"action" binding:"required"` // "approve" or "reject"
	ReviewNote string `json:"review_note"`
}

// ReviewVideo handles POST /api/v1/admin/videos/:id/review.
func (h *Handler) ReviewVideo(c *gin.Context) {
	videoID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("invalid video id"))
		return
	}

	var req ReviewVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("invalid request body: action is required"))
		return
	}

	reviewerID := middleware.MustGetUserID(c)

	if err := h.svc.ReviewVideo(c.Request.Context(), videoID, reviewerID, req.Action, req.ReviewNote); err != nil {
		if err == domainmoderation.ErrTaskNotFound {
			response.Fail(c, response.ErrNotFound("review task not found"))
			return
		}
		if err == domainmoderation.ErrTaskAlreadyProcessed {
			response.Fail(c, response.ErrConflict("review task is already processed"))
			return
		}
		if err == domainmoderation.ErrInvalidAction {
			response.Fail(c, response.ErrInvalidParam("action must be 'approve' or 'reject'"))
			return
		}
		response.Fail(c, response.ErrInternal(err.Error()))
		return
	}

	statusStr := "approved"
	if req.Action == "reject" {
		statusStr = "rejected"
	}

	response.OK(c, gin.H{
		"video_id": videoID,
		"action":   req.Action,
		"status":   statusStr,
	})
}

// TakedownRequest represents body for taking down a video.
type TakedownRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// TakedownVideo handles POST /api/v1/admin/videos/:id/takedown.
func (h *Handler) TakedownVideo(c *gin.Context) {
	videoID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("invalid video id"))
		return
	}

	var req TakedownRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("reason is required for takedown"))
		return
	}

	operatorID := middleware.MustGetUserID(c)

	if err := h.svc.TakedownVideo(c.Request.Context(), videoID, operatorID, req.Reason); err != nil {
		if err == domainmoderation.ErrTaskAlreadyProcessed {
			response.Fail(c, response.ErrConflict("video is already taken down"))
			return
		}
		response.Fail(c, response.ErrInternal(err.Error()))
		return
	}

	response.OK(c, gin.H{
		"video_id":      videoID,
		"status":        "takedown",
		"index_revoked": true,
	})
}

// BanUserRequest represents body for banning a user.
type BanUserRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// BanUser handles POST /api/v1/admin/users/:id/ban.
func (h *Handler) BanUser(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		response.Fail(c, response.ErrInvalidParam("invalid user id"))
		return
	}

	var req BanUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("reason is required for banning user"))
		return
	}

	operatorID := middleware.MustGetUserID(c)

	if err := h.svc.BanUser(c.Request.Context(), userID, operatorID, req.Reason); err != nil {
		response.Fail(c, response.ErrInternal(err.Error()))
		return
	}

	response.OK(c, gin.H{
		"user_id":       userID,
		"status":        "suspended",
		"token_revoked": true,
	})
}

