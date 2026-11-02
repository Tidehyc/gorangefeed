package playback

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricsOnce sync.Once

	playbackFirstFrameDuration *prometheus.HistogramVec
	playbackStutterEvents      *prometheus.CounterVec
	playbackStutterDuration    *prometheus.CounterVec
	playbackCompletionTotal    *prometheus.CounterVec
	playbackSessionsTotal      *prometheus.CounterVec
)

// initMetrics registers Prometheus playback QoS metrics idempotently.
func initMetrics() {
	metricsOnce.Do(func() {
		playbackFirstFrameDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "playback",
				Name:      "first_frame_duration_seconds",
				Help:      "Histogram of video playback first frame rendering latency in seconds.",
				Buckets:   []float64{0.05, 0.1, 0.2, 0.3, 0.5, 0.8, 1.0, 1.5, 2.0, 3.0, 5.0},
			},
			[]string{"network_type"},
		)

		playbackStutterEvents = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "playback",
				Name:      "stutter_events_total",
				Help:      "Total count of video re-buffering stall occurrences during playback.",
			},
			[]string{"network_type"},
		)

		playbackStutterDuration = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "playback",
				Name:      "stutter_duration_seconds_total",
				Help:      "Total cumulative duration spent in playback stall buffer in seconds.",
			},
			[]string{"network_type"},
		)

		playbackCompletionTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "playback",
				Name:      "completion_total",
				Help:      "Total count of video full completion playback sessions.",
			},
			[]string{"network_type"},
		)

		playbackSessionsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "playback",
				Name:      "sessions_total",
				Help:      "Total count of client playback QoS sessions reported.",
			},
			[]string{"network_type"},
		)

		prometheus.MustRegister(
			playbackFirstFrameDuration,
			playbackStutterEvents,
			playbackStutterDuration,
			playbackCompletionTotal,
			playbackSessionsTotal,
		)
	})
}
