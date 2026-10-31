package interaction_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpinteraction "github.com/Tidehyc/gorangefeed/internal/http/interaction"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

type mockInteractionService struct {
	likeVideoFn         func(ctx context.Context, userID, videoID int64) error
	unlikeVideoFn       func(ctx context.Context, userID, videoID int64) error
	listMyLikedVideosFn func(ctx context.Context, userID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error)
	createCommentFn     func(ctx context.Context, userID, videoID, parentID int64, content, idempotencyKey string) (*domaininteraction.CommentDetail, bool, error)
	deleteCommentFn     func(ctx context.Context, userID, commentID int64) error
	listCommentsFn      func(ctx context.Context, viewerID, videoID int64, sort string, cursorTime int64, cursorLikeCount, cursorID int64, limit int) ([]*domaininteraction.CommentDetail, int64, int64, bool, error)
	likeCommentFn       func(ctx context.Context, userID, commentID int64) error
	unlikeCommentFn     func(ctx context.Context, userID, commentID int64) error
}

func (m *mockInteractionService) LikeVideo(ctx context.Context, userID, videoID int64) error {
	if m.likeVideoFn != nil {
		return m.likeVideoFn(ctx, userID, videoID)
	}
	return nil
}

func (m *mockInteractionService) UnlikeVideo(ctx context.Context, userID, videoID int64) error {
	if m.unlikeVideoFn != nil {
		return m.unlikeVideoFn(ctx, userID, videoID)
	}
	return nil
}

func (m *mockInteractionService) ListMyLikedVideos(ctx context.Context, userID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error) {
	if m.listMyLikedVideosFn != nil {
		return m.listMyLikedVideosFn(ctx, userID, cursor, limit)
	}
	return nil, 0, false, nil
}

func (m *mockInteractionService) CreateComment(ctx context.Context, userID, videoID, parentID int64, content, idempotencyKey string) (*domaininteraction.CommentDetail, bool, error) {
	if m.createCommentFn != nil {
		return m.createCommentFn(ctx, userID, videoID, parentID, content, idempotencyKey)
	}
	return &domaininteraction.CommentDetail{
		Comment: &domaininteraction.Comment{ID: 1, Content: content},
	}, true, nil
}

func (m *mockInteractionService) DeleteComment(ctx context.Context, userID, commentID int64) error {
	if m.deleteCommentFn != nil {
		return m.deleteCommentFn(ctx, userID, commentID)
	}
	return nil
}

func (m *mockInteractionService) ListComments(ctx context.Context, viewerID, videoID int64, sort string, cursorTime int64, cursorLikeCount, cursorID int64, limit int) ([]*domaininteraction.CommentDetail, int64, int64, bool, error) {
	if m.listCommentsFn != nil {
		return m.listCommentsFn(ctx, viewerID, videoID, sort, cursorTime, cursorLikeCount, cursorID, limit)
	}
	return nil, 0, 0, false, nil
}

func (m *mockInteractionService) LikeComment(ctx context.Context, userID, commentID int64) error {
	if m.likeCommentFn != nil {
		return m.likeCommentFn(ctx, userID, commentID)
	}
	return nil
}

func (m *mockInteractionService) UnlikeComment(ctx context.Context, userID, commentID int64) error {
	if m.unlikeCommentFn != nil {
		return m.unlikeCommentFn(ctx, userID, commentID)
	}
	return nil
}

func setupTestEngine(svc httpinteraction.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	handler := httpinteraction.NewHandler(svc)

	authMiddleware := func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "Bearer valid" {
			c.Set("user_id", int64(1001))
		}
		c.Next()
	}

	r.Use(authMiddleware)

	r.POST("/api/v1/videos/:videoId/like", handler.LikeVideo)
	r.DELETE("/api/v1/videos/:videoId/like", handler.UnlikeVideo)
	r.POST("/api/v1/videos/:videoId/comments", handler.CreateComment)
	r.GET("/api/v1/videos/:videoId/comments", handler.ListComments)

	r.DELETE("/api/v1/comments/:commentId", handler.DeleteComment)
	r.POST("/api/v1/comments/:commentId/like", handler.LikeComment)
	r.DELETE("/api/v1/comments/:commentId/like", handler.UnlikeComment)

	r.GET("/api/v1/users/me/likes", handler.ListMyLikedVideos)

	return r
}

func TestHandler_LikeAndUnlikeVideo(t *testing.T) {
	mockSvc := &mockInteractionService{
		likeVideoFn: func(ctx context.Context, userID, videoID int64) error {
			if videoID == 999 {
				return domaininteraction.ErrVideoNotFound
			}
			return nil
		},
		unlikeVideoFn: func(ctx context.Context, userID, videoID int64) error {
			return nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Like with auth -> 200 OK
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/101/like", nil)
	req.Header.Set("Authorization", "Bearer valid")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 2. Like without auth -> 401
	reqUnauth, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/101/like", nil)
	wUnauth := httptest.NewRecorder()
	engine.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", wUnauth.Code)
	}

	// 3. Like non-existent video -> 404
	reqNotFound, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/999/like", nil)
	reqNotFound.Header.Set("Authorization", "Bearer valid")
	wNotFound := httptest.NewRecorder()
	engine.ServeHTTP(wNotFound, reqNotFound)
	if wNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", wNotFound.Code)
	}

	// 4. Unlike with auth -> 200 OK
	reqUnlike, _ := http.NewRequest(http.MethodDelete, "/api/v1/videos/101/like", nil)
	reqUnlike.Header.Set("Authorization", "Bearer valid")
	wUnlike := httptest.NewRecorder()
	engine.ServeHTTP(wUnlike, reqUnlike)
	if wUnlike.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wUnlike.Code)
	}

	// 5. Bad videoId -> 400
	reqBad, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/abc/like", nil)
	reqBad.Header.Set("Authorization", "Bearer valid")
	wBad := httptest.NewRecorder()
	engine.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", wBad.Code)
	}
}

func TestHandler_ListMyLikedVideos(t *testing.T) {
	mockSvc := &mockInteractionService{
		listMyLikedVideosFn: func(ctx context.Context, userID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error) {
			return []*domainvideo.VideoDetail{
				{
					Video: &domainvideo.Video{ID: 10, Title: "Liked Video 1"},
				},
			}, 10, true, nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Authorized -> 200 OK
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/users/me/likes?cursor=0&limit=10", nil)
	req.Header.Set("Authorization", "Bearer valid")
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
	if dataMap["next_cursor"] != float64(10) {
		t.Fatalf("expected next_cursor=10, got %v", dataMap["next_cursor"])
	}

	// 2. Unauthorized -> 401
	reqUnauth, _ := http.NewRequest(http.MethodGet, "/api/v1/users/me/likes", nil)
	wUnauth := httptest.NewRecorder()
	engine.ServeHTTP(wUnauth, reqUnauth)
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", wUnauth.Code)
	}

	// 3. Bad cursor -> 400
	reqBad, _ := http.NewRequest(http.MethodGet, "/api/v1/users/me/likes?cursor=-1", nil)
	reqBad.Header.Set("Authorization", "Bearer valid")
	wBad := httptest.NewRecorder()
	engine.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", wBad.Code)
	}
}

func TestHandler_Comments(t *testing.T) {
	mockSvc := &mockInteractionService{
		createCommentFn: func(ctx context.Context, userID, videoID, parentID int64, content, idempotencyKey string) (*domaininteraction.CommentDetail, bool, error) {
			if content == "" {
				return nil, false, domaininteraction.ErrEmptyCommentContent
			}
			return &domaininteraction.CommentDetail{
				Comment: &domaininteraction.Comment{
					ID:             501,
					VideoID:        videoID,
					UserID:         userID,
					Content:        content,
					IdempotencyKey: idempotencyKey,
					CreatedAt:      time.Now(),
				},
			}, true, nil
		},
		listCommentsFn: func(ctx context.Context, viewerID, videoID int64, sort string, cursorTime int64, cursorLikeCount, cursorID int64, limit int) ([]*domaininteraction.CommentDetail, int64, int64, bool, error) {
			if sort == "hot" {
				return []*domaininteraction.CommentDetail{
					{Comment: &domaininteraction.Comment{ID: 501, LikeCount: 100}},
				}, 100, 501, true, nil
			}
			return []*domaininteraction.CommentDetail{
				{Comment: &domaininteraction.Comment{ID: 501}},
			}, 501, 0, true, nil
		},
		deleteCommentFn: func(ctx context.Context, userID, commentID int64) error {
			if commentID == 999 {
				return domaininteraction.ErrCommentNotFound
			}
			return nil
		},
		likeCommentFn: func(ctx context.Context, userID, commentID int64) error {
			return nil
		},
		unlikeCommentFn: func(ctx context.Context, userID, commentID int64) error {
			return nil
		},
	}

	engine := setupTestEngine(mockSvc)

	// 1. Create comment -> 201 Created
	body, _ := json.Marshal(httpinteraction.CreateCommentRequest{
		Content:        "Nice video!",
		ParentID:       0,
		IdempotencyKey: "idem-1",
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/101/comments", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer valid")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	// 2. Create comment with empty content -> 400 Bad Request
	bodyEmpty, _ := json.Marshal(httpinteraction.CreateCommentRequest{
		Content: "",
	})
	reqEmpty, _ := http.NewRequest(http.MethodPost, "/api/v1/videos/101/comments", bytes.NewBuffer(bodyEmpty))
	reqEmpty.Header.Set("Authorization", "Bearer valid")
	reqEmpty.Header.Set("Content-Type", "application/json")
	wEmpty := httptest.NewRecorder()
	engine.ServeHTTP(wEmpty, reqEmpty)
	if wEmpty.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", wEmpty.Code)
	}

	// 3. List comments latest -> 200 OK
	reqList, _ := http.NewRequest(http.MethodGet, "/api/v1/videos/101/comments?sort=latest&cursor=0&limit=10", nil)
	wList := httptest.NewRecorder()
	engine.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wList.Code)
	}

	// 4. List comments hot -> 200 OK
	reqHot, _ := http.NewRequest(http.MethodGet, "/api/v1/videos/101/comments?sort=hot&cursor=200_50&limit=10", nil)
	wHot := httptest.NewRecorder()
	engine.ServeHTTP(wHot, reqHot)
	if wHot.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wHot.Code)
	}

	// 5. Delete comment -> 200 OK
	reqDel, _ := http.NewRequest(http.MethodDelete, "/api/v1/comments/501", nil)
	reqDel.Header.Set("Authorization", "Bearer valid")
	wDel := httptest.NewRecorder()
	engine.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wDel.Code)
	}

	// 6. Delete comment not found -> 404
	reqDel404, _ := http.NewRequest(http.MethodDelete, "/api/v1/comments/999", nil)
	reqDel404.Header.Set("Authorization", "Bearer valid")
	wDel404 := httptest.NewRecorder()
	engine.ServeHTTP(wDel404, reqDel404)
	if wDel404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", wDel404.Code)
	}

	// 7. Like & Unlike comment -> 200 OK
	reqLikeC, _ := http.NewRequest(http.MethodPost, "/api/v1/comments/501/like", nil)
	reqLikeC.Header.Set("Authorization", "Bearer valid")
	wLikeC := httptest.NewRecorder()
	engine.ServeHTTP(wLikeC, reqLikeC)
	if wLikeC.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wLikeC.Code)
	}

	reqUnlikeC, _ := http.NewRequest(http.MethodDelete, "/api/v1/comments/501/like", nil)
	reqUnlikeC.Header.Set("Authorization", "Bearer valid")
	wUnlikeC := httptest.NewRecorder()
	engine.ServeHTTP(wUnlikeC, reqUnlikeC)
	if wUnlikeC.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wUnlikeC.Code)
	}
}
