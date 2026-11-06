package admin

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers administrative review and moderation routes under /api/v1/admin.
// Every route is gated by JWTAuth plus RequireRole(admin): moderation endpoints must never be
// reachable by ordinary authenticated users.
func RegisterRoutes(
	r *gin.Engine,
	h *Handler,
	jwtMgr *jwt.TokenManager,
	tokenStore domainaccount.TokenStore,
	users middleware.RoleResolver,
) {
	v1 := r.Group("/api/v1/admin")
	v1.Use(
		middleware.JWTAuth(jwtMgr, tokenStore),
		middleware.RequireRole(users, domainaccount.RoleAdmin),
	)

	v1.GET("/reviews", h.ListReviews)
	v1.POST("/videos/:id/review", h.ReviewVideo)
	v1.POST("/videos/:id/takedown", h.TakedownVideo)
	v1.POST("/users/:id/ban", h.BanUser)
	v1.POST("/users/:id/unban", h.UnbanUser)
}
