package asset

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers all user asset routes (favorites and history) under /api/v1.
func RegisterRoutes(
	r *gin.Engine,
	h *Handler,
	jwtMgr *jwt.TokenManager,
	tokenStore domainaccount.TokenStore,
	rdb *redis.Client,
) {
	jwtAuth := middleware.JWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted)
	softAuth := middleware.SoftJWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted)

	v1 := r.Group("/api/v1")

	// Video favorite & playback breakpoint endpoints
	videos := v1.Group("/videos")
	{
		videos.POST("/:id/favorite", jwtAuth, h.FavoriteVideo)
		videos.DELETE("/:id/favorite", jwtAuth, h.UnfavoriteVideo)
		videos.GET("/:id/history", jwtAuth, h.GetWatchProgress)
		videos.POST("/:id/history", jwtAuth, h.RecordWatchProgress)
	}

	// User asset center: favorite folders & playback history
	user := v1.Group("/user")
	{
		// Favorite folders management
		user.GET("/favorites", jwtAuth, h.ListFolders)
		user.POST("/favorites", jwtAuth, h.CreateFolder)
		user.DELETE("/favorites/:id", jwtAuth, h.DeleteFolder)
		user.GET("/favorites/:id/items", softAuth, h.ListFolderItems)

		// Watch history management
		user.GET("/history", jwtAuth, h.ListHistory)
		user.POST("/history", jwtAuth, h.RecordHistory)
		user.DELETE("/history", jwtAuth, h.ClearHistory)
	}
}
