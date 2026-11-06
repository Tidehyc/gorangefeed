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
	getGlobalFeedFn     func(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error)
	recordViewFn        func(ctx context.Context, viewerID int64, videoID int64, event domainfeed.ViewEvent) error
	listFollowingFeedFn func(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error)
	getLikeRankFeedFn   func(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.LikeRankPage, error)
	getHotFeedFn        func(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.HotFeedPage, error)
	getRecommendFeedFn  func(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.RecommendFeedPage, error)
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

func (m *mockFeedService) AppendTimelineWithSeq(ctx context.Context, videoID int64, seq int64) error {
	return nil
}

func (m *mockFeedService) SyncFloorAndTrim(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockFeedService) ListFollowingFeed(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error) {
	if m.listFollowingFeedFn != nil {
		return m.listFollowingFeedFn(ctx, viewerID, cursor, limit)
	}
	return &domainfeed.FeedPage{Items: []*domainfeed.FeedItem{}}, nil
}

func (m *mockFeedService) GetLikeRankFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.LikeRankPage, error) {
	if m.getLikeRankFeedFn != nil {
		return m.getLikeRankFeedFn(ctx, viewerID, cursor, limit)
	}
	return &domainfeed.LikeRankPage{Items: []*domainfeed.FeedItem{}}, nil
}

func (m *mockFeedService) GetHotFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.HotFeedPage, error) {
	if m.getHotFeedFn != nil {
		return m.getHotFeedFn(ctx, viewerID, cursor, limit)
	}
	return &domainfeed.HotFeedPage{Items: []*domainfeed.FeedItem{}}, nil
}

func (m *mockFeedService) ListHotFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.HotFeedPage, error) {
	return m.GetHotFeed(ctx, viewerID, cursor, limit)
}

func (m *mockFeedService) GetTagFeed(ctx context.Context, viewerID int64, tag string, cursor int64, limit int) (*domainfeed.FeedPage, error) {
	return &domainfeed.FeedPage{Items: []*domainfeed.FeedItem{}}, nil
}

func (m *mockFeedService) GetRecommendFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.RecommendFeedPage, error) {
	if m.getRecommendFeedFn != nil {
		return m.getRecommendFeedFn(ctx, viewerID, cursor, limit)
	}
	return &domainfeed.RecommendFeedPage{Items: []*domainfeed.FeedItem{}}, nil
}

func (m *mockFeedService) ListRecommendFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.RecommendFeedPage, error) {
	return m.GetRecommendFeed(ctx, viewerID, cursor, limit)
}

func setupTestEngine(svc *mockFeedService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := httpfeed.NewHandler(svc)

	r.GET("/api/v1/feed/global", handler.GetGlobalFeed)
	r.GET("/api/v1/feed/following", handler.GetFollowingFeed)
	r.GET("/api/v1/feed/like-rank", handler.GetLikeRankFeed)
	r.GET("/api/v1/feed/hot", handler.GetHotFeed)
	r.GET("/api/v1/feed/recommend", handler.GetRecommendFeed)
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

func TestHandler_GetFollowingFeed(t *testing.T) {
	mockSvc := &mockFeedService{
		listFollowingFeedFn: func(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error) {
			if viewerID != 1001 {
				t.Fatalf("expected viewerID=1001, got %d", viewerID)
			}
			return &domainfeed.FeedPage{
				Items: []*domainfeed.FeedItem{
					{ID: 201, Title: "Following Video 1"},
				},
				NextCursor: 201,
				HasMore:    true,
			}, nil
		},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := httpfeed.NewHandler(mockSvc)

	// Inject viewerID simulating JWTAuth
	r.GET("/api/v1/feed/following", func(c *gin.Context) {
		if auth := c.GetHeader("Authorization"); auth == "Bearer valid" {
			c.Set("user_id", int64(1001))
		}
		handler.GetFollowingFeed(c)
	})

	// 1. Authorized request -> 200 OK
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/following?cursor=0&limit=10", nil)
	req.Header.Set("Authorization", "Bearer valid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp response.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataMap := resp.Data.(map[string]interface{})
	items := dataMap["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	// 2. Unauthorized request -> 401
	reqUnauth, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/following", nil)
	wUnauth := httptest.NewRecorder()
	r.ServeHTTP(wUnauth, reqUnauth)

	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", wUnauth.Code)
	}

	// 3. Bad cursor -> 400
	reqBadCursor, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/following?cursor=-1", nil)
	reqBadCursor.Header.Set("Authorization", "Bearer valid")
	wBadCursor := httptest.NewRecorder()
	r.ServeHTTP(wBadCursor, reqBadCursor)

	if wBadCursor.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", wBadCursor.Code)
	}
}

func TestHandler_GetLikeRankFeed(t *testing.T) {
	mockSvc := &mockFeedService{
		getLikeRankFeedFn: func(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.LikeRankPage, error) {
			return &domainfeed.LikeRankPage{
				Items: []*domainfeed.FeedItem{
					{ID: 301, Title: "Top Liked Video"},
				},
				NextCursor: "500_301",
				HasMore:    true,
			}, nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Get like rank feed -> 200 OK
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/like-rank?cursor=1000_50&limit=5", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp response.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataMap := resp.Data.(map[string]interface{})
	if dataMap["next_cursor"] != "500_301" {
		t.Fatalf("expected next_cursor=500_301, got %v", dataMap["next_cursor"])
	}
}

func TestHandler_GetHotFeed(t *testing.T) {
	mockSvc := &mockFeedService{
		getHotFeedFn: func(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.HotFeedPage, error) {
			if cursor == "invalid-cursor-error" {
				return nil, domainfeed.ErrInvalidHotCursor
			}
			return &domainfeed.HotFeedPage{
				Items: []*domainfeed.FeedItem{
					{ID: 501, Title: "Hot Rank Video 1"},
					{ID: 502, Title: "Hot Rank Video 2"},
				},
				NextCursor: "eyJzY2VuZSI6ImhvdCIsImFzX29mIjoxNzMwMDAwMDAwLCJvZmZzZXQiOjJ9",
				HasMore:    true,
			}, nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Success request
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/hot?limit=2", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp response.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataMap := resp.Data.(map[string]interface{})
	if dataMap["has_more"] != true || dataMap["next_cursor"] == "" {
		t.Fatalf("expected has_more=true and non-empty next_cursor, got: %+v", dataMap)
	}

	// 2. Invalid limit <= 0
	reqBadLimit, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/hot?limit=0", nil)
	wBadLimit := httptest.NewRecorder()
	engine.ServeHTTP(wBadLimit, reqBadLimit)
	if wBadLimit.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for 0 limit, got %d", wBadLimit.Code)
	}

	// 3. Invalid cursor
	reqBadCursor, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/hot?cursor=invalid-cursor-error", nil)
	wBadCursor := httptest.NewRecorder()
	engine.ServeHTTP(wBadCursor, reqBadCursor)
	if wBadCursor.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad hot cursor, got %d", wBadCursor.Code)
	}
}

func TestHandler_GetRecommendFeed(t *testing.T) {
	mockSvc := &mockFeedService{
		getRecommendFeedFn: func(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.RecommendFeedPage, error) {
			if cursor == "invalid-recommend-cursor" {
				return nil, domainfeed.ErrInvalidRecommendCursor
			}
			return &domainfeed.RecommendFeedPage{
				Items: []*domainfeed.FeedItem{
					{ID: 701, Title: "Recommended Video 1"},
				},
				NextCursor: "eyJzY2VuZSI6InJlY29tbWVuZCIsInBhZ2UiOjIsInNlZWQiOjEyMzQ1Nn0=",
				HasMore:    true,
			}, nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Success request
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/recommend?limit=10", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp response.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataMap := resp.Data.(map[string]interface{})
	if dataMap["has_more"] != true || dataMap["next_cursor"] == "" {
		t.Fatalf("expected has_more=true and non-empty next_cursor, got: %+v", dataMap)
	}

	// 2. Invalid limit <= 0
	reqBadLimit, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/recommend?limit=-5", nil)
	wBadLimit := httptest.NewRecorder()
	engine.ServeHTTP(wBadLimit, reqBadLimit)
	if wBadLimit.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative limit, got %d", wBadLimit.Code)
	}

	// 3. Invalid cursor
	reqBadCursor, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/recommend?cursor=invalid-recommend-cursor", nil)
	wBadCursor := httptest.NewRecorder()
	engine.ServeHTTP(wBadCursor, reqBadCursor)
	if wBadCursor.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad recommend cursor, got %d", wBadCursor.Code)
	}
}


