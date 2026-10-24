package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/logger"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	"github.com/Tidehyc/gorangefeed/internal/infra/redis"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/config.yaml", "path to configuration file")
	flag.Parse()

	// 1. Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// 2. Initialize logger
	l := logger.Init(cfg.App.LogLevel, cfg.App.LogFormat)
	l.Info("starting Worker service", "env", cfg.App.Env)

	// 3. Assemble infrastructure dependencies
	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		l.Error("worker failed to connect to MySQL", "error", err)
		os.Exit(1)
	}
	l.Info("worker connected to MySQL", "host", cfg.MySQL.Host, "port", cfg.MySQL.Port)

	rdb, err := redis.New(&cfg.Redis)
	if err != nil {
		l.Error("worker failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	l.Info("worker connected to Redis", "addr", cfg.Redis.Addr())

	mqConn, err := rabbitmq.New(&cfg.RabbitMQ)
	if err != nil {
		l.Error("worker failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	l.Info("worker connected to RabbitMQ", "host", cfg.RabbitMQ.Host, "port", cfg.RabbitMQ.Port)

	// 4. Worker execution context
	ctx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()

	// 5. Worker metrics HTTP server
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	metricsSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Worker.MetricsPort),
		Handler: metricsMux,
	}

	go func() {
		l.Info("worker metrics server listening", "port", cfg.Worker.MetricsPort)
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.Error("worker metrics server error", "error", err)
		}
	}()

	// Background worker loop placeholder (will house Outbox dispatcher and MQ consumers in M4/M5)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				l.Info("worker loop exiting")
				return
			case <-ticker.C:
				l.Debug("worker heartbeat tick")
			}
		}
	}()

	l.Info("Worker service running and ready for event dispatching")

	// 6. Signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	l.Info("worker received termination signal", "signal", sig.String())

	// 7. Stop worker processing and shutdown metrics server
	stopWorker()

	metricsCtx, metricsCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer metricsCancel()
	if err := metricsSrv.Shutdown(metricsCtx); err != nil {
		l.Warn("error shutting down worker metrics server", "error", err)
	}

	// 8. Cleanup resources
	if err := mqConn.Close(); err != nil {
		l.Warn("error closing RabbitMQ connection", "error", err)
	}
	if err := rdb.Close(); err != nil {
		l.Warn("error closing Redis client", "error", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}

	l.Info("Worker service shutdown completed cleanly")
}
