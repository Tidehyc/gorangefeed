package playback

import (
	"context"
	"testing"

	domainplayback "github.com/Tidehyc/gorangefeed/internal/domain/playback"
)

func TestPlaybackService_GetPlaybackConfig(t *testing.T) {
	svc := NewService()
	ctx := context.Background()

	tests := []struct {
		networkType     string
		expectedPreload int
		expectedBitrate string
		autoPlay        bool
	}{
		{"wifi", 3, domainplayback.BitrateLevelHigh, true},
		{"5G", 2, domainplayback.BitrateLevelHigh, true},
		{"4g", 1, domainplayback.BitrateLevelMedium, true},
		{"weak_net", 0, domainplayback.BitrateLevelLow, false},
		{"unknown", 1, domainplayback.BitrateLevelMedium, true},
	}

	for _, tt := range tests {
		cfg := svc.GetPlaybackConfig(ctx, tt.networkType)
		if cfg.PreloadCount != tt.expectedPreload {
			t.Errorf("network %s expected preload %d, got %d", tt.networkType, tt.expectedPreload, cfg.PreloadCount)
		}
		if cfg.BitrateLevel != tt.expectedBitrate {
			t.Errorf("network %s expected bitrate %s, got %s", tt.networkType, tt.expectedBitrate, cfg.BitrateLevel)
		}
		if cfg.EnableAutoPlay != tt.autoPlay {
			t.Errorf("network %s expected autoplay %v, got %v", tt.networkType, tt.autoPlay, cfg.EnableAutoPlay)
		}
	}
}

func TestPlaybackService_ReportQualityEvents(t *testing.T) {
	svc := NewService()
	ctx := context.Background()

	t.Run("EmptyEvents", func(t *testing.T) {
		_, err := svc.ReportQualityEvents(ctx, 1001, nil)
		if err == nil {
			t.Errorf("expected error on empty events")
		}
	})

	t.Run("ValidAndInvalidBatch", func(t *testing.T) {
		events := []*domainplayback.PlaybackQualityEvent{
			{
				VideoID:           101,
				FirstFrameMs:      120,
				StutterCount:      1,
				StutterDurationMs: 450,
				PlayDurationMs:    15000,
				VideoDurationMs:   15000,
				IsCompleted:       true,
				NetworkType:       "wifi",
			},
			{
				VideoID: 0, // Invalid, will be skipped
			},
			{
				VideoID:           102,
				FirstFrameMs:      250,
				StutterCount:      0,
				StutterDurationMs: 0,
				PlayDurationMs:    5000,
				VideoDurationMs:   20000,
				IsCompleted:       false,
				NetworkType:       "4g",
			},
		}

		processed, err := svc.ReportQualityEvents(ctx, 2001, events)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if processed != 2 {
			t.Errorf("expected 2 processed events, got %d", processed)
		}
	})
}
