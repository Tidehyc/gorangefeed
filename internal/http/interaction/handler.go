package interaction

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

// Service defines application contracts for interaction HTTP handling.
type Service interface {
	LikeVideo(ctx context.Context, userID, videoID int64) error
	UnlikeVideo(ctx context.Context, userID, videoID int64) error
	ListMyLikedVideos(ctx context.Context, userID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error)
	CreateComment(ctx context.Context, userID, videoID, parentID int64, content, idempotencyKey string) (*domaininteraction.CommentDetail, bool, error)
	DeleteComment(ctx context.Context, userID, commentID int64) error
	ListComments(ctx context.Context, viewerID, videoID int64, sort string, cursorTime int64, cursorLikeCount, cursorID int64, limit int) ([]*domaininteraction.CommentDetail, int64, int64, bool, error)
	LikeComment(ctx context.Context, userID, commentID int64) error
	UnlikeComment(ctx context.Context, userID, commentID int64) error
}

// Handler handles HTTP requests for user interactions (likes, comments).
type Handler struct {
	svc Service
}

// NewHandler creates a new interaction Handler instance.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// CreateCommentRequest represents the payload for POST /api/v1/videos/:videoId/comments.
type CreateCommentRequest struct {
	Content        string `json:"content"`
	ParentID       int64  `json:"parent_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

// ListMyLikedVideosResponse represents the response for GET /api/v1/users/me/likes.
type ListMyLikedVideosResponse struct {
	Items      []*domainvideo.VideoDetail `json:"items"`
	NextCursor int64                      `json:"next_cursor"`
	HasMore    bool                       `json:"has_more"`
}

// ListCommentsResponse represents the response for GET /api/v1/videos/:videoId/comments.
type ListCommentsResponse struct {
	Items      []*domaininteraction.CommentDetail `json:"items"`
	NextCursor string                             `json:"next_cursor"`
	HasMore    bool                               `json:"has_more"`
}

// LikeVideo handles POST /api/v1/videos/:videoId/like.
func (h *Handler) LikeVideo(c *gin.Context) {
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

	if err := h.svc.LikeVideo(c.Request.Context(), userID, videoID); err != nil {
		response.Fail(c, mapInteractionError(err))
		return
	}

	response.OK(c, gin.H{})
}

// UnlikeVideo handles DELETE /api/v1/videos/:videoId/like.
func (h *Handler) UnlikeVideo(c *gin.Context) {
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

	if err := h.svc.UnlikeVideo(c.Request.Context(), userID, videoID); err != nil {
		response.Fail(c, mapInteractionError(err))
		return
	}

	response.OK(c, gin.H{})
}

// ListMyLikedVideos handles GET /api/v1/users/me/likes.
func (h *Handler) ListMyLikedVideos(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
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
	if limit > 30 {
		limit = 30
	}

	items, nextCursor, hasMore, err := h.svc.ListMyLikedVideos(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		response.Fail(c, mapInteractionError(err))
		return
	}

	if items == nil {
		items = []*domainvideo.VideoDetail{}
	}

	response.OK(c, ListMyLikedVideosResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// CreateComment handles POST /api/v1/videos/:videoId/comments.
func (h *Handler) CreateComment(c *gin.Context) {
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

	var req CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request body: "+err.Error()))
		return
	}

	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	}

	detail, created, err := h.svc.CreateComment(
		c.Request.Context(),
		userID,
		videoID,
		req.ParentID,
		req.Content,
		idempotencyKey,
	)
	if err != nil {
		response.Fail(c, mapInteractionError(err))
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	response.Success(c, status, gin.H{
		"comment": detail,
	})
}

// ListComments handles GET /api/v1/videos/:videoId/comments.
func (h *Handler) ListComments(c *gin.Context) {
	viewerID := c.GetInt64(middleware.ContextKeyUserID)

	videoIDStr := c.Param("videoId")
	videoID, err := strconv.ParseInt(videoIDStr, 10, 64)
	if err != nil || videoID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid videoId"))
		return
	}

	sort := strings.ToLower(strings.TrimSpace(c.DefaultQuery("sort", "latest")))
	if sort != "hot" && sort != "latest" {
		sort = "latest"
	}

	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid limit"))
		return
	}
	if limit > 30 {
		limit = 30
	}

	cursorStr := strings.TrimSpace(c.DefaultQuery("cursor", ""))

	var (
		cursorTime      int64
		cursorLikeCount int64
		cursorID        int64
	)

	if sort == "hot" {
		if cursorStr != "" {
			parts := strings.Split(cursorStr, "_")
			if len(parts) == 2 {
				cursorLikeCount, _ = strconv.ParseInt(parts[0], 10, 64)
				cursorID, _ = strconv.ParseInt(parts[1], 10, 64)
			}
		}
	} else {
		if cursorStr != "" {
			cursorTime, _ = strconv.ParseInt(cursorStr, 10, 64)
		}
	}

	items, nextTimeCursor, nextID, hasMore, err := h.svc.ListComments(
		c.Request.Context(),
		viewerID,
		videoID,
		sort,
		cursorTime,
		cursorLikeCount,
		cursorID,
		limit,
	)
	if err != nil {
		response.Fail(c, mapInteractionError(err))
		return
	}

	if items == nil {
		items = []*domaininteraction.CommentDetail{}
	}

	var nextCursor string
	if hasMore {
		if sort == "hot" {
			nextCursor = fmt.Sprintf("%d_%d", nextTimeCursor, nextID)
		} else {
			nextCursor = strconv.FormatInt(nextTimeCursor, 10)
		}
	}

	response.OK(c, ListCommentsResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// DeleteComment handles DELETE /api/v1/comments/:commentId.
func (h *Handler) DeleteComment(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	commentIDStr := c.Param("commentId")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil || commentID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid commentId"))
		return
	}

	if err := h.svc.DeleteComment(c.Request.Context(), userID, commentID); err != nil {
		response.Fail(c, mapInteractionError(err))
		return
	}

	response.OK(c, gin.H{"message": "comment deleted successfully"})
}

// LikeComment handles POST /api/v1/comments/:commentId/like.
func (h *Handler) LikeComment(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	commentIDStr := c.Param("commentId")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil || commentID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid commentId"))
		return
	}

	if err := h.svc.LikeComment(c.Request.Context(), userID, commentID); err != nil {
		response.Fail(c, mapInteractionError(err))
		return
	}

	response.OK(c, gin.H{})
}

// UnlikeComment handles DELETE /api/v1/comments/:commentId/like.
func (h *Handler) UnlikeComment(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	commentIDStr := c.Param("commentId")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil || commentID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid commentId"))
		return
	}

	if err := h.svc.UnlikeComment(c.Request.Context(), userID, commentID); err != nil {
		response.Fail(c, mapInteractionError(err))
		return
	}

	response.OK(c, gin.H{})
}

func mapInteractionError(err error) error {
	var appErr *response.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	switch {
	case errors.Is(err, domaininteraction.ErrVideoNotFound):
		return response.ErrNotFound("Video not found")
	case errors.Is(err, domaininteraction.ErrCommentNotFound):
		return response.ErrNotFound("Comment not found")
	case errors.Is(err, domaininteraction.ErrCommentForbidden):
		return response.ErrForbidden("Forbidden to operate this comment")
	case errors.Is(err, domaininteraction.ErrEmptyCommentContent),
		errors.Is(err, domaininteraction.ErrCommentContentTooLong),
		errors.Is(err, domaininteraction.ErrEmptyIdempotencyKey),
		errors.Is(err, domaininteraction.ErrIdempotencyKeyTooLong),
		errors.Is(err, domaininteraction.ErrInvalidParentComment),
		errors.Is(err, domaininteraction.ErrInvalidUserID),
		errors.Is(err, domaininteraction.ErrInvalidVideoID):
		return response.ErrInvalidParam(err.Error())
	default:
		return response.ErrInternal(err.Error())
	}
}
