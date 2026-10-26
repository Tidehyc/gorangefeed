package video_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpvideo "github.com/Tidehyc/gorangefeed/internal/http/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// mockVideoService implements servicevideo.Service for HTTP handler unit tests.
type mockVideoService struct {
	publishFn          func(ctx context.Context, authorID int64, title, description, playURL, coverURL string, durationMs int, idempotencyKey string) (*domainvideo.VideoDetail, error)
	getDetailFn        func(ctx context.Context, videoID int64, currentUserID int64) (*domainvideo.VideoDetail, error)
	listAuthorVideosFn func(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error)
	deleteVideoFn      func(ctx context.Context, videoID int64, currentUserID int64) error
}

func (m *mockVideoService) Publish(ctx context.Context, authorID int64, title, description, playURL, coverURL string, durationMs int, idempotencyKey string) (*domainvideo.VideoDetail, error) {
	if m.publishFn != nil {
		return m.publishFn(ctx, authorID, title, description, playURL, coverURL, durationMs, idempotencyKey)
	}
	return &domainvideo.VideoDetail{
		Video: &domainvideo.Video{ID: 100, AuthorID: authorID, Title: title},
		Stats: &domainvideo.VideoStats{VideoID: 100},
	}, nil
}

func (m *mockVideoService) GetDetail(ctx context.Context, videoID int64, currentUserID int64) (*domainvideo.VideoDetail, error) {
	if m.getDetailFn != nil {
		return m.getDetailFn(ctx, videoID, currentUserID)
	}
	return nil, response.ErrNotFound("video not found")
}

func (m *mockVideoService) ListAuthorVideos(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error) {
	if m.listAuthorVideosFn != nil {
		return m.listAuthorVideosFn(ctx, authorID, cursor, limit)
	}
	return []*domainvideo.VideoDetail{}, 0, false, nil
}

func (m *mockVideoService) DeleteVideo(ctx context.Context, videoID int64, currentUserID int64) error {
	if m.deleteVideoFn != nil {
		return m.deleteVideoFn(ctx, videoID, currentUserID)
	}
	return nil
}

// mockTokenStore implements domainaccount.TokenStore for testing routes.
type mockTokenStore struct{}

func (m *mockTokenStore) CreateSession(ctx context.Context, sessionID, tokenID string, ttl time.Duration) error {
	return nil
}
func (m *mockTokenStore) RotateSession(ctx context.Context, sessionID, oldTokenID, newTokenID string, ttl time.Duration) (domainaccount.RotationResult, error) {
	return domainaccount.RotationOK, nil
}
func (m *mockTokenStore) RevokeSession(ctx context.Context, sessionID string) error { return nil }
func (m *mockTokenStore) BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	return nil
}
func (m *mockTokenStore) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	return false, nil
}

func setupTestRouter(t *testing.T, svc *mockVideoService, store *storage.VideoStorage) (*gin.Engine, *jwt.TokenManager, string) {
	t.Helper()
	jwtCfg := config.JWTConfig{
		AccessSecret:  "test_secret_for_handler_testing_key_12345",
		RefreshSecret: "test_refresh_secret_key_12345",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    7 * 24 * time.Hour,
	}
	jwtMgr := jwt.NewTokenManager(jwtCfg)

	// Issue a valid token for userID = 1001
	validToken, _, err := jwtMgr.GenerateAccessToken(1001, "tok-1")
	if err != nil {
		t.Fatalf("failed to generate access token: %v", err)
	}

	h := httpvideo.NewHandler(svc, store)
	r := gin.New()
	r.Use(gin.Recovery())

	httpvideo.RegisterRoutes(r, h, jwtMgr, &mockTokenStore{}, nil)
	return r, jwtMgr, validToken
}

func fakeMP4Bytes() []byte {
	header := []byte{
		0x00, 0x00, 0x00, 0x18,
		'f', 't', 'y', 'p',
		'm', 'p', '4', '2',
		0x00, 0x00, 0x00, 0x00,
		'i', 's', 'o', 'm',
		'm', 'p', '4', '2',
	}
	return append(header, bytes.Repeat([]byte{0x01}, 100)...)
}

func TestHandler_UploadVideo(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewVideoStorage(tempDir)
	svc := &mockVideoService{}
	r, _, validToken := setupTestRouter(t, svc, store)

	// 1. Unauthenticated -> 401 Unauthorized
	t.Run("unauthenticated rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/videos/upload", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	// 2. Authenticated but missing file -> 400 Bad Request
	t.Run("missing file 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/videos/upload", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	// 3. Authenticated valid upload -> 201 Created
	t.Run("valid mp4 upload 201", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "valid.mp4")
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		part.Write(fakeMP4Bytes())
		writer.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/videos/upload", body)
		req.Header.Set("Authorization", "Bearer "+validToken)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
		}

		var resp response.Response
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("parse response failed: %v", err)
		}
		if resp.Error != nil {
			t.Fatalf("expected nil error, got %+v", resp.Error)
		}
	})
}

func TestHandler_Publish(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewVideoStorage(tempDir)

	var capturedKey string
	svc := &mockVideoService{
		publishFn: func(ctx context.Context, authorID int64, title, description, playURL, coverURL string, durationMs int, idempotencyKey string) (*domainvideo.VideoDetail, error) {
			capturedKey = idempotencyKey
			return &domainvideo.VideoDetail{
				Video: &domainvideo.Video{ID: 202, AuthorID: authorID, Title: title},
				Stats: &domainvideo.VideoStats{VideoID: 202},
			}, nil
		},
	}
	r, _, validToken := setupTestRouter(t, svc, store)

	// 1. Publish with Idempotency-Key header
	bodyJSON, _ := json.Marshal(httpvideo.PublishVideoRequest{
		Title:       "Sample Title",
		Description: "Sample Description",
		PlayURL:     "http://example.com/media/videos/1.mp4",
		DurationMs:  15000,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/videos", bytes.NewReader(bodyJSON))
	req.Header.Set("Authorization", "Bearer "+validToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "header_idem_key_777")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}
	if capturedKey != "header_idem_key_777" {
		t.Fatalf("expected captured key 'header_idem_key_777', got %q", capturedKey)
	}
}

func TestHandler_GetDetail(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewVideoStorage(tempDir)
	svc := &mockVideoService{
		getDetailFn: func(ctx context.Context, videoID int64, currentUserID int64) (*domainvideo.VideoDetail, error) {
			if videoID == 123 {
				return &domainvideo.VideoDetail{
					Video: &domainvideo.Video{ID: 123, Title: "Existing Video"},
					Stats: &domainvideo.VideoStats{VideoID: 123, ViewCount: 10},
				}, nil
			}
			return nil, response.ErrNotFound("video not found")
		},
	}
	r, _, _ := setupTestRouter(t, svc, store)

	// 1. Existing video -> 200 OK
	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 2. Missing video -> 404 Not Found
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/videos/999", nil)
	w404 := httptest.NewRecorder()
	r.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w404.Code)
	}

	// 3. Invalid videoId -> 400 Bad Request
	reqBad := httptest.NewRequest(http.MethodGet, "/api/v1/videos/abc", nil)
	wBad := httptest.NewRecorder()
	r.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", wBad.Code)
	}
}

func TestHandler_ListAuthorVideos(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewVideoStorage(tempDir)
	svc := &mockVideoService{
		listAuthorVideosFn: func(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error) {
			return []*domainvideo.VideoDetail{
				{Video: &domainvideo.Video{ID: 10, Title: "Work 1"}},
			}, 10, true, nil
		},
	}
	r, _, _ := setupTestRouter(t, svc, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/1001/videos?cursor=20&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response failed: %v", err)
	}
	dataMap := resp.Data.(map[string]interface{})
	if dataMap["next_cursor"] != float64(10) || dataMap["has_more"] != true {
		t.Fatalf("unexpected pagination data: %+v", dataMap)
	}
}

func TestHandler_DeleteVideo(t *testing.T) {
	tempDir := t.TempDir()
	store, _ := storage.NewVideoStorage(tempDir)
	svc := &mockVideoService{
		deleteVideoFn: func(ctx context.Context, videoID int64, currentUserID int64) error {
			if videoID == 88 && currentUserID == 1001 {
				return nil
			}
			if videoID == 88 && currentUserID != 1001 {
				return response.ErrForbidden("forbidden")
			}
			return response.ErrNotFound("video not found")
		},
	}
	r, _, validToken := setupTestRouter(t, svc, store)

	// 1. Author deletes video -> 200 OK
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/videos/88", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 2. Unauthenticated delete -> 401
	reqUnauth := httptest.NewRequest(http.MethodDelete, "/api/v1/videos/88", nil)
	wUnauth := httptest.NewRecorder()
	r.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", wUnauth.Code)
	}
}
