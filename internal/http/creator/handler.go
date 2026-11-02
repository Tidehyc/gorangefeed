package creator

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	servicecreator "github.com/Tidehyc/gorangefeed/internal/service/creator"
	"github.com/gin-gonic/gin"
)

// Service defines application contracts for creator HTTP handling.
type Service interface {
	SaveDraft(ctx context.Context, authorID int64, req servicecreator.SaveDraftRequest) (*domaincreator.Draft, error)
	GetDraft(ctx context.Context, draftID int64, authorID int64) (*domaincreator.Draft, error)
	UpdateDraft(ctx context.Context, draftID int64, authorID int64, req servicecreator.UpdateDraftRequest) (*domaincreator.Draft, error)
	DeleteDraft(ctx context.Context, draftID int64, authorID int64) error
	ListDrafts(ctx context.Context, authorID int64, cursor string, limit int) ([]*domaincreator.Draft, string, bool, error)
	PublishDraft(ctx context.Context, draftID int64, authorID int64) (*domainvideo.Video, error)

	SchedulePublish(ctx context.Context, authorID int64, req servicecreator.SchedulePublishRequest) (*domaincreator.ScheduledTask, error)
	ListScheduledTasks(ctx context.Context, authorID int64, limit int) ([]*domaincreator.ScheduledTask, error)
	CancelScheduledTask(ctx context.Context, taskID string, authorID int64) error
}

// Handler handles Creator Center HTTP requests.
type Handler struct {
	svc Service
}

// NewHandler creates a new Creator Center Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// SaveDraftRequestPayload represents POST /api/v1/creator/drafts request payload.
type SaveDraftRequestPayload struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	PlayURL     string `json:"play_url"`
	CoverURL    string `json:"cover_url"`
	DurationMs  int    `json:"duration_ms"`
}

// UpdateDraftRequestPayload represents PUT /api/v1/creator/drafts/:id request payload.
type UpdateDraftRequestPayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	PlayURL     string `json:"play_url"`
	CoverURL    string `json:"cover_url"`
	DurationMs  int    `json:"duration_ms"`
}

// DraftResponse represents a single draft entity in HTTP responses.
type DraftResponse struct {
	ID          int64     `json:"id"`
	AuthorID    int64     `json:"author_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PlayURL     string    `json:"play_url"`
	CoverURL    string    `json:"cover_url"`
	DurationMs  int       `json:"duration_ms"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListDraftsResponse represents GET /api/v1/creator/drafts response payload.
type ListDraftsResponse struct {
	Items      []*DraftResponse `json:"items"`
	NextCursor string           `json:"next_cursor"`
	HasMore    bool             `json:"has_more"`
}

// SchedulePublishRequestPayload represents POST /api/v1/creator/publish-scheduled request payload.
type SchedulePublishRequestPayload struct {
	DraftID     *int64 `json:"draft_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PlayURL     string `json:"play_url"`
	CoverURL    string `json:"cover_url"`
	DurationMs  int    `json:"duration_ms"`
	ScheduledAt string `json:"scheduled_at" binding:"required"`
}

// ScheduledTaskResponse represents a single scheduled publish task in HTTP responses.
type ScheduledTaskResponse struct {
	ID          int64     `json:"id"`
	TaskID      string    `json:"task_id"`
	AuthorID    int64     `json:"author_id"`
	DraftID     *int64    `json:"draft_id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PlayURL     string    `json:"play_url"`
	CoverURL    string    `json:"cover_url"`
	DurationMs  int       `json:"duration_ms"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Status      string    `json:"status"`
	VideoID     *int64    `json:"video_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ListScheduledTasksResponse represents GET /api/v1/creator/scheduled-tasks response payload.
type ListScheduledTasksResponse struct {
	Items []*ScheduledTaskResponse `json:"items"`
}

func toDraftResponse(d *domaincreator.Draft) *DraftResponse {
	if d == nil {
		return nil
	}
	return &DraftResponse{
		ID:          d.ID,
		AuthorID:    d.AuthorID,
		Title:       d.Title,
		Description: d.Description,
		PlayURL:     d.PlayURL,
		CoverURL:    d.CoverURL,
		DurationMs:  d.DurationMs,
		Status:      d.StatusName(),
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

func toScheduledTaskResponse(t *domaincreator.ScheduledTask) *ScheduledTaskResponse {
	if t == nil {
		return nil
	}
	return &ScheduledTaskResponse{
		ID:          t.ID,
		TaskID:      t.TaskID,
		AuthorID:    t.AuthorID,
		DraftID:     t.DraftID,
		Title:       t.Title,
		Description: t.Description,
		PlayURL:     t.PlayURL,
		CoverURL:    t.CoverURL,
		DurationMs:  t.DurationMs,
		ScheduledAt: t.ScheduledAt,
		Status:      t.StatusName(),
		VideoID:     t.VideoID,
		CreatedAt:   t.CreatedAt,
	}
}

// SaveDraft handles POST /api/v1/creator/drafts.
func (h *Handler) SaveDraft(c *gin.Context) {
	authorID, ok := middleware.GetUserID(c)
	if !ok || authorID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	var req SaveDraftRequestPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam(err.Error()))
		return
	}

	draft, err := h.svc.SaveDraft(c.Request.Context(), authorID, servicecreator.SaveDraftRequest{
		Title:       req.Title,
		Description: req.Description,
		PlayURL:     req.PlayURL,
		CoverURL:    req.CoverURL,
		DurationMs:  req.DurationMs,
	})
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.Success(c, http.StatusCreated, toDraftResponse(draft))
}

// GetDraft handles GET /api/v1/creator/drafts/:id.
func (h *Handler) GetDraft(c *gin.Context) {
	authorID, ok := middleware.GetUserID(c)
	if !ok || authorID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	draftID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || draftID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid draft ID"))
		return
	}

	draft, err := h.svc.GetDraft(c.Request.Context(), draftID, authorID)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, toDraftResponse(draft))
}

// UpdateDraft handles PUT /api/v1/creator/drafts/:id.
func (h *Handler) UpdateDraft(c *gin.Context) {
	authorID, ok := middleware.GetUserID(c)
	if !ok || authorID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	draftID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || draftID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid draft ID"))
		return
	}

	var req UpdateDraftRequestPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam(err.Error()))
		return
	}

	draft, err := h.svc.UpdateDraft(c.Request.Context(), draftID, authorID, servicecreator.UpdateDraftRequest{
		Title:       req.Title,
		Description: req.Description,
		PlayURL:     req.PlayURL,
		CoverURL:    req.CoverURL,
		DurationMs:  req.DurationMs,
	})
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, gin.H{
		"id":         draft.ID,
		"updated_at": draft.UpdatedAt,
	})
}

// DeleteDraft handles DELETE /api/v1/creator/drafts/:id.
func (h *Handler) DeleteDraft(c *gin.Context) {
	authorID, ok := middleware.GetUserID(c)
	if !ok || authorID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	draftID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || draftID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid draft ID"))
		return
	}

	if err := h.svc.DeleteDraft(c.Request.Context(), draftID, authorID); err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, gin.H{"success": true})
}

// ListDrafts handles GET /api/v1/creator/drafts.
func (h *Handler) ListDrafts(c *gin.Context) {
	authorID, ok := middleware.GetUserID(c)
	if !ok || authorID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	cursor := c.Query("cursor")
	limit := 10
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	drafts, nextCursor, hasMore, err := h.svc.ListDrafts(c.Request.Context(), authorID, cursor, limit)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	items := make([]*DraftResponse, len(drafts))
	for i := range drafts {
		items[i] = toDraftResponse(drafts[i])
	}

	response.OK(c, ListDraftsResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// PublishDraft handles POST /api/v1/creator/drafts/:id/publish.
func (h *Handler) PublishDraft(c *gin.Context) {
	authorID, ok := middleware.GetUserID(c)
	if !ok || authorID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	draftID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || draftID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid draft ID"))
		return
	}

	video, err := h.svc.PublishDraft(c.Request.Context(), draftID, authorID)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, gin.H{
		"video_id":    video.ID,
		"publish_seq": video.PublishSeq,
		"title":       video.Title,
		"status":      "published",
	})
}

// SchedulePublish handles POST /api/v1/creator/publish-scheduled.
func (h *Handler) SchedulePublish(c *gin.Context) {
	authorID, ok := middleware.GetUserID(c)
	if !ok || authorID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	var req SchedulePublishRequestPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam(err.Error()))
		return
	}

	schedTime, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ScheduledAt))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid scheduled_at format, RFC3339 required"))
		return
	}

	task, err := h.svc.SchedulePublish(c.Request.Context(), authorID, servicecreator.SchedulePublishRequest{
		DraftID:     req.DraftID,
		Title:       req.Title,
		Description: req.Description,
		PlayURL:     req.PlayURL,
		CoverURL:    req.CoverURL,
		DurationMs:  req.DurationMs,
		ScheduledAt: schedTime,
	})
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.Success(c, http.StatusCreated, gin.H{
		"task_id":      task.TaskID,
		"scheduled_at": task.ScheduledAt.Format(time.RFC3339),
		"status":       task.StatusName(),
	})
}

// ListScheduledTasks handles GET /api/v1/creator/scheduled-tasks.
func (h *Handler) ListScheduledTasks(c *gin.Context) {
	authorID, ok := middleware.GetUserID(c)
	if !ok || authorID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	limit := 20
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}

	tasks, err := h.svc.ListScheduledTasks(c.Request.Context(), authorID, limit)
	if err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	items := make([]*ScheduledTaskResponse, len(tasks))
	for i := range tasks {
		items[i] = toScheduledTaskResponse(tasks[i])
	}

	response.OK(c, ListScheduledTasksResponse{Items: items})
}

// CancelScheduledTask handles DELETE /api/v1/creator/scheduled-tasks/:id.
func (h *Handler) CancelScheduledTask(c *gin.Context) {
	authorID, ok := middleware.GetUserID(c)
	if !ok || authorID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.Fail(c, response.ErrInvalidParam("Task ID is required"))
		return
	}

	if err := h.svc.CancelScheduledTask(c.Request.Context(), taskID, authorID); err != nil {
		response.Fail(c, h.mapError(err))
		return
	}

	response.OK(c, gin.H{"success": true})
}

func (h *Handler) mapError(err error) error {
	switch {
	case errors.Is(err, domaincreator.ErrDraftNotFound), errors.Is(err, domaincreator.ErrTaskNotFound):
		return response.ErrNotFound(err.Error())
	case errors.Is(err, domaincreator.ErrDraftForbidden), errors.Is(err, domaincreator.ErrTaskForbidden):
		return response.ErrForbidden(err.Error())
	case errors.Is(err, domaincreator.ErrEmptyDraftTitle),
		errors.Is(err, domaincreator.ErrEmptyDraftPlayURL),
		errors.Is(err, domaincreator.ErrDraftTitleTooLong),
		errors.Is(err, domaincreator.ErrDraftDescTooLong),
		errors.Is(err, domaincreator.ErrInvalidScheduledTime),
		errors.Is(err, domaincreator.ErrTaskNotScheduled),
		errors.Is(err, domaincreator.ErrDraftAlreadyPublished),
		errors.Is(err, domaincreator.ErrDraftAlreadyScheduled),
		errors.Is(err, domaincreator.ErrInvalidCursor):
		return response.ErrInvalidParam(err.Error())
	default:
		return response.ErrInternal(err.Error())
	}
}
