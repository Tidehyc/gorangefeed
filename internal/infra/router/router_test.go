package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRouter_ProbesAndMetrics(t *testing.T) {
	deps := DependencyChecks{
		CheckMySQL: func(ctx context.Context) error { return nil },
		CheckRedis: func(ctx context.Context) error { return nil },
		CheckRabbitMQ: func() error { return nil },
	}

	r := NewRouter(deps)

	// Test 1: RequestID header propagation
	t.Run("request_id header present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		reqID := w.Header().Get(middleware.HeaderRequestID)
		if reqID == "" {
			t.Error("expected X-Request-ID header to be set")
		}
	})

	// Test 2: Readyz 200 when all dependencies OK
	t.Run("readyz healthy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for readyz, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), `"status":"ready"`) {
			t.Errorf("expected body to contain ready status, got: %s", w.Body.String())
		}
	})

	// Test 3: Stop Redis scenario -> readyz=503, healthz=200
	t.Run("redis down: readyz=503, healthz=200", func(t *testing.T) {
		degradedDeps := DependencyChecks{
			CheckMySQL: func(ctx context.Context) error { return nil },
			CheckRedis: func(ctx context.Context) error { return errors.New("connection refused to redis:6380") },
			CheckRabbitMQ: func() error { return nil },
		}
		degradedRouter := NewRouter(degradedDeps)

		// healthz should still return 200
		healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		healthW := httptest.NewRecorder()
		degradedRouter.ServeHTTP(healthW, healthReq)
		if healthW.Code != http.StatusOK {
			t.Fatalf("expected healthz to remain 200, got %d", healthW.Code)
		}

		// readyz should return 503
		readyReq := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		readyW := httptest.NewRecorder()
		degradedRouter.ServeHTTP(readyW, readyReq)
		if readyW.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected readyz to return 503, got %d", readyW.Code)
		}
		if !strings.Contains(readyW.Body.String(), `"status":"unavailable"`) {
			t.Errorf("expected body to indicate unavailable status, got: %s", readyW.Body.String())
		}
	})

	// Test 4: Metrics endpoint
	t.Run("metrics endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for /metrics, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "http_requests_total") {
			t.Errorf("expected metrics to contain http_requests_total, got: %s", w.Body.String())
		}
	})
}

func TestRouter_MediaRangeStreaming(t *testing.T) {
	tempDir := t.TempDir()
	videoSubDir := filepath.Join(tempDir, "videos", "101", "20261026")
	if err := os.MkdirAll(videoSubDir, 0o755); err != nil {
		t.Fatalf("failed to create video subdir: %v", err)
	}

	testData := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	videoFilePath := filepath.Join(videoSubDir, "sample.mp4")
	if err := os.WriteFile(videoFilePath, testData, 0o644); err != nil {
		t.Fatalf("failed to write test video: %v", err)
	}

	r := NewRouter(DependencyChecks{})
	MountMediaRoutes(r, tempDir)

	// 1. Full request without Range
	t.Run("full file request 200 OK", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/media/videos/101/20261026/sample.mp4", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
		if w.Header().Get("Accept-Ranges") != "bytes" {
			t.Errorf("expected Accept-Ranges: bytes, got %q", w.Header().Get("Accept-Ranges"))
		}
		if w.Body.String() != string(testData) {
			t.Fatalf("expected full content, got %q", w.Body.String())
		}
	})

	// 2. Partial Range request (bytes=0-15) -> 206 Partial Content
	t.Run("partial range request 206", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/media/videos/101/20261026/sample.mp4", nil)
		req.Header.Set("Range", "bytes=0-15")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content, got %d", w.Code)
		}
		if got := w.Header().Get("Content-Range"); got != "bytes 0-15/32" {
			t.Fatalf("expected Content-Range 'bytes 0-15/32', got %q", got)
		}
		if w.Body.String() != string(testData[:16]) {
			t.Fatalf("expected body %q, got %q", string(testData[:16]), w.Body.String())
		}
	})

	// 3. Second slice Range request (bytes=16-31) -> 206 Partial Content
	t.Run("second slice range request 206", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/media/videos/101/20261026/sample.mp4", nil)
		req.Header.Set("Range", "bytes=16-31")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content, got %d", w.Code)
		}
		if got := w.Header().Get("Content-Range"); got != "bytes 16-31/32" {
			t.Fatalf("expected Content-Range 'bytes 16-31/32', got %q", got)
		}
		if w.Body.String() != string(testData[16:32]) {
			t.Fatalf("expected body %q, got %q", string(testData[16:32]), w.Body.String())
		}
	})

	// 4. Out of bounds Range request -> 416 Range Not Satisfiable
	t.Run("out of bounds range request 416", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/media/videos/101/20261026/sample.mp4", nil)
		req.Header.Set("Range", "bytes=100-200")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Fatalf("expected 416 Requested Range Not Satisfiable, got %d", w.Code)
		}
	})
}
