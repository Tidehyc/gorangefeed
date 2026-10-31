package outbox

import "github.com/prometheus/client_golang/prometheus"

var (
	outboxDispatchedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "gorangefeed",
		Subsystem: "outbox",
		Name:      "dispatched_total",
		Help:      "Total number of outbox events successfully dispatched to message broker",
	})

	outboxRetriedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "gorangefeed",
		Subsystem: "outbox",
		Name:      "retried_total",
		Help:      "Total number of outbox events retried due to publish failure",
	})

	outboxDeadTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "gorangefeed",
		Subsystem: "outbox",
		Name:      "dead_total",
		Help:      "Total number of outbox events moved to dead status after exhausting max attempts",
	})

	outboxBacklogGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "gorangefeed",
		Subsystem: "outbox",
		Name:      "backlog_gauge",
		Help:      "Current number of pending outbox events awaiting dispatch",
	})
)

func init() {
	prometheus.MustRegister(
		outboxDispatchedTotal,
		outboxRetriedTotal,
		outboxDeadTotal,
		outboxBacklogGauge,
	)
}
