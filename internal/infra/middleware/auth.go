package middleware

import (
	"context"
	"strings"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

const (
	ContextKeyUserID  = "user_id"
	ContextKeyTokenID = "token_id"
)

// AuthStateChecker reports whether a credential has been revoked and whether its owner is
// currently banned. Implemented by the Redis-backed account.TokenStore.
type AuthStateChecker interface {
	IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error)
	IsUserBanned(ctx context.Context, userID int64) (bool, error)
}

// RoleResolver loads a user record so RequireRole can enforce a permission scope.
type RoleResolver interface {
	FindByID(ctx context.Context, id int64) (*domainaccount.User, error)
}

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

// isBanned reports whether the account behind a validated token has been banned.
// A lookup failure is treated as "not banned" so a Redis hiccup degrades to token-scoped
// security instead of locking every user out; the ban still blocks refresh and re-login.
func isBanned(c *gin.Context, state AuthStateChecker, userID int64) bool {
	if state == nil || userID <= 0 {
		return false
	}
	banned, err := state.IsUserBanned(c.Request.Context(), userID)
	return err == nil && banned
}

// isBlacklisted reports whether the presented token has been revoked.
func isBlacklisted(c *gin.Context, state AuthStateChecker, tokenID string) bool {
	if state == nil {
		return false
	}
	revoked, err := state.IsAccessTokenBlacklisted(c.Request.Context(), tokenID)
	return err == nil && revoked
}

// JWTAuth enforces authentication. Requests without a valid token are rejected with 401 Unauthorized.
func JWTAuth(jwtMgr *jwt.TokenManager, state AuthStateChecker) gin.HandlerFunc {
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

		if isBlacklisted(c, state, claims.TokenID) {
			response.Fail(c, response.ErrUnauthorized("Token has been revoked"))
			c.Abort()
			return
		}

		if isBanned(c, state, claims.UserID) {
			response.Fail(c, response.ErrForbidden("Account has been suspended"))
			c.Abort()
			return
		}

		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyTokenID, claims.TokenID)
		c.Next()
	}
}

// SoftJWTAuth provides optional authentication.
// If a valid token is provided, userID is injected into context.
// If no token or an invalid token is provided, request continues anonymously as a guest.
func SoftJWTAuth(jwtMgr *jwt.TokenManager, state AuthStateChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ExtractToken(c)
		if tokenStr == "" {
			c.Next()
			return
		}

		claims, err := jwtMgr.VerifyAccessToken(tokenStr)
		if err == nil && claims != nil {
			// A revoked token or a banned owner degrades to guest rather than failing the
			// request, because these routes are also readable anonymously.
			if !isBlacklisted(c, state, claims.TokenID) && !isBanned(c, state, claims.UserID) {
				c.Set(ContextKeyUserID, claims.UserID)
				c.Set(ContextKeyTokenID, claims.TokenID)
			}
		}

		c.Next()
	}
}

// RequireRole rejects authenticated requests whose owner lacks the required role.
// It must run after JWTAuth.
func RequireRole(resolver RoleResolver, required domainaccount.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := GetUserID(c)
		if !ok || userID <= 0 {
			response.Fail(c, response.ErrUnauthorized("Authentication required"))
			c.Abort()
			return
		}

		user, err := resolver.FindByID(c.Request.Context(), userID)
		if err != nil {
			// Unknown or deleted account: do not leak the difference between the two cases.
			response.Fail(c, response.ErrForbidden("Insufficient permissions"))
			c.Abort()
			return
		}

		if user.Role != required {
			response.Fail(c, response.ErrForbidden("Insufficient permissions"))
			c.Abort()
			return
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
