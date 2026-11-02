package playback

import (
	"context"
	"errors"

	domainplayback "github.com/Tidehyc/gorangefeed/internal/domain/playback"
)

// Service defines application contracts for playback configuration and QoS aggregation.
type Service interface {
	GetPlaybackConfig(ctx context.Context, networkType string) *domainplayback.PlaybackConfig
	ReportQualityEvents(ctx context.Context, userID int64, events []*domainplayback.PlaybackQualityEvent) (int, error)
}

type playbackService struct{}

// NewService creates a new playback QoS service instance.
func NewService() Service {
	initMetrics()
	return &playbackService{}
}

// GetPlaybackConfig returns adaptive preload count and buffer thresholds based on client network environment.
func (s *playbackService) GetPlaybackConfig(ctx context.Context, rawNetworkType string) *domainplayback.PlaybackConfig {
	netType := domainplayback.NormalizeNetworkType(rawNetworkType)

	switch netType {
	case domainplayback.NetworkTypeWiFi:
		return &domainplayback.PlaybackConfig{
			NetworkType:     domainplayback.NetworkTypeWiFi,
			PreloadCount:    3,
			InitialBufferMs: 300,
			MaxBufferMs:     5000,
			BitrateLevel:    domainplayback.BitrateLevelHigh,
			EnableAutoPlay:  true,
		}
	case domainplayback.NetworkType5G:
		return &domainplayback.PlaybackConfig{
			NetworkType:     domainplayback.NetworkType5G,
			PreloadCount:    2,
			InitialBufferMs: 300,
			MaxBufferMs:     4000,
			BitrateLevel:    domainplayback.BitrateLevelHigh,
			EnableAutoPlay:  true,
		}
	case domainplayback.NetworkType4G:
		return &domainplayback.PlaybackConfig{
			NetworkType:     domainplayback.NetworkType4G,
			PreloadCount:    1,
			InitialBufferMs: 400,
			MaxBufferMs:     3000,
			BitrateLevel:    domainplayback.BitrateLevelMedium,
			EnableAutoPlay:  true,
		}
	case domainplayback.NetworkTypeWeakNet:
		return &domainplayback.PlaybackConfig{
			NetworkType:     domainplayback.NetworkTypeWeakNet,
			PreloadCount:    0,
			InitialBufferMs: 800,
			MaxBufferMs:     2000,
			BitrateLevel:    domainplayback.BitrateLevelLow,
			EnableAutoPlay:  false,
		}
	default:
		return &domainplayback.PlaybackConfig{
			NetworkType:     domainplayback.NetworkTypeUnknown,
			PreloadCount:    1,
			InitialBufferMs: 500,
			MaxBufferMs:     3000,
			BitrateLevel:    domainplayback.BitrateLevelMedium,
			EnableAutoPlay:  true,
		}
	}
}

// ReportQualityEvents aggregates client telemetry into Prometheus QoS monitoring metrics.
func (s *playbackService) ReportQualityEvents(ctx context.Context, userID int64, events []*domainplayback.PlaybackQualityEvent) (int, error) {
	if len(events) == 0 {
		return 0, errors.New("events slice cannot be empty")
	}

	processed := 0
	for _, ev := range events {
		if ev == nil {
			continue
		}
		if err := ev.Validate(); err != nil {
			// Skip individual invalid event without aborting entire batch
			continue
		}
		if ev.UserID <= 0 && userID > 0 {
			ev.UserID = userID
		}

		labels := map[string]string{
			"network_type": ev.NetworkType,
		}

		if ev.FirstFrameMs > 0 {
			playbackFirstFrameDuration.With(labels).Observe(float64(ev.FirstFrameMs) / 1000.0)
		}
		if ev.StutterCount > 0 {
			playbackStutterEvents.With(labels).Add(float64(ev.StutterCount))
		}
		if ev.StutterDurationMs > 0 {
			playbackStutterDuration.With(labels).Add(float64(ev.StutterDurationMs) / 1000.0)
		}
		if ev.IsCompleted {
			playbackCompletionTotal.With(labels).Inc()
		}
		playbackSessionsTotal.With(labels).Inc()

		processed++
	}

	return processed, nil
}
