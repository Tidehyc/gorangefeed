package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
