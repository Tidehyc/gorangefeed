package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
	httpadmin "github.com/Tidehyc/gorangefeed/internal/http/admin"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/gin-gonic/gin"
)

type mockModerationService struct {
	submitFn   func(ctx context.Context, videoID int64, policy domainmoderation.ReviewPolicy, title, desc string) (*domainmoderation.ReviewTask, error)
	reviewFn   func(ctx context.Context, videoID int64, reviewerID int64, action string, note string) error
	takedownFn func(ctx context.Context, videoID int64, reviewerID int64, reason string) error
	banUserFn  func(ctx context.Context, userID int64, operatorID int64, reason string) error
	listFn     func(ctx context.Context, status domainmoderation.ReviewStatus, cursor int64, limit int) ([]*domainmoderation.ReviewTask, int64, bool, error)
	logsFn     func(ctx context.Context, targetType string, targetID int64, limit int) ([]*domainmoderation.ModerationLog, error)
}

func (m *mockModerationService) SubmitVideoForReview(ctx context.Context, videoID int64, policy domainmoderation.ReviewPolicy, title, desc string) (*domainmoderation.ReviewTask, error) {
	if m.submitFn != nil {
		return m.submitFn(ctx, videoID, policy, title, desc)
	}
	return nil, nil
}
func (m *mockModerationService) ReviewVideo(ctx context.Context, videoID int64, reviewerID int64, action string, note string) error {
	if m.reviewFn != nil {
		return m.reviewFn(ctx, videoID, reviewerID, action, note)
	}
	return nil
}
func (m *mockModerationService) TakedownVideo(ctx context.Context, videoID int64, reviewerID int64, reason string) error {
	if m.takedownFn != nil {
		return m.takedownFn(ctx, videoID, reviewerID, reason)
	}
	return nil
}
func (m *mockModerationService) BanUser(ctx context.Context, userID int64, operatorID int64, reason string) error {
	if m.banUserFn != nil {
		return m.banUserFn(ctx, userID, operatorID, reason)
	}
	return nil
}
func (m *mockModerationService) ListReviewTasks(ctx context.Context, status domainmoderation.ReviewStatus, cursor int64, limit int) ([]*domainmoderation.ReviewTask, int64, bool, error) {
	if m.listFn != nil {
		return m.listFn(ctx, status, cursor, limit)
	}
	return []*domainmoderation.ReviewTask{
		{ID: 1, VideoID: 101, Status: domainmoderation.ReviewStatusPendingHuman},
	}, 1, false, nil
}
func (m *mockModerationService) ListAuditLogs(ctx context.Context, targetType string, targetID int64, limit int) ([]*domainmoderation.ModerationLog, error) {
	if m.logsFn != nil {
		return m.logsFn(ctx, targetType, targetID, limit)
	}
	return nil, nil
}

func setupAdminRouter(svc *mockModerationService) (*gin.Engine, string) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	jwtCfg := config.JWTConfig{
		AccessSecret:  "test_secret_for_admin_handler_32_bytes_long!!",
		RefreshSecret: "test_refresh_secret_admin_32_bytes_long!!",
		AccessTTL:     time.Hour,
		RefreshTTL:    time.Hour * 24,
	}
	jwtMgr := jwt.NewTokenManager(jwtCfg)
	token, _, _ := jwtMgr.GenerateAccessToken(999, "tok-admin-1") // Admin UserID = 999

	h := httpadmin.NewHandler(svc)
	httpadmin.RegisterRoutes(r, h, jwtMgr, nil)

	return r, token
}

func TestAdminHandler_ListReviews(t *testing.T) {
	svc := &mockModerationService{}
	router, token := setupAdminRouter(svc)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/reviews?status=2&limit=10", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminHandler_ReviewVideo(t *testing.T) {
	reviewed := false
	svc := &mockModerationService{
		reviewFn: func(ctx context.Context, videoID int64, reviewerID int64, action string, note string) error {
			if videoID == 101 && reviewerID == 999 && action == "approve" {
				reviewed = true
			}
			return nil
		},
	}
	router, token := setupAdminRouter(svc)

	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"action":      "approve",
		"review_note": "looks clean and complies with policy",
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/videos/101/review", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	if !reviewed {
		t.Fatalf("expected reviewFn to be invoked with matching params")
	}
}

func TestAdminHandler_TakedownVideo(t *testing.T) {
	takedownCalled := false
	svc := &mockModerationService{
		takedownFn: func(ctx context.Context, videoID int64, reviewerID int64, reason string) error {
			if videoID == 202 && reviewerID == 999 && reason == "DMCA notice" {
				takedownCalled = true
			}
			return nil
		},
	}
	router, token := setupAdminRouter(svc)

	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"reason": "DMCA notice",
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/videos/202/takedown", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	if !takedownCalled {
		t.Fatalf("expected takedownFn to be called")
	}
}

func TestAdminHandler_BanUser(t *testing.T) {
	banned := false
	svc := &mockModerationService{
		banUserFn: func(ctx context.Context, userID int64, operatorID int64, reason string) error {
			if userID == 303 && operatorID == 999 {
				banned = true
			}
			return nil
		},
	}
	router, token := setupAdminRouter(svc)

	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"reason": "malicious spamming",
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/users/303/ban", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	if !banned {
		t.Fatalf("expected banUserFn to be called")
	}
}
