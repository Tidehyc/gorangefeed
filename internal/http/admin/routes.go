package admin

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers administrative review and moderation routes under /api/v1/admin.
func RegisterRoutes(r *gin.Engine, h *Handler, jwtMgr *jwt.TokenManager, tokenStore domainaccount.TokenStore) {
	v1 := r.Group("/api/v1/admin")

	var isBlacklisted middleware.BlacklistChecker
	if tokenStore != nil {
		isBlacklisted = tokenStore.IsAccessTokenBlacklisted
	}
	adminAuth := middleware.JWTAuth(jwtMgr, isBlacklisted)

	v1.GET("/reviews", adminAuth, h.ListReviews)
	v1.POST("/videos/:id/review", adminAuth, h.ReviewVideo)
	v1.POST("/videos/:id/takedown", adminAuth, h.TakedownVideo)
	v1.POST("/users/:id/ban", adminAuth, h.BanUser)
}
