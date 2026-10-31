package relation

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers all user relation domain routes under /api/v1.
func RegisterRoutes(r *gin.Engine, h *Handler, jwtMgr *jwt.TokenManager, tokenStore domainaccount.TokenStore, rdb *redis.Client) {
	v1 := r.Group("/api/v1")

	jwtAuth := middleware.JWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted)
	softAuth := middleware.SoftJWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted)

	userGroup := v1.Group("/users")
	{
		userGroup.POST("/:userId/follow", jwtAuth, h.Follow)
		userGroup.DELETE("/:userId/follow", jwtAuth, h.Unfollow)
		userGroup.GET("/:userId/followings", softAuth, h.ListFollowings)
		userGroup.GET("/:userId/followers", softAuth, h.ListFollowers)
		userGroup.GET("/:userId/relation", jwtAuth, h.GetRelation)
	}
}
