package asset_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpasset "github.com/Tidehyc/gorangefeed/internal/http/asset"
	"github.com/gin-gonic/gin"
)

type mockAssetService struct {
	favoriteVideoFn func(ctx context.Context, userID, videoID int64, folderID *int64) (*domainasset.FavoriteItem, bool, error)
	unfavoriteVideoFn func(ctx context.Context, userID, videoID int64, folderID *int64) (bool, error)
	createFolderFn func(ctx context.Context, userID int64, name, description string, isPublic bool) (*domainasset.FavoriteFolder, error)
	listFoldersFn func(ctx context.Context, userID int64) ([]*domainasset.FavoriteFolder, error)
	deleteFolderFn func(ctx context.Context, userID, folderID int64) error
	listFolderItemsFn func(ctx context.Context, viewerID, folderID int64, cursor int64, limit int) ([]*domainasset.FavoriteItem, int64, bool, error)
	isFavoritedFn func(ctx context.Context, userID, videoID int64) (bool, error)
	batchIsFavoritedFn func(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error)
	recordWatchProgressFn func(ctx context.Context, userID, videoID int64, positionMs, durationMs int) (*domainasset.WatchHistory, error)
	getWatchProgressFn func(ctx context.Context, userID, videoID int64) (*domainasset.WatchHistory, error)
	listWatchHistoryFn func(ctx context.Context, userID int64, limit int) ([]*domainasset.WatchHistory, error)
	clearWatchHistoryFn func(ctx context.Context, userID int64) error
}

func (m *mockAssetService) FavoriteVideo(ctx context.Context, userID, videoID int64, folderID *int64) (*domainasset.FavoriteItem, bool, error) {
	if m.favoriteVideoFn != nil {
		return m.favoriteVideoFn(ctx, userID, videoID, folderID)
	}
	fid := int64(1)
	if folderID != nil {
		fid = *folderID
	}
	return &domainasset.FavoriteItem{ID: 10, FavoriteID: fid, VideoID: videoID, UserID: userID}, true, nil
}

func (m *mockAssetService) UnfavoriteVideo(ctx context.Context, userID, videoID int64, folderID *int64) (bool, error) {
	if m.unfavoriteVideoFn != nil {
		return m.unfavoriteVideoFn(ctx, userID, videoID, folderID)
	}
	return true, nil
}

func (m *mockAssetService) CreateFolder(ctx context.Context, userID int64, name, description string, isPublic bool) (*domainasset.FavoriteFolder, error) {
	if m.createFolderFn != nil {
		return m.createFolderFn(ctx, userID, name, description, isPublic)
	}
	return &domainasset.FavoriteFolder{ID: 2, UserID: userID, Name: name, Description: description, IsPublic: isPublic}, nil
}

func (m *mockAssetService) ListFolders(ctx context.Context, userID int64) ([]*domainasset.FavoriteFolder, error) {
	if m.listFoldersFn != nil {
		return m.listFoldersFn(ctx, userID)
	}
	return []*domainasset.FavoriteFolder{{ID: 1, UserID: userID, Name: "默认收藏夹", IsDefault: true}}, nil
}

func (m *mockAssetService) DeleteFolder(ctx context.Context, userID, folderID int64) error {
	if m.deleteFolderFn != nil {
		return m.deleteFolderFn(ctx, userID, folderID)
	}
	return nil
}

func (m *mockAssetService) ListFolderItems(ctx context.Context, viewerID, folderID int64, cursor int64, limit int) ([]*domainasset.FavoriteItem, int64, bool, error) {
	if m.listFolderItemsFn != nil {
		return m.listFolderItemsFn(ctx, viewerID, folderID, cursor, limit)
	}
	return []*domainasset.FavoriteItem{{ID: 10, FavoriteID: folderID, VideoID: 100, UserID: viewerID}}, 0, false, nil
}

func (m *mockAssetService) IsFavorited(ctx context.Context, userID, videoID int64) (bool, error) {
	if m.isFavoritedFn != nil {
		return m.isFavoritedFn(ctx, userID, videoID)
	}
	return true, nil
}

func (m *mockAssetService) BatchIsFavorited(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error) {
	if m.batchIsFavoritedFn != nil {
		return m.batchIsFavoritedFn(ctx, userID, videoIDs)
	}
	return map[int64]bool{100: true}, nil
}

func (m *mockAssetService) RecordWatchProgress(ctx context.Context, userID, videoID int64, positionMs, durationMs int) (*domainasset.WatchHistory, error) {
	if m.recordWatchProgressFn != nil {
		return m.recordWatchProgressFn(ctx, userID, videoID, positionMs, durationMs)
	}
	return &domainasset.WatchHistory{ID: 1, UserID: userID, VideoID: videoID, LastPositionMs: positionMs, DurationMs: durationMs}, nil
}

func (m *mockAssetService) GetWatchProgress(ctx context.Context, userID, videoID int64) (*domainasset.WatchHistory, error) {
	if m.getWatchProgressFn != nil {
		return m.getWatchProgressFn(ctx, userID, videoID)
	}
	return &domainasset.WatchHistory{ID: 1, UserID: userID, VideoID: videoID, LastPositionMs: 5000, DurationMs: 10000}, nil
}

func (m *mockAssetService) ListWatchHistory(ctx context.Context, userID int64, limit int) ([]*domainasset.WatchHistory, error) {
	if m.listWatchHistoryFn != nil {
		return m.listWatchHistoryFn(ctx, userID, limit)
	}
	return []*domainasset.WatchHistory{{ID: 1, UserID: userID, VideoID: 100, LastPositionMs: 5000, DurationMs: 10000}}, nil
}

func (m *mockAssetService) ClearWatchHistory(ctx context.Context, userID int64) error {
	if m.clearWatchHistoryFn != nil {
		return m.clearWatchHistoryFn(ctx, userID)
	}
	return nil
}

func setupTestRouter(svc httpasset.Service, userID int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := httpasset.NewHandler(svc)

	authMiddleware := func(c *gin.Context) {
		if userID > 0 {
			c.Set("user_id", userID)
		}
		c.Next()
	}

	v1 := r.Group("/api/v1", authMiddleware)
	v1.POST("/videos/:id/favorite", h.FavoriteVideo)
	v1.DELETE("/videos/:id/favorite", h.UnfavoriteVideo)
	v1.GET("/videos/:id/history", h.GetWatchProgress)
	v1.POST("/videos/:id/history", h.RecordWatchProgress)

	v1.GET("/user/favorites", h.ListFolders)
	v1.POST("/user/favorites", h.CreateFolder)
	v1.DELETE("/user/favorites/:id", h.DeleteFolder)
	v1.GET("/user/favorites/:id/items", h.ListFolderItems)

	v1.GET("/user/history", h.ListHistory)
	v1.POST("/user/history", h.RecordHistory)
	v1.DELETE("/user/history", h.ClearHistory)

	return r
}

func TestHandler_FavoriteEndpoints(t *testing.T) {
	mockSvc := &mockAssetService{}
	r := setupTestRouter(mockSvc, 100)

	// 1. Favorite video with default folder
	body, _ := json.Marshal(map[string]any{})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/200/favorite", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var res struct {
		Code int                          `json:"code"`
		Data httpasset.FavoriteVideoResponse `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Data.VideoID != 200 || res.Data.FavoriteID != 1 {
		t.Fatalf("unexpected favorite response: %+v", res.Data)
	}

	// 2. Favorite video with specific folder
	folderID := int64(5)
	favReqBody, _ := json.Marshal(httpasset.FavoriteVideoRequest{FolderID: &folderID})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/videos/200/favorite", bytes.NewReader(favReqBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Data.FavoriteID != 5 {
		t.Fatalf("expected folder ID 5, got %d", res.Data.FavoriteID)
	}

	// 3. Unfavorite video
	req, _ = http.NewRequest(http.MethodDelete, "/api/v1/videos/200/favorite", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for unfavorite, got %d", w.Code)
	}

	// 4. Invalid video ID
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/videos/abc/favorite", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid video ID, got %d", w.Code)
	}

	// 5. Video not found
	mockSvc.favoriteVideoFn = func(ctx context.Context, userID, videoID int64, folderID *int64) (*domainasset.FavoriteItem, bool, error) {
		return nil, false, domainvideo.ErrVideoNotFound
	}
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/videos/9999/favorite", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for video not found, got %d", w.Code)
	}
}

func TestHandler_FolderEndpoints(t *testing.T) {
	mockSvc := &mockAssetService{}
	r := setupTestRouter(mockSvc, 100)

	// 1. Create folder
	createBody, _ := json.Marshal(map[string]any{
		"name":        "Coding Lessons",
		"description": "Golang tutorials",
		"is_public":   true,
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/user/favorites", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", w.Code)
	}

	// 2. Create folder validation error
	invalidBody, _ := json.Marshal(map[string]any{"name": ""})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/user/favorites", bytes.NewReader(invalidBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}

	// 3. List folders
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/user/favorites", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 4. Delete custom folder
	req, _ = http.NewRequest(http.MethodDelete, "/api/v1/user/favorites/2", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 5. Delete default folder fails
	mockSvc.deleteFolderFn = func(ctx context.Context, userID, folderID int64) error {
		return domainasset.ErrCannotDeleteDefaultFolder
	}
	req, _ = http.NewRequest(http.MethodDelete, "/api/v1/user/favorites/1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for deleting default folder, got %d", w.Code)
	}

	// 6. List folder items
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/user/favorites/2/items?limit=10", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 7. List private folder items as non-owner (Forbidden)
	mockSvc.listFolderItemsFn = func(ctx context.Context, viewerID, folderID int64, cursor int64, limit int) ([]*domainasset.FavoriteItem, int64, bool, error) {
		return nil, 0, false, domainasset.ErrFolderAccessDenied
	}
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/user/favorites/99/items", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for private folder, got %d", w.Code)
	}
}

func TestHandler_HistoryEndpoints(t *testing.T) {
	mockSvc := &mockAssetService{}
	r := setupTestRouter(mockSvc, 100)

	// 1. Record progress via /api/v1/videos/:id/history
	progBody, _ := json.Marshal(map[string]any{
		"last_position_ms": 12000,
		"duration_ms":      30000,
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/100/history", bytes.NewReader(progBody))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 2. Record progress via /api/v1/user/history
	userHistBody, _ := json.Marshal(map[string]any{
		"video_id":         100,
		"last_position_ms": 15000,
		"duration_ms":      30000,
	})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/user/history", bytes.NewReader(userHistBody))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 3. Get watch progress (breakpoint resume)
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/videos/100/history", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 4. Get watch progress when nil -> fallback to 0
	mockSvc.getWatchProgressFn = func(ctx context.Context, userID, videoID int64) (*domainasset.WatchHistory, error) {
		return nil, nil
	}
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/videos/200/history", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 5. List history
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/user/history?limit=10", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 6. Clear history
	req, _ = http.NewRequest(http.MethodDelete, "/api/v1/user/history", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
}

func TestHandler_AuthenticationRequirement(t *testing.T) {
	mockSvc := &mockAssetService{}
	unauthRouter := setupTestRouter(mockSvc, 0) // unauthenticated (userID = 0)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/videos/100/favorite"},
		{http.MethodDelete, "/api/v1/videos/100/favorite"},
		{http.MethodGet, "/api/v1/user/favorites"},
		{http.MethodPost, "/api/v1/user/favorites"},
		{http.MethodDelete, "/api/v1/user/favorites/1"},
		{http.MethodGet, "/api/v1/user/history"},
		{http.MethodPost, "/api/v1/user/history"},
		{http.MethodDelete, "/api/v1/user/history"},
	}

	for _, ep := range endpoints {
		req, _ := http.NewRequest(ep.method, ep.path, nil)
		w := httptest.NewRecorder()
		unauthRouter.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for %s %s, got %d", ep.method, ep.path, w.Code)
		}
	}
}
