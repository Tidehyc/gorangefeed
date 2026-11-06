package notification_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	httpnotification "github.com/Tidehyc/gorangefeed/internal/http/notification"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	servicenotification "github.com/Tidehyc/gorangefeed/internal/service/notification"
	"github.com/gin-gonic/gin"
)

type mockService struct {
	items       []*domainnotification.Notification
	unreadCount int64
	markedIDs   []int64
	allMarked   int64
}

func (m *mockService) ListNotifications(ctx context.Context, recipientID int64, filterType string, cursorStr string, limit int) ([]*domainnotification.Notification, string, bool, error) {
	return m.items, "next_cursor_test", false, nil
}

func (m *mockService) GetUnreadCount(ctx context.Context, recipientID int64) (int64, error) {
	return m.unreadCount, nil
}

func (m *mockService) MarkAsRead(ctx context.Context, recipientID, notificationID int64) error {
	m.markedIDs = append(m.markedIDs, notificationID)
	return nil
}

func (m *mockService) MarkAllAsRead(ctx context.Context, recipientID int64) (int64, error) {
	return m.allMarked, nil
}

type mockHub struct {
	tickets map[string]int64
}

func newMockHub() *mockHub {
	return &mockHub{tickets: make(map[string]int64)}
}

func (m *mockHub) CreateTicket(ctx context.Context, userID int64) (string, int, error) {
	ticket := "test-ticket-uuid"
	m.tickets[ticket] = userID
	return ticket, 60, nil
}

func (m *mockHub) ConsumeTicket(ctx context.Context, ticket string) (int64, error) {
	if uid, ok := m.tickets[ticket]; ok {
		delete(m.tickets, ticket)
		return uid, nil
	}
	return 0, domainnotification.ErrTicketExpired
}

func (m *mockHub) Register(userID int64) chan *servicenotification.SSEMessage {
	ch := make(chan *servicenotification.SSEMessage, 10)
	return ch
}

func (m *mockHub) Unregister(userID int64, ch chan *servicenotification.SSEMessage) {
	close(ch)
}

func (m *mockHub) GetCatchUpNotifications(ctx context.Context, recipientID int64, lastEventID string) ([]*domainnotification.Notification, error) {
	return nil, nil
}

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

func (m *mockTokenStore) MarkUserBanned(ctx context.Context, userID int64, ttl time.Duration) error {
	return nil
}
func (m *mockTokenStore) ClearUserBanned(ctx context.Context, userID int64) error { return nil }
func (m *mockTokenStore) IsUserBanned(ctx context.Context, userID int64) (bool, error) {
	return false, nil
}

func (m *mockTokenStore) InvalidateUserTokens(ctx context.Context, userID int64, invalidBefore time.Time, ttl time.Duration) error {
	return nil
}

func (m *mockTokenStore) IsUserTokenInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error) {
	return false, nil
}

func setupTestRouter(svc *mockService, hub *mockHub) (*gin.Engine, *jwt.TokenManager, string) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	jwtCfg := config.JWTConfig{
		AccessSecret:  "test_secret_for_handler_testing_key_12345",
		RefreshSecret: "test_refresh_secret_key_12345",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    7 * 24 * time.Hour,
	}
	jwtMgr := jwt.NewTokenManager(jwtCfg)

	h := httpnotification.NewHandler(svc, hub)
	tokenStore := &mockTokenStore{}
	httpnotification.RegisterRoutes(r, h, jwtMgr, tokenStore, nil)

	// Generate valid access token for user 100
	validToken, _, _ := jwtMgr.GenerateAccessToken(100, "tok-test-1")
	return r, jwtMgr, validToken
}

func TestHandler_RESTEndpoints(t *testing.T) {
	svc := &mockService{
		items: []*domainnotification.Notification{
			{ID: 1, RecipientID: 100, Title: "Test 1", Type: domainnotification.TypeLike},
		},
		unreadCount: 3,
		allMarked:   3,
	}
	hub := newMockHub()
	r, _, token := setupTestRouter(svc, hub)

	t.Run("GET /api/v1/notifications without auth returns 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("GET /api/v1/notifications with auth returns 200", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/notifications?limit=10", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d (body: %s)", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		data := resp["data"].(map[string]any)
		items := data["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
	})

	t.Run("GET /api/v1/notifications/unread-count returns 200", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]any)
		if int(data["unread_count"].(float64)) != 3 {
			t.Fatalf("expected unread_count 3, got %v", data["unread_count"])
		}
	})

	t.Run("PATCH /api/v1/notifications/:id/read marks read", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPatch, "/api/v1/notifications/1/read", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		if len(svc.markedIDs) != 1 || svc.markedIDs[0] != 1 {
			t.Fatalf("expected marked ID 1, got %v", svc.markedIDs)
		}
	})

	t.Run("POST /api/v1/notifications/read-all marks all", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/notifications/read-all", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]any)
		if int(data["marked_count"].(float64)) != 3 {
			t.Fatalf("expected marked_count 3, got %v", data["marked_count"])
		}
	})

	t.Run("POST /api/v1/notifications/sse/ticket exchanges ticket", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/notifications/sse/ticket", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		data := resp["data"].(map[string]any)
		if data["ticket"] != "test-ticket-uuid" || int(data["expires_in"].(float64)) != 60 {
			t.Fatalf("unexpected ticket response: %v", data)
		}
	})
}

func TestHandler_SSEStream(t *testing.T) {
	svc := &mockService{}
	hub := newMockHub()
	hub.tickets["valid-ticket"] = 100
	r, _, _ := setupTestRouter(svc, hub)

	t.Run("stream without ticket or header returns 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/notifications/stream", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})

	t.Run("stream with invalid ticket returns 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/notifications/stream?ticket=invalid-ticket", nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
	})
}
