package notification

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricsOnce sync.Once

	sseConnectedClients = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "gorangefeed",
		Subsystem: "sse",
		Name:      "connected_clients",
		Help:      "Current number of locally connected SSE client streams",
	})

	sseBroadcastReceivedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "gorangefeed",
		Subsystem: "sse",
		Name:      "broadcast_received_total",
		Help:      "Total number of notifications received from Redis Pub/Sub broadcast channel",
	})

	sseBroadcastSentTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "gorangefeed",
		Subsystem: "sse",
		Name:      "broadcast_sent_total",
		Help:      "Total number of notifications dispatched to local SSE client channels",
	})

	sseDispatchLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "gorangefeed",
		Subsystem: "sse",
		Name:      "dispatch_latency_seconds",
		Help:      "Duration from notification creation to local client delivery",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
	})
)

func initMetrics() {
	metricsOnce.Do(func() {
		prometheus.MustRegister(
			sseConnectedClients,
			sseBroadcastReceivedTotal,
			sseBroadcastSentTotal,
			sseDispatchLatency,
		)
	})
}
