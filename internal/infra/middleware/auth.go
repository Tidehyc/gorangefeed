package middleware

import (
	"context"
	"strings"

	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

const (
	ContextKeyUserID  = "user_id"
	ContextKeyTokenID = "token_id"
)

// BlacklistChecker checks whether an access token is in the blacklist.
type BlacklistChecker func(ctx context.Context, tokenID string) (bool, error)

// ExtractToken retrieves the bearer token from the Authorization header or ?token= query parameter.
func ExtractToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	// Fallback to URL query parameter (?token=...), supporting browser EventSource (SSE)
	if tokenQuery := c.Query("token"); tokenQuery != "" {
		return strings.TrimSpace(tokenQuery)
	}

	return ""
}

// JWTAuth enforces authentication. Requests without a valid token are rejected with 401 Unauthorized.
func JWTAuth(jwtMgr *jwt.TokenManager, isBlacklisted BlacklistChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ExtractToken(c)
		if tokenStr == "" {
			response.Fail(c, response.ErrUnauthorized("Missing authentication token"))
			c.Abort()
			return
		}

		claims, err := jwtMgr.VerifyAccessToken(tokenStr)
		if err != nil {
			response.Fail(c, response.ErrUnauthorized("Invalid or expired authentication token"))
			c.Abort()
			return
		}

		if isBlacklisted != nil {
			blacklisted, err := isBlacklisted(c.Request.Context(), claims.TokenID)
			if err == nil && blacklisted {
				response.Fail(c, response.ErrUnauthorized("Token has been revoked"))
				c.Abort()
				return
			}
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyTokenID, claims.TokenID)
		c.Next()
	}
}

// SoftJWTAuth provides optional authentication.
// If a valid token is provided, userID is injected into context.
// If no token or an invalid token is provided, request continues anonymously as a guest.
func SoftJWTAuth(jwtMgr *jwt.TokenManager, isBlacklisted BlacklistChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ExtractToken(c)
		if tokenStr == "" {
			c.Next()
			return
		}

		claims, err := jwtMgr.VerifyAccessToken(tokenStr)
		if err == nil && claims != nil {
			if isBlacklisted != nil {
				blacklisted, checkErr := isBlacklisted(c.Request.Context(), claims.TokenID)
				if checkErr == nil && blacklisted {
					c.Next()
					return
				}
			}
			c.Set(ContextKeyUserID, claims.UserID)
			c.Set(ContextKeyTokenID, claims.TokenID)
		}

		c.Next()
	}
}

// GetUserID retrieves the authenticated user ID from gin context if present.
func GetUserID(c *gin.Context) (int64, bool) {
	val, exists := c.Get(ContextKeyUserID)
	if !exists {
		return 0, false
	}
	uid, ok := val.(int64)
	return uid, ok
}

// MustGetUserID returns the authenticated user ID or panics if not present (to be used after JWTAuth).
func MustGetUserID(c *gin.Context) int64 {
	uid, ok := GetUserID(c)
	if !ok {
		panic("MustGetUserID called on unauthenticated request")
	}
	return uid
}
