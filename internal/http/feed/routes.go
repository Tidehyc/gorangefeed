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

	jwtAuth := middleware.JWTAuth(jwtMgr, tokenStore)
	softAuth := middleware.SoftJWTAuth(jwtMgr, tokenStore)

	// Feed endpoints (global, following, like-rank, hot, recommend)
	feedGroup := v1.Group("/feed")
	{
		feedGroup.GET("/global", softAuth, h.GetGlobalFeed)
		feedGroup.GET("/following", jwtAuth, h.GetFollowingFeed)
		feedGroup.GET("/like-rank", softAuth, h.GetLikeRankFeed)
		feedGroup.GET("/hot", softAuth, h.GetHotFeed)
		feedGroup.GET("/recommend", softAuth, h.GetRecommendFeed)
	}


	// Playback view reporting endpoint (FR-VID-08)
	videoGroup := v1.Group("/videos")
	{
		videoGroup.POST("/:videoId/view", softAuth, h.RecordView)
	}

	// Tag timeline endpoint: the topic page a feed card links into.
	tagGroup := v1.Group("/tags")
	{
		tagGroup.GET("/:tag/videos", softAuth, h.GetTagFeed)
	}
}
