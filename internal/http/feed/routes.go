package feed

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers all feed domain endpoints under /api/v1 with corresponding security middlewares.
func RegisterRoutes(r *gin.Engine, h *Handler, jwtMgr *jwt.TokenManager, tokenStore domainaccount.TokenStore, rdb *redis.Client) {
	v1 := r.Group("/api/v1")

	softAuth := middleware.SoftJWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted)

	// Global feed endpoint with SoftJWTAuth (supports anonymous visitors and logged-in viewers)
	feedGroup := v1.Group("/feed")
	{
		feedGroup.GET("/global", softAuth, h.GetGlobalFeed)
	}

	// Playback view reporting endpoint (FR-VID-08)
	videoGroup := v1.Group("/videos")
	{
		videoGroup.POST("/:videoId/view", softAuth, h.RecordView)
	}
}
