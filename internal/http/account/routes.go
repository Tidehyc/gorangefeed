package account

import (
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers all account domain routes and associates security middlewares.
func RegisterRoutes(r *gin.Engine, h *Handler, jwtMgr *jwt.TokenManager, tokenStore domainaccount.TokenStore, rdb *redis.Client) {
	v1 := r.Group("/api/v1")

	// 1. Auth routes with IP rate limiting (10 requests per minute per IP per DESIGN.md)
	authGroup := v1.Group("/auth")
	if rdb != nil {
		authGroup.Use(middleware.IPRateLimiter(rdb, "auth", 10, time.Minute))
	}
	{
		authGroup.POST("/register", h.Register)
		authGroup.POST("/login", h.Login)
		authGroup.POST("/refresh", h.RefreshToken)
		authGroup.POST("/logout", h.Logout)
	}

	// 2. User routes
	userGroup := v1.Group("/users")
	{
		// /users/me and /users/avatar require mandatory JWTAuth and token revocation check
		authRequired := userGroup.Group("")
		authRequired.Use(middleware.JWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted))
		{
			authRequired.GET("/me", h.GetMe)
			authRequired.PUT("/me", h.UpdateMe)
			authRequired.POST("/avatar", h.UploadAvatar)
		}

		// Public user profile with soft auth (allows guests, extracts identity if valid token is provided)
		userGroup.GET("/:userId", middleware.SoftJWTAuth(jwtMgr, tokenStore.IsAccessTokenBlacklisted), h.GetPublicProfile)
	}
}
