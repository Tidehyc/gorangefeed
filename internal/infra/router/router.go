package router

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// MountMediaRoutes mounts the media uploads directory to the /media route,
// providing HTTP 206 Range support and disabling directory browsing.
func MountMediaRoutes(r *gin.Engine, mediaDir string) {
	if mediaDir == "" {
		mediaDir = "uploads"
	}
	r.Static("/media", mediaDir)
}

// MountWebRoutes mounts the web client.
//
// Two layouts are supported so a fresh checkout still serves the demo page:
//   - webDir/dist/index.html exists → the React build output (web/dist) is the document root;
//   - otherwise webDir itself is used, which is what the pre-M14 static demo shipped.
//
// /web keeps serving the raw directory so the archived M10 page stays reachable at
// /web/legacy/index.html. Everything else falls through to the SPA history fallback below.
func MountWebRoutes(r *gin.Engine, webDir string) {
	if webDir == "" {
		webDir = "web"
	}

	documentRoot := webDir
	if info, err := os.Stat(filepath.Join(webDir, "dist", "index.html")); err == nil && !info.IsDir() {
		documentRoot = filepath.Join(webDir, "dist")
	}
	indexFile := filepath.Join(documentRoot, "index.html")

	r.Static("/web", webDir)
	r.StaticFile("/", indexFile)
	if info, err := os.Stat(filepath.Join(documentRoot, "assets")); err == nil && info.IsDir() {
		r.Static("/assets", filepath.Join(documentRoot, "assets"))
	}

	// SPA history fallback: a refresh on /user/12 must not 404. Without this the router would
	// answer "404 page not found" for every deep link the app itself produces.
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// An unknown API endpoint is an API error, never a 200 HTML document: returning the shell
		// would hide typos and break JSON clients.
		for _, prefix := range []string{"/api/", "/media/", "/internal/"} {
			if strings.HasPrefix(path, prefix) {
				c.JSON(http.StatusNotFound, gin.H{
					"data":  nil,
					"error": gin.H{"code": "not_found", "message": "Not Found"},
				})
				return
			}
		}
		switch path {
		case "/api", "/healthz", "/readyz", "/metrics":
			c.Status(http.StatusNotFound)
			return
		}

		// Requests that look like a file must resolve to that file or 404. Serving index.html for
		// a missing .js would make the browser fail with an opaque MIME error instead of a clean 404.
		if filepath.Ext(path) != "" {
			candidate := filepath.Join(documentRoot, strings.TrimPrefix(filepath.Clean(path), "/"))
			inside := candidate == documentRoot ||
				strings.HasPrefix(candidate, documentRoot+string(os.PathSeparator))
			if inside {
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					c.File(candidate)
					return
				}
			}
			c.Status(http.StatusNotFound)
			return
		}

		c.File(indexFile)
	})
}
