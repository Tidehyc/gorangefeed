package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupAuthTest() (*jwt.TokenManager, *gin.Engine) {
	cfg := config.JWTConfig{
		AccessSecret: "test_secret_for_middleware_123456",
		AccessTTL:    1 * time.Hour,
	}
	jwtMgr := jwt.NewTokenManager(cfg)
	r := gin.New()
	return jwtMgr, r
}

func TestJWTAuth_HeaderAndQuery(t *testing.T) {
	jwtMgr, r := setupAuthTest()

	isBlacklisted := func(ctx context.Context, tokenID string) (bool, error) {
		return tokenID == "revoked-token", nil
	}

	r.GET("/protected", JWTAuth(jwtMgr, isBlacklisted), func(c *gin.Context) {
		uid := MustGetUserID(c)
		c.JSON(http.StatusOK, gin.H{"user_id": uid})
	})

	validToken, _, _ := jwtMgr.GenerateAccessToken(1001, "valid-token")
	revokedToken, _, _ := jwtMgr.GenerateAccessToken(1002, "revoked-token")

	// 1. Valid token in Header
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Valid token in Query ?token=...
	req, _ = http.NewRequest(http.MethodGet, "/protected?token="+validToken, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with query token, got %d: %s", w.Code, w.Body.String())
	}

	// 3. Missing token
	req, _ = http.NewRequest(http.MethodGet, "/protected", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d", w.Code)
	}

	// 4. Invalid token string
	req, _ = http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-junk")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", w.Code)
	}

	// 5. Revoked (blacklisted) token
	req, _ = http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+revokedToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked token, got %d", w.Code)
	}
}

func TestSoftJWTAuth(t *testing.T) {
	jwtMgr, r := setupAuthTest()

	r.GET("/soft", SoftJWTAuth(jwtMgr, nil), func(c *gin.Context) {
		uid, ok := GetUserID(c)
		c.JSON(http.StatusOK, gin.H{"has_user": ok, "user_id": uid})
	})

	validToken, _, _ := jwtMgr.GenerateAccessToken(2002, "valid-token")

	// 1. With valid token -> has_user = true
	req, _ := http.NewRequest(http.MethodGet, "/soft", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// 2. Without token -> has_user = false, but still 200 OK
	req, _ = http.NewRequest(http.MethodGet, "/soft", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for anonymous, got %d", w.Code)
	}

	// 3. With invalid token -> has_user = false, but still 200 OK
	req, _ = http.NewRequest(http.MethodGet, "/soft", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for bad token in soft auth, got %d", w.Code)
	}
}
