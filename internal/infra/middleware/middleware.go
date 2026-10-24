package middleware

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

const HeaderRequestID = "X-Request-ID"
const ContextKeyRequestID = "request_id"

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
}

// RequestID middleware extracts or generates a unique Request ID for each HTTP request.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(HeaderRequestID)
		if reqID == "" {
			reqID = uuid.NewString()
		}

		c.Set(ContextKeyRequestID, reqID)
		c.Header(HeaderRequestID, reqID)
		c.Next()
	}
}

// AccessLog middleware logs each HTTP request using structured slog.
func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		reqID, _ := c.Get(ContextKeyRequestID)

		if rawQuery != "" {
			path = path + "?" + rawQuery
		}

		logger.Info("http request",
			slog.String("request_id", reqID.(string)),
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", statusCode),
			slog.Duration("latency", latency),
			slog.String("client_ip", clientIP),
		)
	}
}

// Metrics records Prometheus counters and histograms for HTTP requests.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		duration := time.Since(start).Seconds()

		pattern := c.FullPath()
		if pattern == "" {
			pattern = "unmatched"
		}

		httpRequestsTotal.WithLabelValues(c.Request.Method, pattern, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, pattern).Observe(duration)
	}
}
