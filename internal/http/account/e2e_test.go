package account

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/migration"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceaccount "github.com/Tidehyc/gorangefeed/internal/infra/persistence/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/redis"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/Tidehyc/gorangefeed/internal/infra/router"
	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
	serviceaccount "github.com/Tidehyc/gorangefeed/internal/service/account"
	"github.com/gin-gonic/gin"
)

// setupE2ETest initializes a live test router with actual MySQL and Redis connections.
func setupE2ETest(t *testing.T) (*gin.Engine, string, func()) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		for _, candidate := range []string{
			"../../../configs/config.yaml",
			"../../configs/config.yaml",
			"configs/config.yaml",
			"../configs/config.yaml",
		} {
			if _, err := os.Stat(candidate); err == nil {
				configPath = candidate
				break
			}
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Skipf("skipping E2E test: failed to load config (%v)", err)
		return nil, "", nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping E2E test: MySQL not reachable on %s:%d (%v)", cfg.MySQL.Host, cfg.MySQL.Port, err)
		return nil, "", nil
	}

	// Ensure database schema is migrated for E2E tests
	migrationsDir := "../../../migrations"
	if _, err := os.Stat(migrationsDir); err != nil {
		for _, cand := range []string{"../../migrations", "migrations", "../migrations"} {
			if _, err := os.Stat(cand); err == nil {
				migrationsDir = cand
				break
			}
		}
	}
	if _, _, err := migration.RunUp(cfg.MySQL.DSN(), migrationsDir); err != nil {
		t.Fatalf("failed to run migrations for E2E test: %v", err)
	}

	rdb, err := redis.New(&cfg.Redis)
	if err != nil {
		t.Skipf("skipping E2E test: Redis not reachable on %s (%v)", cfg.Redis.Addr(), err)
		return nil, "", nil
	}

	tempDir, err := os.MkdirTemp("", "gorangefeed_e2e_storage_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	avatarStorage, err := storage.NewAvatarStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create avatar storage: %v", err)
	}

	repo := persistenceaccount.NewRepository(db)
	tokenStore := persistenceaccount.NewTokenStore(rdb)
	jwtMgr := jwt.NewTokenManager(cfg.JWT)
	svc := serviceaccount.NewService(repo, tokenStore, jwtMgr, cfg.JWT)
	handler := NewHandler(svc, jwtMgr, avatarStorage)

	gin.SetMode(gin.TestMode)
	engine := router.NewRouter(router.DependencyChecks{
		CheckMySQL: func(ctx context.Context) error { return mysql.Ping(ctx, db) },
		CheckRedis: func(ctx context.Context) error { return redis.Ping(ctx, rdb) },
	})

	router.MountMediaRoutes(engine, avatarStorage.BaseDir())
	RegisterRoutes(engine, handler, jwtMgr, tokenStore, rdb)

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
		_ = rdb.Close()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	return engine, tempDir, cleanup
}

func parseJSONResponse(t *testing.T, w *httptest.ResponseRecorder, target interface{}) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
		t.Fatalf("failed to parse JSON response: %v, raw body: %s", err, w.Body.String())
	}
}

func newJSONRequest(method, path string, body interface{}, ip string) *http.Request {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if ip != "" {
		req.RemoteAddr = ip + ":54321"
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestM1_AccountLifecycle_EndToEnd(t *testing.T) {
	engine, _, cleanup := setupE2ETest(t)
	if cleanup != nil {
		defer cleanup()
	}

	suffix := time.Now().Format("0405_000")
	testUser := fmt.Sprintf("u_%s", suffix)
	testPass := "StrongPass123"

	ipRegister := "192.168.10.11"
	ipLogin := "192.168.10.12"
	ipRefresh := "192.168.10.13"
	ipLogout := "192.168.10.14"
	ipRateTest := fmt.Sprintf("192.168.99.%d", time.Now().Nanosecond()%200+10)

	// ==========================================
	// 1. FR-ACC-01: 用户名密码注册
	// ==========================================

	// 1.1 密码强度不足 -> 400 invalid_param
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/register", RegisterRequest{Username: testUser, Password: "123"}, ipRegister))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for weak password, got %d: %s", w.Code, w.Body.String())
	}
	var weakResp response.Response
	parseJSONResponse(t, w, &weakResp)
	if weakResp.Error == nil || weakResp.Error.Code != response.CodeInvalidParam {
		t.Fatalf("expected error code %q, got %+v", response.CodeInvalidParam, weakResp.Error)
	}

	// 1.2 成功注册 -> 201 Created
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/register", RegisterRequest{Username: testUser, Password: testPass}, ipRegister))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for register, got %d: %s", w.Code, w.Body.String())
	}
	var regResp struct {
		Data UserProfileResponse `json:"data"`
	}
	parseJSONResponse(t, w, &regResp)
	if regResp.Data.ID <= 0 || regResp.Data.Username != testUser {
		t.Fatalf("unexpected register response data: %+v", regResp.Data)
	}
	createdUserID := regResp.Data.ID

	// 1.3 用户名重复注册 -> 409 Conflict
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/register", RegisterRequest{Username: testUser, Password: testPass}, ipRegister))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for duplicate username, got %d: %s", w.Code, w.Body.String())
	}
	var dupResp response.Response
	parseJSONResponse(t, w, &dupResp)
	if dupResp.Error == nil || dupResp.Error.Code != response.CodeConflict {
		t.Fatalf("expected error code %q, got %+v", response.CodeConflict, dupResp.Error)
	}

	// ==========================================
	// 2. FR-ACC-02: 登录认证与双 Token 签发
	// ==========================================

	// 2.1 凭证错误 -> 401 Unauthorized
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/login", LoginRequest{Username: testUser, Password: "WrongPassword999"}, ipLogin))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong credentials, got %d: %s", w.Code, w.Body.String())
	}

	// 2.2 登录成功 -> 200 OK + Access + Refresh Token
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/login", LoginRequest{Username: testUser, Password: testPass}, ipLogin))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for login, got %d: %s", w.Code, w.Body.String())
	}
	var loginResp struct {
		Data LoginResponse `json:"data"`
	}
	parseJSONResponse(t, w, &loginResp)
	accessToken := loginResp.Data.Tokens.AccessToken
	refreshToken := loginResp.Data.Tokens.RefreshToken
	if accessToken == "" || refreshToken == "" || loginResp.Data.Tokens.ExpiresIn <= 0 {
		t.Fatalf("expected valid tokens, got %+v", loginResp.Data.Tokens)
	}
	if loginResp.Data.User.ID != createdUserID {
		t.Fatalf("user id mismatch: expected %d, got %d", createdUserID, loginResp.Data.User.ID)
	}

	// ==========================================
	// 3. FR-ACC-05: 用户资料维护与公开资料查询
	// ==========================================

	// 3.1 未认证访问 /users/me -> 401 Unauthorized
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodGet, "/api/v1/users/me", nil, ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /users/me, got %d", w.Code)
	}

	// 3.2 携带 Access Token 访问 /users/me -> 200 OK
	reqMe := newJSONRequest(http.MethodGet, "/api/v1/users/me", nil, "")
	reqMe.Header.Set("Authorization", "Bearer "+accessToken)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, reqMe)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated /users/me, got %d", w.Code)
	}

	// 3.3 更新个人资料 PUT /users/me
	reqUpdate := newJSONRequest(http.MethodPut, "/api/v1/users/me", UpdateProfileRequest{
		Nickname: "E2E Nickname",
		Bio:      "E2E Bio description",
	}, "")
	reqUpdate.Header.Set("Authorization", "Bearer "+accessToken)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, reqUpdate)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for update profile, got %d: %s", w.Code, w.Body.String())
	}
	var updatedResp struct {
		Data UserProfileResponse `json:"data"`
	}
	parseJSONResponse(t, w, &updatedResp)
	if updatedResp.Data.Nickname != "E2E Nickname" || updatedResp.Data.Bio != "E2E Bio description" {
		t.Fatalf("updated profile mismatch: %+v", updatedResp.Data)
	}

	// 3.4 他人公开主页查询 GET /users/:userId (不泄露 username)
	reqPub := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/users/%d", createdUserID), nil, "")
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, reqPub)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for public profile, got %d", w.Code)
	}
	var pubResp struct {
		Data PublicUserProfileResponse `json:"data"`
	}
	parseJSONResponse(t, w, &pubResp)
	if pubResp.Data.ID != createdUserID || pubResp.Data.Nickname != "E2E Nickname" {
		t.Fatalf("public profile mismatch: %+v", pubResp.Data)
	}

	// ==========================================
	// 4. FR-ACC-06: 头像上传与媒体服务
	// ==========================================
	validPNGBytes := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
		0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "avatar.png")
	_, _ = part.Write(validPNGBytes)
	_ = writer.Close()

	reqAvatar := httptest.NewRequest(http.MethodPost, "/api/v1/users/avatar", body)
	reqAvatar.Header.Set("Content-Type", writer.FormDataContentType())
	reqAvatar.Header.Set("Authorization", "Bearer "+accessToken)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, reqAvatar)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for avatar upload, got %d: %s", w.Code, w.Body.String())
	}
	var avResp struct {
		Data struct {
			AvatarURL string `json:"avatar_url"`
		} `json:"data"`
	}
	parseJSONResponse(t, w, &avResp)
	avatarURL := avResp.Data.AvatarURL
	if avatarURL == "" {
		t.Fatalf("expected non-empty avatar_url")
	}

	// 验证可通过 /media/* 路由拉取到该头像
	reqMedia := httptest.NewRequest(http.MethodGet, avatarURL, nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, reqMedia)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for media avatar access, got %d: %s", w.Code, w.Body.String())
	}

	// ==========================================
	// 5. FR-ACC-03: Token 轮转与重放防护
	// ==========================================

	// 5.1 正常轮转：使用 refreshToken 换新
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/refresh", RefreshRequest{RefreshToken: refreshToken}, ipRefresh))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for refresh token, got %d: %s", w.Code, w.Body.String())
	}
	var rotResp struct {
		Data serviceaccount.TokenPair `json:"data"`
	}
	parseJSONResponse(t, w, &rotResp)
	newAccessToken := rotResp.Data.AccessToken
	newRefreshToken := rotResp.Data.RefreshToken
	if newAccessToken == accessToken || newRefreshToken == refreshToken {
		t.Fatalf("expected both tokens to be refreshed and rotated")
	}

	// 5.2 验证新 Access Token 可用
	reqMeNew := newJSONRequest(http.MethodGet, "/api/v1/users/me", nil, "")
	reqMeNew.Header.Set("Authorization", "Bearer "+newAccessToken)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, reqMeNew)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with new access token, got %d", w.Code)
	}

	// 5.3 重放攻击检测：故意再次使用已轮转作废的旧 refreshToken
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/refresh", RefreshRequest{RefreshToken: refreshToken}, ipRefresh))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for replay attack, got %d", w.Code)
	}

	// 5.4 验证重放防护已瞬间销毁整个会话：即使刚刚换发的 newRefreshToken 也被强制吊销
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/refresh", RefreshRequest{RefreshToken: newRefreshToken}, ipRefresh))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after session revocation, got %d", w.Code)
	}

	// ==========================================
	// 6. FR-ACC-04: 登出与 Access Token 黑名单
	// ==========================================

	// 重新登录获取有效 Token 进行登出测试
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/login", LoginRequest{Username: testUser, Password: testPass}, ipLogout))
	if w.Code != http.StatusOK {
		t.Fatalf("re-login failed: %d", w.Code)
	}
	var reloginResp struct {
		Data LoginResponse `json:"data"`
	}
	parseJSONResponse(t, w, &reloginResp)
	activeAccess := reloginResp.Data.Tokens.AccessToken
	activeRefresh := reloginResp.Data.Tokens.RefreshToken

	// 正常退出登录 POST /api/v1/auth/logout
	reqLogout := newJSONRequest(http.MethodPost, "/api/v1/auth/logout", LogoutRequest{RefreshToken: activeRefresh}, ipLogout)
	reqLogout.Header.Set("Authorization", "Bearer "+activeAccess)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, reqLogout)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for logout, got %d: %s", w.Code, w.Body.String())
	}

	// 退出登录后：尝试使用刚才登出的 Access Token 访问受保护接口 -> 必须被黑名单拦截 (401)
	reqMeLoggedOut := newJSONRequest(http.MethodGet, "/api/v1/users/me", nil, "")
	reqMeLoggedOut.Header.Set("Authorization", "Bearer "+activeAccess)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, reqMeLoggedOut)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for blacklisted token, got %d", w.Code)
	}

	// 退出登录后：尝试使用刚才登出的 Refresh Token 刷新 -> 必须被拦截 (401)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/refresh", RefreshRequest{RefreshToken: activeRefresh}, ipLogout))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for revoked refresh session, got %d", w.Code)
	}

	// ==========================================
	// 7. Security: IP 级滑动窗口限流验证 (10 req/min)
	// ==========================================
	for i := 1; i <= 10; i++ {
		w = httptest.NewRecorder()
		engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/login", LoginRequest{
			Username: fmt.Sprintf("probe_user_%d", i),
			Password: "SomePass123",
		}, ipRateTest))
		if w.Code == http.StatusTooManyRequests {
			t.Fatalf("request #%d unexpectedly rate limited", i)
		}
	}

	// 第 11 次请求应立即触发 429 Too Many Requests
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/login", LoginRequest{
		Username: "probe_user_11",
		Password: "SomePass123",
	}, ipRateTest))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests on 11th request, got %d: %s", w.Code, w.Body.String())
	}
	var rateResp response.Response
	parseJSONResponse(t, w, &rateResp)
	if rateResp.Error == nil || rateResp.Error.Code != response.CodeRateLimited {
		t.Fatalf("expected error code %q, got %+v", response.CodeRateLimited, rateResp.Error)
	}
}
