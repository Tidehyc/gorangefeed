package playback

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domainplayback "github.com/Tidehyc/gorangefeed/internal/domain/playback"
	"github.com/gin-gonic/gin"
)

type mockService struct {
	configFn func(ctx context.Context, networkType string) *domainplayback.PlaybackConfig
	reportFn func(ctx context.Context, userID int64, events []*domainplayback.PlaybackQualityEvent) (int, error)
}

func (m *mockService) GetPlaybackConfig(ctx context.Context, networkType string) *domainplayback.PlaybackConfig {
	if m.configFn != nil {
		return m.configFn(ctx, networkType)
	}
	return &domainplayback.PlaybackConfig{
		NetworkType:     networkType,
		PreloadCount:    3,
		InitialBufferMs: 300,
		MaxBufferMs:     5000,
		BitrateLevel:    domainplayback.BitrateLevelHigh,
		EnableAutoPlay:  true,
	}
}

func (m *mockService) ReportQualityEvents(ctx context.Context, userID int64, events []*domainplayback.PlaybackQualityEvent) (int, error) {
	if m.reportFn != nil {
		return m.reportFn(ctx, userID, events)
	}
	return len(events), nil
}

func TestHandler_GetConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &mockService{}
	h := NewHandler(svc)

	engine := gin.New()
	engine.GET("/api/v1/playback/config", h.GetConfig)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/playback/config?network_type=wifi", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp struct {
		Data domainplayback.PlaybackConfig `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}
	if resp.Data.PreloadCount != 3 || resp.Data.BitrateLevel != domainplayback.BitrateLevelHigh {
		t.Errorf("unexpected config response: %+v", resp.Data)
	}
}

func TestHandler_ReportEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &mockService{}
	h := NewHandler(svc)

	engine := gin.New()
	engine.POST("/api/v1/playback/events", h.ReportEvents)

	t.Run("BatchEventsSuccess", func(t *testing.T) {
		body := ReportEventsRequest{
			Events: []*domainplayback.PlaybackQualityEvent{
				{
					VideoID:           101,
					FirstFrameMs:      150,
					StutterCount:      0,
					StutterDurationMs: 0,
					PlayDurationMs:    10000,
					VideoDurationMs:   10000,
					IsCompleted:       true,
					NetworkType:       "wifi",
				},
			},
		}
		data, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/playback/events", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("SingleFlatEventSuccess", func(t *testing.T) {
		body := ReportEventsRequest{
			VideoID:         102,
			FirstFrameMs:    200,
			PlayDurationMs:  8000,
			VideoDurationMs: 15000,
			NetworkType:     "4g",
		}
		data, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/playback/events", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("MissingEventsAndVideoID", func(t *testing.T) {
		body := ReportEventsRequest{}
		data, _ := json.Marshal(body)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/playback/events", bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
	})
}
