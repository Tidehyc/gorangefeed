package router

import (
	"context"
	"net/http"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// DependencyChecks holds probe functions for external dependencies.
type DependencyChecks struct {
	CheckMySQL    func(ctx context.Context) error
	CheckRedis    func(ctx context.Context) error
	CheckRabbitMQ func() error
}

// NewRouter constructs and configures the Gin engine with probes, metrics and standard middlewares.
func NewRouter(deps DependencyChecks) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLog(nil))
	r.Use(middleware.Metrics())

	// Liveness probe: reflects process liveness
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// Readiness probe: reflects dependency readiness
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		results := make(map[string]string)
		isReady := true

		if deps.CheckMySQL != nil {
			if err := deps.CheckMySQL(ctx); err != nil {
				isReady = false
				results["mysql"] = err.Error()
			} else {
				results["mysql"] = "ok"
			}
		}

		if deps.CheckRedis != nil {
			if err := deps.CheckRedis(ctx); err != nil {
				isReady = false
				results["redis"] = err.Error()
			} else {
				results["redis"] = "ok"
			}
		}

		if deps.CheckRabbitMQ != nil {
			if err := deps.CheckRabbitMQ(); err != nil {
				isReady = false
				results["rabbitmq"] = err.Error()
			} else {
				results["rabbitmq"] = "ok"
			}
		}

		if !isReady {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":     "unavailable",
				"components": results,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":     "ready",
			"components": results,
		})
	})

	// Prometheus metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Standard API routing group
	api := r.Group("/api")
	{
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "pong"})
		})
	}

	// Internal service routing group
	r.Group("/internal")

	return r
}
