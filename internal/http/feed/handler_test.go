package feed_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	httpfeed "github.com/Tidehyc/gorangefeed/internal/http/feed"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

type mockFeedService struct {
	getGlobalFeedFn func(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error)
	recordViewFn    func(ctx context.Context, viewerID int64, videoID int64, event domainfeed.ViewEvent) error
}

func (m *mockFeedService) GetGlobalFeed(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error) {
	if m.getGlobalFeedFn != nil {
		return m.getGlobalFeedFn(ctx, viewerID, cursor, limit)
	}
	return &domainfeed.FeedPage{Items: []*domainfeed.FeedItem{}}, nil
}

func (m *mockFeedService) RecordView(ctx context.Context, viewerID int64, videoID int64, event domainfeed.ViewEvent) error {
	if m.recordViewFn != nil {
		return m.recordViewFn(ctx, viewerID, videoID, event)
	}
	return nil
}

func (m *mockFeedService) AppendTimeline(ctx context.Context, videoID int64) error {
	return nil
}

func (m *mockFeedService) SyncFloorAndTrim(ctx context.Context) (int64, error) {
	return 0, nil
}

func setupTestEngine(svc *mockFeedService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := httpfeed.NewHandler(svc)

	r.GET("/api/v1/feed/global", handler.GetGlobalFeed)
	r.POST("/api/v1/videos/:videoId/view", handler.RecordView)
	return r
}

func TestHandler_GetGlobalFeed(t *testing.T) {
	mockSvc := &mockFeedService{
		getGlobalFeedFn: func(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error) {
			return &domainfeed.FeedPage{
				Items: []*domainfeed.FeedItem{
					{
						ID:    101,
						Title: "Mock Video 101",
						Stats: domainfeed.FeedStat{VideoID: 101, ViewCount: 42},
					},
				},
				NextCursor: 101,
				HasMore:    true,
			}, nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Success request
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/global?cursor=0&limit=10", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response envelope: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("expected error to be nil, got: %+v", resp.Error)
	}

	// 2. Invalid cursor (< 0)
	reqBadCursor, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/global?cursor=-1", nil)
	wBadCursor := httptest.NewRecorder()
	engine.ServeHTTP(wBadCursor, reqBadCursor)
	if wBadCursor.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative cursor, got %d", wBadCursor.Code)
	}

	// 3. Invalid limit (<= 0)
	reqBadLimit, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/global?limit=0", nil)
	wBadLimit := httptest.NewRecorder()
	engine.ServeHTTP(wBadLimit, reqBadLimit)
	if wBadLimit.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for 0 limit, got %d", wBadLimit.Code)
	}
}

func TestHandler_RecordView(t *testing.T) {
	var capturedViewerID int64
	var capturedVideoID int64
	var capturedEventType domainfeed.ViewEventType

	mockSvc := &mockFeedService{
		recordViewFn: func(ctx context.Context, viewerID int64, videoID int64, event domainfeed.ViewEvent) error {
			capturedViewerID = viewerID
			capturedVideoID = videoID
			capturedEventType = event.EventType
			if videoID == 999999 {
				return domainfeed.ErrVideoNotFound
			}
			return nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Successful view report
	body, _ := json.Marshal(httpfeed.RecordViewRequest{
		EventType:       "complete",
		WatchDurationMs: 15000,
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/12345/view", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if capturedVideoID != 12345 || capturedEventType != domainfeed.ViewEventComplete || capturedViewerID != 0 {
		t.Fatalf("unexpected captured params: video=%d, event=%s, viewer=%d", capturedVideoID, capturedEventType, capturedViewerID)
	}

	// 2. Video not found -> 404
	reqNotFound, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/999999/view", nil)
	wNotFound := httptest.NewRecorder()
	engine.ServeHTTP(wNotFound, reqNotFound)
	if wNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent video, got %d", wNotFound.Code)
	}

	// 3. Invalid videoId param
	reqBadID, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/abc/view", nil)
	wBadID := httptest.NewRecorder()
	engine.ServeHTTP(wBadID, reqBadID)
	if wBadID.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad videoId, got %d", wBadID.Code)
	}
}
