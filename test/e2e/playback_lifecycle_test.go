package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainplayback "github.com/Tidehyc/gorangefeed/internal/domain/playback"
	httpplayback "github.com/Tidehyc/gorangefeed/internal/http/playback"
)

type playbackConfigEnvelope struct {
	Code    int                           `json:"code"`
	Message string                        `json:"message"`
	Data    domainplayback.PlaybackConfig `json:"data"`
}

type playbackReportEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Success        bool `json:"success"`
		ProcessedCount int  `json:"processed_count"`
	} `json:"data"`
}

func TestM6_PlaybackLifecycle_And_Metrics_EndToEnd(t *testing.T) {
	ctxExt, cleanup := setupE2ETestExtended(t)
	if cleanup != nil {
		defer cleanup()
	}

	engine := ctxExt.Engine

	suffix := time.Now().Format("0405_000")
	userName := fmt.Sprintf("m6_user_%s", suffix)
	testPass := "SecurePass123"

	// Register user
	uid, token := registerAndLogin(t, engine, userName, testPass)
	if uid <= 0 || token == "" {
		t.Fatalf("failed to register test user: uid=%d", uid)
	}

	// =========================================================================
	// E2E-M6-01: 端侧播放预加载自适应配置推荐策略断言 (FR-QOS-01)
	// =========================================================================
	t.Run("E2E-M6-01_Adaptive_Playback_Config", func(t *testing.T) {
		tests := []struct {
			netType         string
			expectedPreload int
			expectedInitial int
			expectedMax     int
			expectedBitrate string
			autoPlay        bool
		}{
			{"wifi", 3, 300, 5000, domainplayback.BitrateLevelHigh, true},
			{"5g", 2, 300, 4000, domainplayback.BitrateLevelHigh, true},
			{"4g", 1, 400, 3000, domainplayback.BitrateLevelMedium, true},
			{"weak_net", 0, 800, 2000, domainplayback.BitrateLevelLow, false},
			{"unknown", 1, 500, 3000, domainplayback.BitrateLevelMedium, true},
		}

		for _, tc := range tests {
			w := httptest.NewRecorder()
			req := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/playback/config?network_type=%s", tc.netType), nil, "192.168.1.50")
			req.Header.Set("Authorization", "Bearer "+token)
			engine.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("get playback config for %s failed: code=%d, body=%s", tc.netType, w.Code, w.Body.String())
			}

			var resp playbackConfigEnvelope
			parseJSONResponse(t, w, &resp)

			if resp.Data.PreloadCount != tc.expectedPreload {
				t.Errorf("network %s: expected preload_count=%d, got %d", tc.netType, tc.expectedPreload, resp.Data.PreloadCount)
			}
			if resp.Data.InitialBufferMs != tc.expectedInitial {
				t.Errorf("network %s: expected initial_buffer_ms=%d, got %d", tc.netType, tc.expectedInitial, resp.Data.InitialBufferMs)
			}
			if resp.Data.MaxBufferMs != tc.expectedMax {
				t.Errorf("network %s: expected max_buffer_ms=%d, got %d", tc.netType, tc.expectedMax, resp.Data.MaxBufferMs)
			}
			if resp.Data.BitrateLevel != tc.expectedBitrate {
				t.Errorf("network %s: expected bitrate_level=%s, got %s", tc.netType, tc.expectedBitrate, resp.Data.BitrateLevel)
			}
			if resp.Data.EnableAutoPlay != tc.autoPlay {
				t.Errorf("network %s: expected enable_autoplay=%v, got %v", tc.netType, tc.autoPlay, resp.Data.EnableAutoPlay)
			}
		}
	})

	// =========================================================================
	// E2E-M6-02: 播放质量 QoS 上报批量与单条兼容性断言 (FR-QOS-02)
	// =========================================================================
	t.Run("E2E-M6-02_QoS_Telemetry_Batch_And_Single_Reporting", func(t *testing.T) {
		// 1. Single flat event submission
		singleReq := httpplayback.ReportEventsRequest{
			VideoID:           9001,
			FirstFrameMs:      150,
			StutterCount:      0,
			StutterDurationMs: 0,
			PlayDurationMs:    15000,
			VideoDurationMs:   15000,
			IsCompleted:       true,
			NetworkType:       "wifi",
		}
		wSingle := httptest.NewRecorder()
		reqSingle := newJSONRequest(http.MethodPost, "/api/v1/playback/events", singleReq, "192.168.1.50")
		reqSingle.Header.Set("Authorization", "Bearer "+token)
		engine.ServeHTTP(wSingle, reqSingle)

		if wSingle.Code != http.StatusOK {
			t.Fatalf("single event reporting failed: code=%d, body=%s", wSingle.Code, wSingle.Body.String())
		}
		var singleResp playbackReportEnvelope
		parseJSONResponse(t, wSingle, &singleResp)
		if !singleResp.Data.Success || singleResp.Data.ProcessedCount != 1 {
			t.Fatalf("expected processed_count=1, got %+v", singleResp.Data)
		}

		// 2. Batch events submission
		batchReq := httpplayback.ReportEventsRequest{
			Events: []*domainplayback.PlaybackQualityEvent{
				{
					VideoID:           9002,
					FirstFrameMs:      280,
					StutterCount:      1,
					StutterDurationMs: 500,
					PlayDurationMs:    12000,
					VideoDurationMs:   15000,
					IsCompleted:       false,
					NetworkType:       "4g",
				},
				{
					VideoID:           9003,
					FirstFrameMs:      90,
					StutterCount:      0,
					StutterDurationMs: 0,
					PlayDurationMs:    20000,
					VideoDurationMs:   20000,
					IsCompleted:       true,
					NetworkType:       "5g",
				},
			},
		}
		wBatch := httptest.NewRecorder()
		reqBatch := newJSONRequest(http.MethodPost, "/api/v1/playback/events", batchReq, "192.168.1.50")
		reqBatch.Header.Set("Authorization", "Bearer "+token)
		engine.ServeHTTP(wBatch, reqBatch)

		if wBatch.Code != http.StatusOK {
			t.Fatalf("batch events reporting failed: code=%d, body=%s", wBatch.Code, wBatch.Body.String())
		}
		var batchResp playbackReportEnvelope
		parseJSONResponse(t, wBatch, &batchResp)
		if !batchResp.Data.Success || batchResp.Data.ProcessedCount != 2 {
			t.Fatalf("expected processed_count=2, got %+v", batchResp.Data)
		}

		// 3. Invalid payload check
		wInvalid := httptest.NewRecorder()
		reqInvalid := newJSONRequest(http.MethodPost, "/api/v1/playback/events", httpplayback.ReportEventsRequest{}, "192.168.1.50")
		reqInvalid.Header.Set("Authorization", "Bearer "+token)
		engine.ServeHTTP(wInvalid, reqInvalid)
		if wInvalid.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty event payload, got %d", wInvalid.Code)
		}
	})

	// =========================================================================
	// E2E-M6-03: Prometheus 指标大盘导出与指标原子递增断言 (NFR-OBS-01, FR-QOS-02)
	// =========================================================================
	t.Run("E2E-M6-03_Prometheus_Metrics_Endpoint_Export", func(t *testing.T) {
		wMetrics := httptest.NewRecorder()
		reqMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		engine.ServeHTTP(wMetrics, reqMetrics)

		if wMetrics.Code != http.StatusOK {
			t.Fatalf("fetch /metrics failed: code=%d", wMetrics.Code)
		}

		body := wMetrics.Body.String()

		expectedMetrics := []string{
			"playback_first_frame_duration_seconds",
			"playback_stutter_events_total",
			"playback_stutter_duration_seconds_total",
			"playback_completion_total",
			"playback_sessions_total",
		}

		for _, metric := range expectedMetrics {
			if !strings.Contains(body, metric) {
				t.Errorf("missing expected Prometheus metric %q in /metrics output", metric)
			}
		}

		// Assert that playback_sessions_total has observed the events
		if !strings.Contains(body, `playback_sessions_total{network_type="wifi"}`) {
			t.Errorf("expected wifi session metric in Prometheus output")
		}
		if !strings.Contains(body, `playback_completion_total{network_type="wifi"}`) {
			t.Errorf("expected wifi completion metric in Prometheus output")
		}
	})
}
