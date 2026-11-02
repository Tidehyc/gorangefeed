package notification

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers all notification domain endpoints under /api/v1/notifications.
func RegisterRoutes(r *gin.Engine, h *Handler, jwtMgr *jwt.TokenManager, tokenStore domainaccount.TokenStore, rdb *redis.Client) {
	v1 := r.Group("/api/v1/notifications")

	jwtAuth := middleware.JWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted)
	softAuth := middleware.SoftJWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted)

	// RESTful notification querying and mark-as-read endpoints
	v1.GET("", jwtAuth, h.ListNotifications)
	v1.GET("/unread-count", jwtAuth, h.GetUnreadCount)
	v1.PATCH("/:id/read", jwtAuth, h.MarkAsRead)
	v1.POST("/read-all", jwtAuth, h.MarkAllAsRead)

	// SSE Ticket exchange endpoint (requires JWT)
	v1.POST("/sse/ticket", jwtAuth, h.CreateTicket)

	// SSE stream endpoint (supports ?ticket=uuid or Authorization: Bearer ...)
	v1.GET("/stream", softAuth, h.Stream)
}
