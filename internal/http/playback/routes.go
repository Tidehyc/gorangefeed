package playback

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers playback configuration and QoS telemetry routes.
func RegisterRoutes(
	router *gin.Engine,
	handler *Handler,
	jwtMgr *jwt.TokenManager,
	tokenStore domainaccount.TokenStore,
	rdb *redis.Client,
) {
	softAuth := middleware.SoftJWTAuth(jwtMgr, tokenStore)

	playback := router.Group("/api/v1/playback")
	playback.Use(softAuth)
	{
		playback.GET("/config", handler.GetConfig)
		playback.POST("/events", handler.ReportEvents)
	}
}
