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

	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
	httpaccount "github.com/Tidehyc/gorangefeed/internal/http/account"
	httpadmin "github.com/Tidehyc/gorangefeed/internal/http/admin"
	httpasset "github.com/Tidehyc/gorangefeed/internal/http/asset"
	httpcreator "github.com/Tidehyc/gorangefeed/internal/http/creator"
	httpfeed "github.com/Tidehyc/gorangefeed/internal/http/feed"
	httpinteraction "github.com/Tidehyc/gorangefeed/internal/http/interaction"
	httpnotification "github.com/Tidehyc/gorangefeed/internal/http/notification"
	httpplayback "github.com/Tidehyc/gorangefeed/internal/http/playback"
	httprelation "github.com/Tidehyc/gorangefeed/internal/http/relation"
	httpupload "github.com/Tidehyc/gorangefeed/internal/http/upload"
	httpvideo "github.com/Tidehyc/gorangefeed/internal/http/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/logger"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceaccount "github.com/Tidehyc/gorangefeed/internal/infra/persistence/account"
	persistenceasset "github.com/Tidehyc/gorangefeed/internal/infra/persistence/asset"
	persistencecreator "github.com/Tidehyc/gorangefeed/internal/infra/persistence/creator"
	persistencefeed "github.com/Tidehyc/gorangefeed/internal/infra/persistence/feed"
	persistencemoderation "github.com/Tidehyc/gorangefeed/internal/infra/persistence/moderation"
	persistencenotification "github.com/Tidehyc/gorangefeed/internal/infra/persistence/notification"
	persistenceinteraction "github.com/Tidehyc/gorangefeed/internal/infra/persistence/interaction"
	persistencerelation "github.com/Tidehyc/gorangefeed/internal/infra/persistence/relation"
	persistenceupload "github.com/Tidehyc/gorangefeed/internal/infra/persistence/upload"
	persistencevideo "github.com/Tidehyc/gorangefeed/internal/infra/persistence/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	"github.com/Tidehyc/gorangefeed/internal/infra/redis"
	"github.com/Tidehyc/gorangefeed/internal/infra/router"
	infrascheduler "github.com/Tidehyc/gorangefeed/internal/infra/scheduler"
	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
	serviceaccount "github.com/Tidehyc/gorangefeed/internal/service/account"
	serviceasset "github.com/Tidehyc/gorangefeed/internal/service/asset"
	servicecreator "github.com/Tidehyc/gorangefeed/internal/service/creator"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
	servicemoderation "github.com/Tidehyc/gorangefeed/internal/service/moderation"
	serviceinteraction "github.com/Tidehyc/gorangefeed/internal/service/interaction"
	servicenotification "github.com/Tidehyc/gorangefeed/internal/service/notification"
	serviceplayback "github.com/Tidehyc/gorangefeed/internal/service/playback"
	servicerelation "github.com/Tidehyc/gorangefeed/internal/service/relation"
	serviceupload "github.com/Tidehyc/gorangefeed/internal/service/upload"
	servicevideo "github.com/Tidehyc/gorangefeed/internal/service/video"
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

	// 5. Assemble Video Domain & Storage
	videoStorage, err := storage.NewVideoStorage("uploads")
	if err != nil {
		l.Error("failed to initialize video storage", "error", err)
		os.Exit(1)
	}
	videoRepo := persistencevideo.NewRepository(db)
	videoSvc := servicevideo.NewService(videoRepo, accountRepo)
	videoHandler := httpvideo.NewHandler(videoSvc, videoStorage)

	// 6. Assemble Chunked Upload Domain & Storage
	chunkStorage, err := storage.NewChunkStorage("uploads")
	if err != nil {
		l.Error("failed to initialize chunk storage", "error", err)
		os.Exit(1)
	}
	uploadSessionRepo := persistenceupload.NewSessionRepository(db)
	uploadBitmapStore := persistenceupload.NewBitmapStore(rdb)
	uploadMergeLocker := persistenceupload.NewMergeLocker(rdb)
	uploadSvc := serviceupload.NewService(uploadSessionRepo, uploadBitmapStore, uploadMergeLocker, chunkStorage)
	uploadHandler := httpupload.NewHandler(uploadSvc)

	// 7. Assemble Interaction & Relation Domains
	interactionRepo := persistenceinteraction.NewRepository(db)
	relationRepo := persistencerelation.NewRepository(db)
	interactionSvc := serviceinteraction.NewService(interactionRepo, videoRepo, accountRepo)
	relationSvc := servicerelation.NewService(relationRepo, accountRepo)
	interactionHandler := httpinteraction.NewHandler(interactionSvc)
	relationHandler := httprelation.NewHandler(relationSvc)

	// 8. Assemble Feed Domain & Caches
	followingRepo := persistencefeed.NewFollowingStore(rdb, db)
	timelineRepo := persistencefeed.NewTimelineStore(rdb)
	hotStore := persistencefeed.NewHotStore(rdb)
	pageCache := persistencefeed.NewPageCacheRepository(rdb)

	cardCache := persistencefeed.NewCardCacheRepository(rdb)
	statCache := persistencefeed.NewStatCacheRepository(rdb)
	exposureRepo := persistencefeed.NewExposureRepository(rdb)
	feedDBRepo := persistencefeed.NewFeedDBRepository(db)
	recallEngine := servicefeed.NewMultiSourceRecallEngine(hotStore, timelineRepo, feedDBRepo)
	diversifier := servicefeed.NewAuthorDiversifier()
	feedSvc := servicefeed.NewService(
		timelineRepo, pageCache, cardCache, statCache, exposureRepo, feedDBRepo,
		servicefeed.WithFollowingDependencies(followingRepo, relationRepo, interactionRepo),
		servicefeed.WithHotTimelineStore(hotStore),
		servicefeed.WithRecommendDependencies(recallEngine, diversifier),
	)
	feedHandler := httpfeed.NewHandler(feedSvc)

	// 8.5 Assemble Notification Domain, Broadcaster and SSE Hub
	notifRepo := persistencenotification.NewRepository(db)
	notifBroadcaster, err := redis.NewRedisBroadcaster(rdb, redis.ChannelNotifications)
	if err != nil {
		l.Error("failed to initialize notification broadcaster", "error", err)
		os.Exit(1)
	}
	notifSvc, err := servicenotification.NewService(notifRepo, notifBroadcaster)
	if err != nil {
		l.Error("failed to initialize notification service", "error", err)
		os.Exit(1)
	}
	sseHub, err := servicenotification.NewSSEHub(notifRepo, notifBroadcaster, rdb)
	if err != nil {
		l.Error("failed to initialize sse hub", "error", err)
		os.Exit(1)
	}
	hubCtx, cancelHub := context.WithCancel(context.Background())
	defer cancelHub()
	if err := sseHub.Start(hubCtx); err != nil {
		l.Error("failed to start sse hub", "error", err)
		os.Exit(1)
	}
	notifHandler := httpnotification.NewHandler(notifSvc, sseHub)

	// 8.6 Assemble Playback QoS Domain
	playbackSvc := serviceplayback.NewService()
	playbackHandler := httpplayback.NewHandler(playbackSvc)

	// 8.7 Assemble Creator Center Domain & Distributed Sequence Generator
	draftRepo := persistencecreator.NewDraftRepository(db)
	taskRepo := persistencecreator.NewScheduledTaskRepository(db)
	seqGen := redis.NewSequenceGenerator(rdb)
	var maxPublishSeq int64
	_ = db.Table("videos").Select("COALESCE(MAX(publish_seq), 0)").Scan(&maxPublishSeq).Error
	_ = seqGen.InitializeSequence(context.Background(), maxPublishSeq)
	delayedScheduler := infrascheduler.NewDelayedScheduler(rdb, taskRepo, videoRepo, draftRepo, seqGen, timelineRepo, pageCache, nil)
	creatorSvc := servicecreator.NewService(draftRepo, taskRepo, videoRepo, seqGen, delayedScheduler, feedSvc)
	creatorHandler := httpcreator.NewHandler(creatorSvc)

	// 8.8 Assemble User Asset Center Domain (Favorites & Watch History)
	favoriteRepo := persistenceasset.NewFavoriteRepository(db)
	historyRepo := persistenceasset.NewHistoryRepository(db, rdb)
	assetSvc, err := serviceasset.NewService(favoriteRepo, historyRepo, videoRepo)
	if err != nil {
		l.Error("failed to initialize asset service", "error", err)
		os.Exit(1)
	}
	assetHandler := httpasset.NewHandler(assetSvc)

	// 8.9 Assemble Content Moderation & Compliance Domain
	moderationRepo := persistencemoderation.NewRepository(db)
	acMatcher := servicemoderation.NewACMatcher()
	moderationSvc := servicemoderation.NewService(moderationRepo, acMatcher, videoRepo, timelineRepo, accountRepo, tokenStore, l)
	adminHandler := httpadmin.NewHandler(moderationSvc)

	// Hook into video publishing to allocate monotonic publish_seq and append to timeline
	videoHandler.AddPublishHook(func(ctx context.Context, videoID int64) {
		// Trigger asynchronous content compliance scan
		if v, _, err := videoRepo.FindDetailByID(ctx, videoID); err == nil && v != nil {
			_, _ = moderationSvc.SubmitVideoForReview(ctx, videoID, domainmoderation.ReviewPolicyPostReview, v.Title, v.Description)
		}

		seq, err := seqGen.NextPublishSeq(ctx)
		if err == nil && seq > 0 {
			_ = videoRepo.UpdatePublishSeq(ctx, videoID, seq)
			if err := feedSvc.AppendTimelineWithSeq(ctx, videoID, seq); err != nil {
				l.Warn("failed to append published video to feed timeline with seq", "video_id", videoID, "seq", seq, "error", err)
			}
		} else {
			if err := feedSvc.AppendTimeline(ctx, videoID); err != nil {
				l.Warn("failed to append published video to feed timeline", "video_id", videoID, "error", err)
			}
		}
	})

	// 9. Construct Router with probe checks
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

	// Mount media static route and domain routes
	router.MountMediaRoutes(engine, avatarStorage.BaseDir())
	router.MountWebRoutes(engine, "web")
	httpaccount.RegisterRoutes(engine, accountHandler, jwtMgr, tokenStore, rdb)
	httpvideo.RegisterRoutes(engine, videoHandler, jwtMgr, tokenStore, rdb)
	httpupload.RegisterRoutes(engine, uploadHandler, jwtMgr, tokenStore, rdb)
	httpfeed.RegisterRoutes(engine, feedHandler, jwtMgr, tokenStore, rdb)
	httpinteraction.RegisterRoutes(engine, interactionHandler, jwtMgr, tokenStore, rdb)
	httprelation.RegisterRoutes(engine, relationHandler, jwtMgr, tokenStore, rdb)
	httpnotification.RegisterRoutes(engine, notifHandler, jwtMgr, tokenStore, rdb)
	httpplayback.RegisterRoutes(engine, playbackHandler, jwtMgr, tokenStore, rdb)
	httpcreator.RegisterRoutes(engine, creatorHandler, jwtMgr, tokenStore, rdb)
	httpasset.RegisterRoutes(engine, assetHandler, jwtMgr, tokenStore, rdb)
	httpadmin.RegisterRoutes(engine, adminHandler, jwtMgr, tokenStore)

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

	// 6.5 Gracefully drain active SSE streams and notify connected clients to drift
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 3*time.Second)
	sseHub.Drain(drainCtx)
	drainCancel()
	cancelHub()
	if err := notifBroadcaster.Close(); err != nil {
		l.Warn("error closing notification broadcaster", "error", err)
	}

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
