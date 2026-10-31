package relation

import (
	"context"
	"errors"
	"strconv"

	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

// Service defines application contracts for relation HTTP handling.
type Service interface {
	Follow(ctx context.Context, followerID, vloggerID int64) error
	Unfollow(ctx context.Context, followerID, vloggerID int64) error
	GetRelation(ctx context.Context, userID, targetID int64) (*domainrelation.Friendship, error)
	ListFollowings(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error)
	ListFollowers(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error)
	GetStats(ctx context.Context, userID int64) (*domainrelation.RelationStats, error)
}

// Handler handles HTTP requests for user relationships (follows, followers, friendship).
type Handler struct {
	svc Service
}

// NewHandler creates a new relation Handler instance.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// ListRelationsResponse represents the response for followers and followings list endpoints.
type ListRelationsResponse struct {
	Items      []*domainrelation.UserProfile `json:"items"`
	NextCursor int64                         `json:"next_cursor"`
	HasMore    bool                          `json:"has_more"`
}

// Follow handles POST /api/v1/users/:userId/follow.
func (h *Handler) Follow(c *gin.Context) {
	followerID := c.GetInt64(middleware.ContextKeyUserID)
	if followerID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	targetIDStr := c.Param("userId")
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil || targetID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid userId"))
		return
	}

	if err := h.svc.Follow(c.Request.Context(), followerID, targetID); err != nil {
		response.Fail(c, mapRelationError(err))
		return
	}

	response.OK(c, gin.H{})
}

// Unfollow handles DELETE /api/v1/users/:userId/follow.
func (h *Handler) Unfollow(c *gin.Context) {
	followerID := c.GetInt64(middleware.ContextKeyUserID)
	if followerID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	targetIDStr := c.Param("userId")
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil || targetID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid userId"))
		return
	}

	if err := h.svc.Unfollow(c.Request.Context(), followerID, targetID); err != nil {
		response.Fail(c, mapRelationError(err))
		return
	}

	response.OK(c, gin.H{})
}

// ListFollowings handles GET /api/v1/users/:userId/followings.
func (h *Handler) ListFollowings(c *gin.Context) {
	viewerID := c.GetInt64(middleware.ContextKeyUserID)

	targetIDStr := c.Param("userId")
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil || targetID <= 0 {
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
	if limit > 30 {
		limit = 30
	}

	items, nextCursor, hasMore, err := h.svc.ListFollowings(c.Request.Context(), targetID, viewerID, cursor, limit)
	if err != nil {
		response.Fail(c, mapRelationError(err))
		return
	}

	if items == nil {
		items = []*domainrelation.UserProfile{}
	}

	response.OK(c, ListRelationsResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// ListFollowers handles GET /api/v1/users/:userId/followers.
func (h *Handler) ListFollowers(c *gin.Context) {
	viewerID := c.GetInt64(middleware.ContextKeyUserID)

	targetIDStr := c.Param("userId")
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil || targetID <= 0 {
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
	if limit > 30 {
		limit = 30
	}

	items, nextCursor, hasMore, err := h.svc.ListFollowers(c.Request.Context(), targetID, viewerID, cursor, limit)
	if err != nil {
		response.Fail(c, mapRelationError(err))
		return
	}

	if items == nil {
		items = []*domainrelation.UserProfile{}
	}

	response.OK(c, ListRelationsResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// GetRelation handles GET /api/v1/users/:userId/relation.
func (h *Handler) GetRelation(c *gin.Context) {
	currentUserID := c.GetInt64(middleware.ContextKeyUserID)
	if currentUserID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	targetIDStr := c.Param("userId")
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil || targetID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid userId"))
		return
	}

	friendship, err := h.svc.GetRelation(c.Request.Context(), currentUserID, targetID)
	if err != nil {
		response.Fail(c, mapRelationError(err))
		return
	}

	response.OK(c, friendship)
}

func mapRelationError(err error) error {
	var appErr *response.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	switch {
	case errors.Is(err, domainrelation.ErrSelfFollow),
		errors.Is(err, domainrelation.ErrSelfUnfollow),
		errors.Is(err, domainrelation.ErrInvalidFollowerID),
		errors.Is(err, domainrelation.ErrInvalidVloggerID):
		return response.ErrInvalidParam(err.Error())
	case errors.Is(err, domainrelation.ErrTargetUserNotFound):
		return response.ErrNotFound("Target user not found")
	default:
		return response.ErrInternal(err.Error())
	}
}
