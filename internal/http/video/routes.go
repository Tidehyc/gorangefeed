package video

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers all video domain endpoints under /api/v1 with corresponding security middlewares.
func RegisterRoutes(r *gin.Engine, h *Handler, jwtMgr *jwt.TokenManager, tokenStore domainaccount.TokenStore, rdb *redis.Client) {
	v1 := r.Group("/api/v1")

	videoGroup := v1.Group("/videos")
	{
		// Authenticated operations: upload media, upload cover, publish video, take down video
		authRequired := videoGroup.Group("")
		authRequired.Use(middleware.JWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted))
		{
			authRequired.POST("/upload", h.UploadVideo)
			authRequired.POST("/cover", h.UploadCover)
			authRequired.POST("", h.Publish)
			authRequired.DELETE("/:videoId", h.DeleteVideo)
		}

		// Public video detail with SoftJWTAuth (extracts viewer context if token is present)
		videoGroup.GET("/:videoId", middleware.SoftJWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted), h.GetDetail)
	}

	// Author public videos query with SoftJWTAuth
	userGroup := v1.Group("/users")
	{
		userGroup.GET("/:userId/videos", middleware.SoftJWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted), h.ListAuthorVideos)
	}
}
