package playback

import (
	"errors"
	"strings"
	"time"
)

// NetworkType constants represent client connectivity environments.
const (
	NetworkTypeWiFi    = "wifi"
	NetworkType5G      = "5g"
	NetworkType4G      = "4g"
	NetworkTypeWeakNet = "weak_net"
	NetworkTypeUnknown = "unknown"
)

// BitrateLevel constants recommend video rendering resolutions.
const (
	BitrateLevelHigh   = "high"   // 1080p / 720p 60fps
	BitrateLevelMedium = "medium" // 720p 30fps
	BitrateLevelLow    = "low"    // 480p / 360p
)

var (
	ErrInvalidVideoID = errors.New("video_id must be greater than 0")
	ErrNegativeValue  = errors.New("duration and count metrics cannot be negative")
)

// PlaybackConfig models cloud-side guidance for client playback & preload strategy.
type PlaybackConfig struct {
	NetworkType     string `json:"network_type"`
	PreloadCount    int    `json:"preload_count"`
	InitialBufferMs int    `json:"initial_buffer_ms"`
	MaxBufferMs     int    `json:"max_buffer_ms"`
	BitrateLevel    string `json:"bitrate_level"`
	EnableAutoPlay  bool   `json:"enable_autoplay"`
}

// PlaybackQualityEvent models client-reported quality of service telemetry.
type PlaybackQualityEvent struct {
	VideoID           int64     `json:"video_id"`
	UserID            int64     `json:"user_id,omitempty"`
	FirstFrameMs      int       `json:"first_frame_ms"`      // Time from request/slide to first frame rendered
	StutterCount      int       `json:"stutter_count"`        // Re-buffering stall count
	StutterDurationMs int       `json:"stutter_duration_ms"` // Total duration in ms spent in stall
	PlayDurationMs    int       `json:"play_duration_ms"`    // Effective playback duration watched
	VideoDurationMs   int       `json:"video_duration_ms"`   // Total duration of media
	IsCompleted       bool      `json:"is_completed"`        // Whether video was watched to end
	NetworkType       string    `json:"network_type"`        // wifi, 5g, 4g, weak_net, unknown
	ErrorCode         string    `json:"error_code,omitempty"`
	ReportedAt        time.Time `json:"reported_at"`
}

// NormalizeNetworkType standardizes arbitrary client network strings.
func NormalizeNetworkType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case NetworkTypeWiFi, "wlan":
		return NetworkTypeWiFi
	case NetworkType5G:
		return NetworkType5G
	case NetworkType4G, "lte":
		return NetworkType4G
	case NetworkTypeWeakNet, "2g", "3g", "poor":
		return NetworkTypeWeakNet
	default:
		return NetworkTypeUnknown
	}
}

// Validate verifies playback quality event payload integrity.
func (e *PlaybackQualityEvent) Validate() error {
	if e.VideoID <= 0 {
		return ErrInvalidVideoID
	}
	if e.FirstFrameMs < 0 || e.StutterCount < 0 || e.StutterDurationMs < 0 || e.PlayDurationMs < 0 || e.VideoDurationMs < 0 {
		return ErrNegativeValue
	}
	e.NetworkType = NormalizeNetworkType(e.NetworkType)
	if e.ReportedAt.IsZero() {
		e.ReportedAt = time.Now().UTC()
	}
	return nil
}
