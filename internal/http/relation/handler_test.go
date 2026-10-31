package relation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	httprelation "github.com/Tidehyc/gorangefeed/internal/http/relation"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

type mockRelationService struct {
	followFn         func(ctx context.Context, followerID, vloggerID int64) error
	unfollowFn       func(ctx context.Context, followerID, vloggerID int64) error
	getRelationFn    func(ctx context.Context, userID, targetID int64) (*domainrelation.Friendship, error)
	listFollowingsFn func(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error)
	listFollowersFn  func(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error)
	getStatsFn       func(ctx context.Context, userID int64) (*domainrelation.RelationStats, error)
}

func (m *mockRelationService) Follow(ctx context.Context, followerID, vloggerID int64) error {
	if m.followFn != nil {
		return m.followFn(ctx, followerID, vloggerID)
	}
	return nil
}

func (m *mockRelationService) Unfollow(ctx context.Context, followerID, vloggerID int64) error {
	if m.unfollowFn != nil {
		return m.unfollowFn(ctx, followerID, vloggerID)
	}
	return nil
}

func (m *mockRelationService) GetRelation(ctx context.Context, userID, targetID int64) (*domainrelation.Friendship, error) {
	if m.getRelationFn != nil {
		return m.getRelationFn(ctx, userID, targetID)
	}
	return &domainrelation.Friendship{
		IsFollowing: true,
		IsFollower:  true,
		IsFriend:    true,
	}, nil
}

func (m *mockRelationService) ListFollowings(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error) {
	if m.listFollowingsFn != nil {
		return m.listFollowingsFn(ctx, userID, viewerID, cursor, limit)
	}
	return nil, 0, false, nil
}

func (m *mockRelationService) ListFollowers(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error) {
	if m.listFollowersFn != nil {
		return m.listFollowersFn(ctx, userID, viewerID, cursor, limit)
	}
	return nil, 0, false, nil
}

func (m *mockRelationService) GetStats(ctx context.Context, userID int64) (*domainrelation.RelationStats, error) {
	if m.getStatsFn != nil {
		return m.getStatsFn(ctx, userID)
	}
	return &domainrelation.RelationStats{UserID: userID}, nil
}

func setupTestEngine(svc httprelation.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := httprelation.NewHandler(svc)

	authMiddleware := func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "Bearer valid" {
			c.Set("user_id", int64(1001))
		}
		c.Next()
	}

	r.Use(authMiddleware)

	r.POST("/api/v1/users/:userId/follow", handler.Follow)
	r.DELETE("/api/v1/users/:userId/follow", handler.Unfollow)
	r.GET("/api/v1/users/:userId/followings", handler.ListFollowings)
	r.GET("/api/v1/users/:userId/followers", handler.ListFollowers)
	r.GET("/api/v1/users/:userId/relation", handler.GetRelation)

	return r
}

func TestHandler_FollowAndUnfollow(t *testing.T) {
	mockSvc := &mockRelationService{
		followFn: func(ctx context.Context, followerID, vloggerID int64) error {
			if followerID == vloggerID {
				return domainrelation.ErrSelfFollow
			}
			if vloggerID == 999 {
				return domainrelation.ErrTargetUserNotFound
			}
			return nil
		},
		unfollowFn: func(ctx context.Context, followerID, vloggerID int64) error {
			if followerID == vloggerID {
				return domainrelation.ErrSelfUnfollow
			}
			return nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Follow with auth -> 200 OK
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/users/500/follow", nil)
	req.Header.Set("Authorization", "Bearer valid")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 2. Follow without auth -> 401
	reqUnauth, _ := http.NewRequest(http.MethodPost, "/api/v1/users/500/follow", nil)
	wUnauth := httptest.NewRecorder()
	engine.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", wUnauth.Code)
	}

	// 3. Self-follow -> 400 Bad Request
	reqSelf, _ := http.NewRequest(http.MethodPost, "/api/v1/users/1001/follow", nil)
	reqSelf.Header.Set("Authorization", "Bearer valid")
	wSelf := httptest.NewRecorder()
	engine.ServeHTTP(wSelf, reqSelf)
	if wSelf.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for self-follow, got %d", wSelf.Code)
	}

	// 4. Follow non-existent user -> 404
	reqNotFound, _ := http.NewRequest(http.MethodPost, "/api/v1/users/999/follow", nil)
	reqNotFound.Header.Set("Authorization", "Bearer valid")
	wNotFound := httptest.NewRecorder()
	engine.ServeHTTP(wNotFound, reqNotFound)
	if wNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", wNotFound.Code)
	}

	// 5. Unfollow with auth -> 200 OK
	reqUnfollow, _ := http.NewRequest(http.MethodDelete, "/api/v1/users/500/follow", nil)
	reqUnfollow.Header.Set("Authorization", "Bearer valid")
	wUnfollow := httptest.NewRecorder()
	engine.ServeHTTP(wUnfollow, reqUnfollow)
	if wUnfollow.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wUnfollow.Code)
	}

	// 6. Bad userId -> 400
	reqBad, _ := http.NewRequest(http.MethodPost, "/api/v1/users/abc/follow", nil)
	reqBad.Header.Set("Authorization", "Bearer valid")
	wBad := httptest.NewRecorder()
	engine.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", wBad.Code)
	}
}

func TestHandler_ListFollowingsAndFollowers(t *testing.T) {
	mockSvc := &mockRelationService{
		listFollowingsFn: func(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error) {
			return []*domainrelation.UserProfile{
				{ID: 501, Username: "vlogger1"},
			}, 501, true, nil
		},
		listFollowersFn: func(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error) {
			return []*domainrelation.UserProfile{
				{ID: 601, Username: "follower1"},
			}, 601, false, nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. List followings -> 200 OK
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/users/500/followings?cursor=0&limit=10", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp response.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataMap := resp.Data.(map[string]interface{})
	items := dataMap["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	// 2. List followers -> 200 OK
	reqFollowers, _ := http.NewRequest(http.MethodGet, "/api/v1/users/500/followers?cursor=0&limit=10", nil)
	wFollowers := httptest.NewRecorder()
	engine.ServeHTTP(wFollowers, reqFollowers)
	if wFollowers.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wFollowers.Code)
	}

	// 3. Bad cursor -> 400
	reqBad, _ := http.NewRequest(http.MethodGet, "/api/v1/users/500/followings?cursor=-1", nil)
	wBad := httptest.NewRecorder()
	engine.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", wBad.Code)
	}
}

func TestHandler_GetRelation(t *testing.T) {
	mockSvc := &mockRelationService{
		getRelationFn: func(ctx context.Context, userID, targetID int64) (*domainrelation.Friendship, error) {
			return &domainrelation.Friendship{
				IsFollowing: true,
				IsFollower:  true,
				IsFriend:    true,
			}, nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Authorized -> 200 OK with friendship
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/users/500/relation", nil)
	req.Header.Set("Authorization", "Bearer valid")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp response.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	dataMap := resp.Data.(map[string]interface{})
	if dataMap["is_friend"] != true || dataMap["is_following"] != true || dataMap["is_follower"] != true {
		t.Fatalf("unexpected friendship data: %+v", dataMap)
	}

	// 2. Unauthorized -> 401
	reqUnauth, _ := http.NewRequest(http.MethodGet, "/api/v1/users/500/relation", nil)
	wUnauth := httptest.NewRecorder()
	engine.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", wUnauth.Code)
	}
}
