package interaction

import (
	"time"

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

	// Write paths are limited per UID at 30 requests per minute (NFR-SEC-07). One bucket per
	// action rather than one shared bucket: a user who comments a lot must not be locked out of
	// liking, and vice versa. The limiters sit after JWTAuth so the key is the user ID.
	likeLimit := middleware.UserRateLimiter(rdb, "like", 30, time.Minute)
	commentLimit := middleware.UserRateLimiter(rdb, "comment", 30, time.Minute)

	// Video interaction endpoints
	videoGroup := v1.Group("/videos")
	{
		videoGroup.POST("/:videoId/like", jwtAuth, likeLimit, h.LikeVideo)
		videoGroup.DELETE("/:videoId/like", jwtAuth, likeLimit, h.UnlikeVideo)
		videoGroup.POST("/:videoId/comments", jwtAuth, commentLimit, h.CreateComment)
		videoGroup.GET("/:videoId/comments", softAuth, h.ListComments)
	}

	// Comment interaction endpoints
	commentGroup := v1.Group("/comments")
	{
		commentGroup.DELETE("/:commentId", jwtAuth, commentLimit, h.DeleteComment)
		commentGroup.POST("/:commentId/like", jwtAuth, likeLimit, h.LikeComment)
		commentGroup.DELETE("/:commentId/like", jwtAuth, likeLimit, h.UnlikeComment)
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
