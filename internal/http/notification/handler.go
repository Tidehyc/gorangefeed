package notification

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	servicenotification "github.com/Tidehyc/gorangefeed/internal/service/notification"
	"github.com/gin-gonic/gin"
)

// Service defines application contracts for notification HTTP handling.
type Service interface {
	ListNotifications(ctx context.Context, recipientID int64, filterType string, cursorStr string, limit int) ([]*domainnotification.Notification, string, bool, error)
	GetUnreadCount(ctx context.Context, recipientID int64) (int64, error)
	MarkAsRead(ctx context.Context, recipientID, notificationID int64) error
	MarkAllAsRead(ctx context.Context, recipientID int64) (int64, error)
}

// Hub defines SSE Hub contracts required by the HTTP handler.
type Hub interface {
	CreateTicket(ctx context.Context, userID int64) (string, int, error)
	ConsumeTicket(ctx context.Context, ticket string) (int64, error)
	Register(userID int64) chan *servicenotification.SSEMessage
	Unregister(userID int64, ch chan *servicenotification.SSEMessage)
	GetCatchUpNotifications(ctx context.Context, recipientID int64, lastEventID string) ([]*domainnotification.Notification, error)
}

// Handler handles notification REST endpoints and SSE stream handshakes.
type Handler struct {
	svc Service
	hub Hub
}

// NewHandler creates a new notification Handler instance.
func NewHandler(svc Service, hub Hub) *Handler {
	return &Handler{
		svc: svc,
		hub: hub,
	}
}

// ListNotificationsResponse represents GET /api/v1/notifications response payload.
type ListNotificationsResponse struct {
	Items      []*domainnotification.Notification `json:"items"`
	NextCursor string                             `json:"next_cursor"`
	HasMore    bool                               `json:"has_more"`
}

// UnreadCountResponse represents GET /api/v1/notifications/unread-count response payload.
type UnreadCountResponse struct {
	UnreadCount int64 `json:"unread_count"`
}

// TicketResponse represents POST /api/v1/notifications/sse/ticket response payload.
type TicketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expires_in"`
}

// ListNotifications handles GET /api/v1/notifications.
func (h *Handler) ListNotifications(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	filterType := c.Query("type")
	cursor := c.Query("cursor")
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)

	items, nextCursor, hasMore, err := h.svc.ListNotifications(c.Request.Context(), userID, filterType, cursor, limit)
	if err != nil {
		response.Fail(c, mapNotificationError(err))
		return
	}

	response.OK(c, ListNotificationsResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// GetUnreadCount handles GET /api/v1/notifications/unread-count.
func (h *Handler) GetUnreadCount(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	count, err := h.svc.GetUnreadCount(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, mapNotificationError(err))
		return
	}

	response.OK(c, UnreadCountResponse{
		UnreadCount: count,
	})
}

// MarkAsRead handles PATCH /api/v1/notifications/:id/read.
func (h *Handler) MarkAsRead(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	idStr := c.Param("id")
	notifID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || notifID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid notification ID"))
		return
	}

	if err := h.svc.MarkAsRead(c.Request.Context(), userID, notifID); err != nil {
		response.Fail(c, mapNotificationError(err))
		return
	}

	response.OK(c, gin.H{
		"success": true,
		"is_read": true,
	})
}

// MarkAllAsRead handles POST /api/v1/notifications/read-all.
func (h *Handler) MarkAllAsRead(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	markedCount, err := h.svc.MarkAllAsRead(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, mapNotificationError(err))
		return
	}

	response.OK(c, gin.H{
		"marked_count": markedCount,
	})
}

// CreateTicket handles POST /api/v1/notifications/sse/ticket.
func (h *Handler) CreateTicket(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	ticket, expiresIn, err := h.hub.CreateTicket(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, mapNotificationError(err))
		return
	}

	response.OK(c, TicketResponse{
		Ticket:    ticket,
		ExpiresIn: expiresIn,
	})
}

// Stream handles GET /api/v1/notifications/stream (Server-Sent Events long-lived stream).
func (h *Handler) Stream(c *gin.Context) {
	var targetUID int64

	// 1. Authenticate either via single-use Ticket or Header JWT
	ticket := c.Query("ticket")
	if ticket != "" {
		uid, err := h.hub.ConsumeTicket(c.Request.Context(), ticket)
		if err != nil {
			response.Fail(c, response.ErrUnauthorized("Invalid or expired ticket"))
			return
		}
		targetUID = uid
	} else {
		uid, ok := middleware.GetUserID(c)
		if !ok || uid <= 0 {
			response.Fail(c, response.ErrUnauthorized("Authentication required (ticket query or Bearer token)"))
			return
		}
		targetUID = uid
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Fail(c, response.ErrInternal("Streaming unsupported by connection"))
		return
	}

	// 2. Set RFC text/event-stream headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	// Send reconnect retry advice (3 seconds)
	_, _ = c.Writer.Write([]byte("retry: 3000\n\n"))
	flusher.Flush()

	// 3. 60s overlapping catch-up for reconnecting clients
	lastEventID := c.GetHeader("Last-Event-ID")
	if lastEventID == "" {
		lastEventID = c.Query("last_event_id")
	}
	if lastEventID != "" {
		missed, err := h.hub.GetCatchUpNotifications(c.Request.Context(), targetUID, lastEventID)
		if err == nil {
			for _, notif := range missed {
				msg, msgErr := servicenotification.NewNotificationSSEMessage(notif)
				if msgErr == nil {
					_, _ = c.Writer.Write(msg.Format())
					flusher.Flush()
				}
			}
		}
	}

	// 4. Register client to hub channel
	ch := h.hub.Register(targetUID)
	defer h.hub.Unregister(targetUID, ch)

	ticker := time.NewTicker(servicenotification.KeepaliveInterval)
	defer ticker.Stop()

	// 5. Active SSE event delivery and keepalive loop
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			_, _ = c.Writer.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				// Channel closed by Hub (e.g. server restarting or drain)
				return
			}
			_, _ = c.Writer.Write(msg.Format())
			flusher.Flush()
		}
	}
}

func mapNotificationError(err error) error {
	var appErr *response.AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	switch {
	case errors.Is(err, domainnotification.ErrInvalidRecipientID),
		errors.Is(err, domainnotification.ErrInvalidNotificationID),
		errors.Is(err, domainnotification.ErrInvalidType),
		errors.Is(err, domainnotification.ErrInvalidCursor),
		errors.Is(err, domainnotification.ErrInvalidEventID),
		errors.Is(err, domainnotification.ErrEmptyTitle),
		errors.Is(err, domainnotification.ErrTitleTooLong),
		errors.Is(err, domainnotification.ErrContentTooLong),
		errors.Is(err, domainnotification.ErrSelfNotification):
		return response.ErrInvalidParam(err.Error())
	case errors.Is(err, domainnotification.ErrNotificationNotFound):
		return response.ErrNotFound(err.Error())
	case errors.Is(err, domainnotification.ErrTicketExpired):
		return response.ErrUnauthorized(err.Error())
	default:
		return response.ErrInternal(err.Error())
	}
}
