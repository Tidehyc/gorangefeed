package upload

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers chunked upload endpoints under /api/v1/videos/chunk with JWT authentication.
func RegisterRoutes(r *gin.Engine, h *Handler, jwtMgr *jwt.TokenManager, tokenStore domainaccount.TokenStore, rdb *redis.Client) {
	v1 := r.Group("/api/v1")
	chunkGroup := v1.Group("/videos/chunk")
	chunkGroup.Use(middleware.JWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted))
	{
		chunkGroup.POST("/init", h.InitSession)
		chunkGroup.POST("/upload", h.UploadChunk)
		chunkGroup.GET("/status", h.GetStatus)
		chunkGroup.POST("/complete", h.Complete)
	}
}
