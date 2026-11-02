package playback

import (
	"testing"
)

func TestNormalizeNetworkType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"wifi", NetworkTypeWiFi},
		{"WIFI", NetworkTypeWiFi},
		{"wlan", NetworkTypeWiFi},
		{"5g", NetworkType5G},
		{"5G", NetworkType5G},
		{"4g", NetworkType4G},
		{"LTE", NetworkType4G},
		{"2g", NetworkTypeWeakNet},
		{"3g", NetworkTypeWeakNet},
		{"weak_net", NetworkTypeWeakNet},
		{"poor", NetworkTypeWeakNet},
		{"bluetooth", NetworkTypeUnknown},
		{"", NetworkTypeUnknown},
	}

	for _, tt := range tests {
		got := NormalizeNetworkType(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeNetworkType(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestPlaybackQualityEvent_Validate(t *testing.T) {
	t.Run("ValidEvent", func(t *testing.T) {
		ev := &PlaybackQualityEvent{
			VideoID:           101,
			FirstFrameMs:      120,
			StutterCount:      0,
			StutterDurationMs: 0,
			PlayDurationMs:    15000,
			VideoDurationMs:   15000,
			IsCompleted:       true,
			NetworkType:       "WIFI",
		}
		if err := ev.Validate(); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
		if ev.NetworkType != NetworkTypeWiFi {
			t.Errorf("expected normalized network type %q, got %q", NetworkTypeWiFi, ev.NetworkType)
		}
		if ev.ReportedAt.IsZero() {
			t.Errorf("expected non-zero reported_at")
		}
	})

	t.Run("InvalidVideoID", func(t *testing.T) {
		ev := &PlaybackQualityEvent{
			VideoID: 0,
		}
		if err := ev.Validate(); err != ErrInvalidVideoID {
			t.Errorf("expected ErrInvalidVideoID, got %v", err)
		}
	})

	t.Run("NegativeMetrics", func(t *testing.T) {
		ev := &PlaybackQualityEvent{
			VideoID:      101,
			FirstFrameMs: -1,
		}
		if err := ev.Validate(); err != ErrNegativeValue {
			t.Errorf("expected ErrNegativeValue, got %v", err)
		}
	})
}
