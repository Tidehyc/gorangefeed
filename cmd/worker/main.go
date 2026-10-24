package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/logger"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	"github.com/Tidehyc/gorangefeed/internal/infra/redis"
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

	// 5. Signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	l.Info("worker received termination signal", "signal", sig.String())

	// 6. Stop worker processing
	stopWorker()

	// 7. Cleanup resources
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
