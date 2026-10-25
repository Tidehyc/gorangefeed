package account

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
	"github.com/gin-gonic/gin"
)

func setupTestHandler(t *testing.T) (*Handler, Service, *jwt.TokenManager, *storage.AvatarStorage, string) {
	tempDir, err := os.MkdirTemp("", "gorangefeed_handler_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := config.JWTConfig{
		AccessSecret:  "handler_test_access_secret_123",
		RefreshSecret: "handler_test_refresh_secret_123",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    7 * 24 * time.Hour,
	}
	jwtMgr := jwt.NewTokenManager(cfg)
	repo := newMockRepository()
	store := newMockTokenStore()
	svc := NewService(repo, store, jwtMgr, cfg)
	avatarStorage, err := storage.NewAvatarStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create avatar storage: %v", err)
	}

	handler := NewHandler(svc, jwtMgr, avatarStorage)
	return handler, svc, jwtMgr, avatarStorage, tempDir
}

func setupTestEngine(h *Handler, jwtMgr *jwt.TokenManager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	authGroup := r.Group("/api/v1/auth")
	{
		authGroup.POST("/register", h.Register)
		authGroup.POST("/login", h.Login)
		authGroup.POST("/refresh", h.RefreshToken)
		authGroup.POST("/logout", h.Logout)
	}

	userGroup := r.Group("/api/v1/users")
	{
		userGroup.GET("/me", middleware.JWTAuth(jwtMgr, nil), h.GetMe)
		userGroup.PUT("/me", middleware.JWTAuth(jwtMgr, nil), h.UpdateMe)
		userGroup.POST("/avatar", middleware.JWTAuth(jwtMgr, nil), h.UploadAvatar)
		userGroup.GET("/:userId", h.GetPublicProfile)
	}

	return r
}

func TestHandler_RegisterAndLoginFlow(t *testing.T) {
	h, _, jwtMgr, _, tempDir := setupTestHandler(t)
	defer os.RemoveAll(tempDir)

	r := setupTestEngine(h, jwtMgr)

	// 1. Register with empty fields -> 400
	body, _ := json.Marshal(RegisterRequest{Username: "", Password: ""})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty register, got %d", w.Code)
	}

	// 2. Register valid user -> 201 Created
	body, _ = json.Marshal(RegisterRequest{Username: "alice_test", Password: "Password123"})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for register, got %d: %s", w.Code, w.Body.String())
	}

	var regResp response.Response
	_ = json.Unmarshal(w.Body.Bytes(), &regResp)
	if regResp.Error != nil {
		t.Fatalf("expected nil error in response envelope: %+v", regResp.Error)
	}

	// 3. Register duplicate user -> 409 Conflict
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate register, got %d", w.Code)
	}

	// 4. Login wrong password -> 401
	body, _ = json.Marshal(LoginRequest{Username: "alice_test", Password: "WrongPassword123"})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", w.Code)
	}

	// 5. Login correct password -> 200 OK
	body, _ = json.Marshal(LoginRequest{Username: "alice_test", Password: "Password123"})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for login, got %d", w.Code)
	}

	var loginResp struct {
		Data LoginResponse `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &loginResp)
	accessToken := loginResp.Data.Tokens.AccessToken
	refreshToken := loginResp.Data.Tokens.RefreshToken
	if accessToken == "" || refreshToken == "" {
		t.Fatalf("expected non-empty tokens in login response: %+v", loginResp)
	}

	// 6. Refresh Token -> 200 OK
	refBody, _ := json.Marshal(RefreshRequest{RefreshToken: refreshToken})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(refBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for refresh, got %d", w.Code)
	}

	// 7. GetMe with accessToken -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /users/me, got %d", w.Code)
	}

	var meResp struct {
		Data UserProfileResponse `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &meResp)
	if meResp.Data.Username != "alice_test" {
		t.Fatalf("expected alice_test, got %s", meResp.Data.Username)
	}

	// 8. UpdateMe -> 200 OK
	upBody, _ := json.Marshal(UpdateProfileRequest{Nickname: "AliceInWonderland", Bio: "Hello Feed!"})
	req, _ = http.NewRequest(http.MethodPut, "/api/v1/users/me", bytes.NewReader(upBody))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for update /users/me, got %d", w.Code)
	}

	// 9. GetPublicProfile -> 200 OK (verifying username is omitted)
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/users/1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for public profile, got %d", w.Code)
	}

	var pubResp struct {
		Data PublicUserProfileResponse `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &pubResp)
	if pubResp.Data.Nickname != "AliceInWonderland" || pubResp.Data.Bio != "Hello Feed!" {
		t.Fatalf("public profile mismatch: %+v", pubResp.Data)
	}

	// 10. Logout -> 200 OK
	outBody, _ := json.Marshal(LogoutRequest{RefreshToken: refreshToken})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/logout", bytes.NewReader(outBody))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for logout, got %d", w.Code)
	}
}

func TestHandler_UploadAvatar(t *testing.T) {
	h, svc, jwtMgr, _, tempDir := setupTestHandler(t)
	defer os.RemoveAll(tempDir)

	r := setupTestEngine(h, jwtMgr)

	user, _ := svc.Register(t.Context(), "avatar_user", "Password123")
	pair, _, _ := svc.Login(t.Context(), "avatar_user", "Password123")

	// 1x1 valid PNG image
	validPNG := []byte{
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
	_, _ = part.Write(validPNG)
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users/avatar", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for avatar upload, got %d: %s", w.Code, w.Body.String())
	}

	var avResp struct {
		Data struct {
			AvatarURL string `json:"avatar_url"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &avResp)
	if avResp.Data.AvatarURL == "" {
		t.Fatalf("expected non-empty avatar_url")
	}

	// Verify profile reflected updated avatar
	p, _ := svc.GetProfile(t.Context(), user.ID)
	if p.AvatarURL != avResp.Data.AvatarURL {
		t.Fatalf("avatar url not updated in profile: %s vs %s", p.AvatarURL, avResp.Data.AvatarURL)
	}
}
