package interaction

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers all interaction domain routes under /api/v1.
func RegisterRoutes(r *gin.Engine, h *Handler, jwtMgr *jwt.TokenManager, tokenStore domainaccount.TokenStore, rdb *redis.Client) {
	v1 := r.Group("/api/v1")

	jwtAuth := middleware.JWTAuth(jwtMgr, tokenStore)
	softAuth := middleware.SoftJWTAuth(jwtMgr, tokenStore)

	// Video interaction endpoints
	videoGroup := v1.Group("/videos")
	{
		videoGroup.POST("/:videoId/like", jwtAuth, h.LikeVideo)
		videoGroup.DELETE("/:videoId/like", jwtAuth, h.UnlikeVideo)
		videoGroup.POST("/:videoId/comments", jwtAuth, h.CreateComment)
		videoGroup.GET("/:videoId/comments", softAuth, h.ListComments)
	}

	// Comment interaction endpoints
	commentGroup := v1.Group("/comments")
	{
		commentGroup.DELETE("/:commentId", jwtAuth, h.DeleteComment)
		commentGroup.POST("/:commentId/like", jwtAuth, h.LikeComment)
		commentGroup.DELETE("/:commentId/like", jwtAuth, h.UnlikeComment)
	}

	// User liked videos endpoint
	userGroup := v1.Group("/users")
	{
		authRequired := userGroup.Group("")
		authRequired.Use(jwtAuth)
		{
			authRequired.GET("/me/likes", h.ListMyLikedVideos)
		}
	}
}
