package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// mockAuthState is an in-memory AuthStateChecker used by the middleware tests.
type mockAuthState struct {
	blacklisted map[string]bool
	banned      map[int64]bool
	invalidated map[int64]time.Time
}

func (m *mockAuthState) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	return m.blacklisted[tokenID], nil
}

func (m *mockAuthState) IsUserBanned(ctx context.Context, userID int64) (bool, error) {
	return m.banned[userID], nil
}

func (m *mockAuthState) IsUserTokenInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error) {
	cutoff, ok := m.invalidated[userID]
	if !ok {
		return false, nil
	}
	return issuedAt.Unix() < cutoff.Unix(), nil
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

	state := &mockAuthState{
		blacklisted: map[string]bool{"revoked-token": true},
		banned:      map[int64]bool{},
	}

	r.GET("/protected", JWTAuth(jwtMgr, state), func(c *gin.Context) {
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

func TestJWTAuth_BannedUserRejected(t *testing.T) {
	jwtMgr, r := setupAuthTest()

	state := &mockAuthState{
		blacklisted: map[string]bool{},
		banned:      map[int64]bool{3003: true},
	}

	r.GET("/protected", JWTAuth(jwtMgr, state), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": MustGetUserID(c)})
	})

	// The token itself is valid and not blacklisted: only the account-level ban can stop it.
	token, _, _ := jwtMgr.GenerateAccessToken(3003, "still-valid-jti")

	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a banned account, got %d: %s", w.Code, w.Body.String())
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

func TestSoftJWTAuth_BannedUserTreatedAsGuest(t *testing.T) {
	jwtMgr, r := setupAuthTest()

	state := &mockAuthState{
		blacklisted: map[string]bool{},
		banned:      map[int64]bool{4004: true},
	}

	r.GET("/soft", SoftJWTAuth(jwtMgr, state), func(c *gin.Context) {
		uid, ok := GetUserID(c)
		c.JSON(http.StatusOK, gin.H{"has_user": ok, "user_id": uid})
	})

	token, _, _ := jwtMgr.GenerateAccessToken(4004, "banned-jti")

	req, _ := http.NewRequest(http.MethodGet, "/soft", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for banned account on a soft-auth route, got %d", w.Code)
	}
	if body := w.Body.String(); body != `{"has_user":false,"user_id":0}` {
		t.Fatalf("expected banned account to degrade to guest, got %s", body)
	}
}

// mockRoleResolver resolves roles for RequireRole tests.
type mockRoleResolver struct {
	users map[int64]*domainaccount.User
}

func (m *mockRoleResolver) FindByID(ctx context.Context, id int64) (*domainaccount.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, domainaccount.ErrUserNotFound
	}
	return u, nil
}

func TestRequireRole(t *testing.T) {
	jwtMgr, r := setupAuthTest()

	resolver := &mockRoleResolver{users: map[int64]*domainaccount.User{
		5005: {ID: 5005, Role: domainaccount.RoleAdmin},
		5006: {ID: 5006, Role: domainaccount.RoleUser},
	}}

	r.GET("/admin", JWTAuth(jwtMgr, nil), RequireRole(resolver, domainaccount.RoleAdmin), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	adminToken, _, _ := jwtMgr.GenerateAccessToken(5005, "admin-jti")
	userToken, _, _ := jwtMgr.GenerateAccessToken(5006, "user-jti")
	unknownToken, _, _ := jwtMgr.GenerateAccessToken(5007, "unknown-jti")

	req, _ := http.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin, got %d: %s", w.Code, w.Body.String())
	}

	req, _ = http.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for regular user, got %d", w.Code)
	}

	req, _ = http.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+unknownToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when the role cannot be resolved, got %d", w.Code)
	}
}

func TestJWTAuth_RejectsTokensIssuedBeforeCredentialChange(t *testing.T) {
	jwtMgr, r := setupAuthTest()

	// A cutoff in the future invalidates every token this account already holds.
	state := &mockAuthState{
		blacklisted: map[string]bool{},
		banned:      map[int64]bool{},
		invalidated: map[int64]time.Time{2001: time.Now().Add(time.Minute)},
	}

	r.GET("/protected", JWTAuth(jwtMgr, state), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": MustGetUserID(c)})
	})
	r.GET("/optional", SoftJWTAuth(jwtMgr, state), func(c *gin.Context) {
		uid, ok := GetUserID(c)
		c.JSON(http.StatusOK, gin.H{"authenticated": ok, "user_id": uid})
	})

	staleToken, _, _ := jwtMgr.GenerateAccessToken(2001, "stale-token")
	freshUserToken, _, _ := jwtMgr.GenerateAccessToken(2002, "other-user-token")

	// 1. Hard auth: a retired credential is rejected outright.
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+staleToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a retired credential, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Soft auth: the same token degrades to guest instead of failing the request.
	req, _ = http.NewRequest(http.MethodGet, "/optional", nil)
	req.Header.Set("Authorization", "Bearer "+staleToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for optional auth, got %d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"authenticated":false`) {
		t.Fatalf("expected the request to fall back to guest, got %s", body)
	}

	// 3. Another account's token is unaffected by the cutoff.
	req, _ = http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+freshUserToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected unaffected user to pass, got %d: %s", w.Code, w.Body.String())
	}
}
