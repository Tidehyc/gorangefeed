package upload_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	httpupload "github.com/Tidehyc/gorangefeed/internal/http/upload"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	serviceupload "github.com/Tidehyc/gorangefeed/internal/service/upload"
	"github.com/gin-gonic/gin"
)

type mockUploadService struct {
	initFn     func(ctx context.Context, userID int64, filename string, fileSize int64, chunkSize int, totalChunks int, fileHash string) (*serviceupload.InitResult, error)
	uploadFn   func(ctx context.Context, userID int64, uploadID string, chunkIndex int, chunkHash string, src io.Reader) error
	statusFn   func(ctx context.Context, userID int64, uploadID string) (*serviceupload.StatusResult, error)
	completeFn func(ctx context.Context, userID int64, uploadID string) (*serviceupload.CompleteResult, error)
}

func (m *mockUploadService) InitSession(ctx context.Context, userID int64, filename string, fileSize int64, chunkSize int, totalChunks int, fileHash string) (*serviceupload.InitResult, error) {
	if m.initFn != nil {
		return m.initFn(ctx, userID, filename, fileSize, chunkSize, totalChunks, fileHash)
	}
	return &serviceupload.InitResult{UploadID: "test-up-1", TotalChunks: 2}, nil
}

func (m *mockUploadService) UploadChunk(ctx context.Context, userID int64, uploadID string, chunkIndex int, chunkHash string, src io.Reader) error {
	if m.uploadFn != nil {
		return m.uploadFn(ctx, userID, uploadID, chunkIndex, chunkHash, src)
	}
	return nil
}

func (m *mockUploadService) GetStatus(ctx context.Context, userID int64, uploadID string) (*serviceupload.StatusResult, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, userID, uploadID)
	}
	return &serviceupload.StatusResult{UploadID: uploadID, TotalChunks: 2, UploadedChunks: []int{0}}, nil
}

func (m *mockUploadService) Complete(ctx context.Context, userID int64, uploadID string) (*serviceupload.CompleteResult, error) {
	if m.completeFn != nil {
		return m.completeFn(ctx, userID, uploadID)
	}
	return &serviceupload.CompleteResult{UploadID: uploadID, PlayURL: "/media/videos/1/test.mp4", Size: 1024}, nil
}

func setupTestRouter(svc serviceupload.Service, userID int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if userID > 0 {
			c.Set(middleware.ContextKeyUserID, userID)
		}
		c.Next()
	})
	h := httpupload.NewHandler(svc)
	r.POST("/api/v1/videos/chunk/init", h.InitSession)
	r.POST("/api/v1/videos/chunk/upload", h.UploadChunk)
	r.GET("/api/v1/videos/chunk/status", h.GetStatus)
	r.POST("/api/v1/videos/chunk/complete", h.Complete)
	return r
}

func TestHandler_InitSession(t *testing.T) {
	svc := &mockUploadService{
		initFn: func(ctx context.Context, userID int64, filename string, fileSize int64, chunkSize int, totalChunks int, fileHash string) (*serviceupload.InitResult, error) {
			return &serviceupload.InitResult{
				UploadID:       "sess-123",
				UploadedChunks: []int{0},
				TotalChunks:    3,
				IsResumed:      true,
			}, nil
		},
	}
	r := setupTestRouter(svc, 101)

	body, _ := json.Marshal(httpupload.InitUploadRequest{
		FileName: "test.mp4",
		FileSize: 15 * 1024 * 1024,
		FileHash: "hash-xyz",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/videos/chunk/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_UploadChunk(t *testing.T) {
	svc := &mockUploadService{}
	r := setupTestRouter(svc, 101)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("upload_id", "sess-123")
	_ = writer.WriteField("chunk_index", "0")
	_ = writer.WriteField("chunk_hash", "hash-0")
	part, _ := writer.CreateFormFile("file", "chunk0.bin")
	_, _ = part.Write([]byte("chunk content"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/videos/chunk/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetStatus(t *testing.T) {
	svc := &mockUploadService{}
	r := setupTestRouter(svc, 101)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/chunk/status?upload_id=sess-123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Complete_Conflict(t *testing.T) {
	svc := &mockUploadService{
		completeFn: func(ctx context.Context, userID int64, uploadID string) (*serviceupload.CompleteResult, error) {
			return nil, domainupload.ErrMergeConflict
		},
	}
	r := setupTestRouter(svc, 101)

	body, _ := json.Marshal(httpupload.CompleteUploadRequest{UploadID: "sess-123"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/videos/chunk/complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
	}
}
