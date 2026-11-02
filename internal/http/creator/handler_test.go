package creator_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpcreator "github.com/Tidehyc/gorangefeed/internal/http/creator"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	servicecreator "github.com/Tidehyc/gorangefeed/internal/service/creator"
	"github.com/gin-gonic/gin"
)

type mockCreatorService struct {
	saveDraftFn           func(ctx context.Context, authorID int64, req servicecreator.SaveDraftRequest) (*domaincreator.Draft, error)
	getDraftFn            func(ctx context.Context, draftID int64, authorID int64) (*domaincreator.Draft, error)
	updateDraftFn         func(ctx context.Context, draftID int64, authorID int64, req servicecreator.UpdateDraftRequest) (*domaincreator.Draft, error)
	deleteDraftFn         func(ctx context.Context, draftID int64, authorID int64) error
	listDraftsFn          func(ctx context.Context, authorID int64, cursor string, limit int) ([]*domaincreator.Draft, string, bool, error)
	publishDraftFn        func(ctx context.Context, draftID int64, authorID int64) (*domainvideo.Video, error)
	schedulePublishFn     func(ctx context.Context, authorID int64, req servicecreator.SchedulePublishRequest) (*domaincreator.ScheduledTask, error)
	listScheduledTasksFn  func(ctx context.Context, authorID int64, limit int) ([]*domaincreator.ScheduledTask, error)
	cancelScheduledTaskFn func(ctx context.Context, taskID string, authorID int64) error
}

func (m *mockCreatorService) SaveDraft(ctx context.Context, authorID int64, req servicecreator.SaveDraftRequest) (*domaincreator.Draft, error) {
	if m.saveDraftFn != nil {
		return m.saveDraftFn(ctx, authorID, req)
	}
	return &domaincreator.Draft{ID: 1, AuthorID: authorID, Title: req.Title, Status: domaincreator.DraftStatusDraft}, nil
}

func (m *mockCreatorService) GetDraft(ctx context.Context, draftID int64, authorID int64) (*domaincreator.Draft, error) {
	if m.getDraftFn != nil {
		return m.getDraftFn(ctx, draftID, authorID)
	}
	return &domaincreator.Draft{ID: draftID, AuthorID: authorID, Title: "Draft", Status: domaincreator.DraftStatusDraft}, nil
}

func (m *mockCreatorService) UpdateDraft(ctx context.Context, draftID int64, authorID int64, req servicecreator.UpdateDraftRequest) (*domaincreator.Draft, error) {
	if m.updateDraftFn != nil {
		return m.updateDraftFn(ctx, draftID, authorID, req)
	}
	return &domaincreator.Draft{ID: draftID, AuthorID: authorID, Title: req.Title, UpdatedAt: time.Now()}, nil
}

func (m *mockCreatorService) DeleteDraft(ctx context.Context, draftID int64, authorID int64) error {
	if m.deleteDraftFn != nil {
		return m.deleteDraftFn(ctx, draftID, authorID)
	}
	return nil
}

func (m *mockCreatorService) ListDrafts(ctx context.Context, authorID int64, cursor string, limit int) ([]*domaincreator.Draft, string, bool, error) {
	if m.listDraftsFn != nil {
		return m.listDraftsFn(ctx, authorID, cursor, limit)
	}
	return []*domaincreator.Draft{{ID: 1, AuthorID: authorID, Title: "Draft 1"}}, "", false, nil
}

func (m *mockCreatorService) PublishDraft(ctx context.Context, draftID int64, authorID int64) (*domainvideo.Video, error) {
	if m.publishDraftFn != nil {
		return m.publishDraftFn(ctx, draftID, authorID)
	}
	return &domainvideo.Video{ID: 100, AuthorID: authorID, Title: "Published Video", PublishSeq: 1, Status: domainvideo.VideoStatusPublished}, nil
}

func (m *mockCreatorService) SchedulePublish(ctx context.Context, authorID int64, req servicecreator.SchedulePublishRequest) (*domaincreator.ScheduledTask, error) {
	if m.schedulePublishFn != nil {
		return m.schedulePublishFn(ctx, authorID, req)
	}
	return &domaincreator.ScheduledTask{TaskID: "task-123", AuthorID: authorID, Title: req.Title, ScheduledAt: req.ScheduledAt, Status: domaincreator.ScheduledStatusScheduled}, nil
}

func (m *mockCreatorService) ListScheduledTasks(ctx context.Context, authorID int64, limit int) ([]*domaincreator.ScheduledTask, error) {
	if m.listScheduledTasksFn != nil {
		return m.listScheduledTasksFn(ctx, authorID, limit)
	}
	return []*domaincreator.ScheduledTask{{TaskID: "task-123", AuthorID: authorID, Title: "Task"}}, nil
}

func (m *mockCreatorService) CancelScheduledTask(ctx context.Context, taskID string, authorID int64) error {
	if m.cancelScheduledTaskFn != nil {
		return m.cancelScheduledTaskFn(ctx, taskID, authorID)
	}
	return nil
}

func setupTestRouter(svc servicecreator.Service, authorID int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := httpcreator.NewHandler(svc)

	// Injected mock auth middleware
	authMiddleware := func(c *gin.Context) {
		if authorID > 0 {
			c.Set("user_id", authorID)
		}
		c.Next()
	}

	g := r.Group("/api/v1/creator", authMiddleware)
	g.POST("/drafts", h.SaveDraft)
	g.GET("/drafts", h.ListDrafts)
	g.GET("/drafts/:id", h.GetDraft)
	g.PUT("/drafts/:id", h.UpdateDraft)
	g.DELETE("/drafts/:id", h.DeleteDraft)
	g.POST("/drafts/:id/publish", h.PublishDraft)
	g.POST("/publish-scheduled", h.SchedulePublish)
	g.GET("/scheduled-tasks", h.ListScheduledTasks)
	g.DELETE("/scheduled-tasks/:id", h.CancelScheduledTask)

	return r
}

func TestHandler_DraftEndpoints(t *testing.T) {
	mockSvc := &mockCreatorService{}
	r := setupTestRouter(mockSvc, 101)

	// 1. POST /api/v1/creator/drafts
	body, _ := json.Marshal(httpcreator.SaveDraftRequestPayload{
		Title: "New Draft",
	})
	req := httptest.NewRequest("POST", "/api/v1/creator/drafts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	// 2. GET /api/v1/creator/drafts/1
	req = httptest.NewRequest("GET", "/api/v1/creator/drafts/1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 3. PUT /api/v1/creator/drafts/1
	updateBody, _ := json.Marshal(httpcreator.UpdateDraftRequestPayload{
		Title: "Updated Title",
	})
	req = httptest.NewRequest("PUT", "/api/v1/creator/drafts/1", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 4. GET /api/v1/creator/drafts
	req = httptest.NewRequest("GET", "/api/v1/creator/drafts?limit=5", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 5. POST /api/v1/creator/drafts/1/publish
	req = httptest.NewRequest("POST", "/api/v1/creator/drafts/1/publish", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}

	// 6. DELETE /api/v1/creator/drafts/1
	req = httptest.NewRequest("DELETE", "/api/v1/creator/drafts/1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
}

func TestHandler_ScheduledPublishEndpoints(t *testing.T) {
	mockSvc := &mockCreatorService{}
	r := setupTestRouter(mockSvc, 102)

	// 1. POST /api/v1/creator/publish-scheduled
	schedTime := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	body, _ := json.Marshal(httpcreator.SchedulePublishRequestPayload{
		Title:       "Scheduled Video",
		PlayURL:     "http://example.com/play.mp4",
		ScheduledAt: schedTime,
	})
	req := httptest.NewRequest("POST", "/api/v1/creator/publish-scheduled", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d, body: %s", w.Code, w.Body.String())
	}

	// 2. GET /api/v1/creator/scheduled-tasks
	req = httptest.NewRequest("GET", "/api/v1/creator/scheduled-tasks", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// 3. DELETE /api/v1/creator/scheduled-tasks/:id
	req = httptest.NewRequest("DELETE", "/api/v1/creator/scheduled-tasks/task-123", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var env response.Response
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if env.Error != nil {
		t.Fatalf("expected error nil, got %+v", env.Error)
	}
}
