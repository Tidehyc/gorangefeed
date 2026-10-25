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

	httpaccount "github.com/Tidehyc/gorangefeed/internal/http/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/logger"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceaccount "github.com/Tidehyc/gorangefeed/internal/infra/persistence/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	"github.com/Tidehyc/gorangefeed/internal/infra/redis"
	"github.com/Tidehyc/gorangefeed/internal/infra/router"
	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
	serviceaccount "github.com/Tidehyc/gorangefeed/internal/service/account"
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
	l.Info("starting API service", "env", cfg.App.Env, "port", cfg.Server.Port)

	// 3. Assemble infrastructure dependencies
	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		l.Error("failed to connect to MySQL", "error", err)
		os.Exit(1)
	}
	l.Info("connected to MySQL", "host", cfg.MySQL.Host, "port", cfg.MySQL.Port)

	rdb, err := redis.New(&cfg.Redis)
	if err != nil {
		l.Error("failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	l.Info("connected to Redis", "addr", cfg.Redis.Addr())

	mqConn, err := rabbitmq.New(&cfg.RabbitMQ)
	if err != nil {
		l.Error("failed to connect to RabbitMQ", "error", err)
		os.Exit(1)
	}
	l.Info("connected to RabbitMQ", "host", cfg.RabbitMQ.Host, "port", cfg.RabbitMQ.Port)

	// 4. Assemble Account Domain & Storage
	avatarStorage, err := storage.NewAvatarStorage("uploads")
	if err != nil {
		l.Error("failed to initialize avatar storage", "error", err)
		os.Exit(1)
	}

	accountRepo := persistenceaccount.NewRepository(db)
	tokenStore := persistenceaccount.NewTokenStore(rdb)
	jwtMgr := jwt.NewTokenManager(cfg.JWT)
	accountSvc := serviceaccount.NewService(accountRepo, tokenStore, jwtMgr, cfg.JWT)
	accountHandler := httpaccount.NewHandler(accountSvc, jwtMgr, avatarStorage)

	// 5. Construct Router with probe checks
	engine := router.NewRouter(router.DependencyChecks{
		CheckMySQL: func(ctx context.Context) error {
			return mysql.Ping(ctx, db)
		},
		CheckRedis: func(ctx context.Context) error {
			return redis.Ping(ctx, rdb)
		},
		CheckRabbitMQ: func() error {
			return rabbitmq.Ping(mqConn)
		},
	})

	// Mount media static route and account domain routes
	router.MountMediaRoutes(engine, avatarStorage.BaseDir())
	httpaccount.RegisterRoutes(engine, accountHandler, jwtMgr, tokenStore, rdb)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 5. Start HTTP server in a goroutine
	go func() {
		l.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 6. Listen for termination signals (SIGINT, SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	l.Info("received termination signal, initiating graceful shutdown", "signal", sig.String())

	// 7. Graceful shutdown: wait for in-flight requests
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		l.Error("HTTP server forced to shutdown", "error", err)
	} else {
		l.Info("HTTP server gracefully stopped")
	}

	// 8. Release infrastructure resources in order
	if err := rdb.Close(); err != nil {
		l.Warn("error closing Redis client", "error", err)
	} else {
		l.Info("Redis connection closed")
	}

	if err := mqConn.Close(); err != nil {
		l.Warn("error closing RabbitMQ connection", "error", err)
	} else {
		l.Info("RabbitMQ connection closed")
	}

	if sqlDB, err := db.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			l.Warn("error closing MySQL database", "error", err)
		} else {
			l.Info("MySQL connection pool closed")
		}
	}

	l.Info("API service shutdown completed cleanly")
}
