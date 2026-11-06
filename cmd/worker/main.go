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
	"github.com/Tidehyc/gorangefeed/internal/infra/outbox"
	persistenceaccount "github.com/Tidehyc/gorangefeed/internal/infra/persistence/account"
	persistencecreator "github.com/Tidehyc/gorangefeed/internal/infra/persistence/creator"
	persistencefeed "github.com/Tidehyc/gorangefeed/internal/infra/persistence/feed"
	persistencenotification "github.com/Tidehyc/gorangefeed/internal/infra/persistence/notification"
	persistenceoutbox "github.com/Tidehyc/gorangefeed/internal/infra/persistence/outbox"
	persistencerelation "github.com/Tidehyc/gorangefeed/internal/infra/persistence/relation"
	persistencevideo "github.com/Tidehyc/gorangefeed/internal/infra/persistence/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	"github.com/Tidehyc/gorangefeed/internal/infra/redis"
	infrascheduler "github.com/Tidehyc/gorangefeed/internal/infra/scheduler"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
	servicenotification "github.com/Tidehyc/gorangefeed/internal/service/notification"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	amqp "github.com/rabbitmq/amqp091-go"
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

	// 6. Initialize Feed service and periodic timeline maintenance
	timelineRepo := persistencefeed.NewTimelineStore(rdb)
	pageCache := persistencefeed.NewPageCacheRepository(rdb)
	cardCache := persistencefeed.NewCardCacheRepository(rdb)
	statCache := persistencefeed.NewStatCacheRepository(rdb)
	exposureRepo := persistencefeed.NewExposureRepository(rdb)
	feedDBRepo := persistencefeed.NewFeedDBRepository(db)
	feedSvc := servicefeed.NewService(timelineRepo, pageCache, cardCache, statCache, exposureRepo, feedDBRepo)

	// Periodic Feed timeline maintenance (SyncFloorAndTrim)
	// Trims videos older than 72 hours from Redis and synchronizes timeline:floor
	go func() {
		// Run once on worker startup
		startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if floor, err := feedSvc.SyncFloorAndTrim(startupCtx); err != nil {
			l.Warn("initial sync floor and trim error", "error", err)
		} else {
			l.Info("initial timeline floor synced", "floor", floor)
		}
		cancel()

		trimTicker := time.NewTicker(10 * time.Minute)
		defer trimTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				l.Info("timeline trimmer loop exiting")
				return
			case <-trimTicker.C:
				trimCtx, trimCancel := context.WithTimeout(context.Background(), 30*time.Second)
				floor, err := feedSvc.SyncFloorAndTrim(trimCtx)
				trimCancel()
				if err != nil {
					l.Warn("periodic sync floor and trim error", "error", err)
				} else {
					l.Info("periodic timeline floor synced and trimmed", "floor", floor)
				}
			}
		}
	}()

	// 7. Initialize Outbox Lease Scheduler
	outboxRepo := persistenceoutbox.NewRepository(db)
	outboxPublisher, err := rabbitmq.NewPublisher(mqConn, rabbitmq.ExchangeEvents)
	if err != nil {
		l.Error("worker failed to initialize rabbitmq publisher", "error", err)
		os.Exit(1)
	}
	outboxScheduler, err := outbox.NewScheduler(outboxRepo, outboxPublisher)
	if err != nil {
		l.Error("worker failed to initialize outbox scheduler", "error", err)
		os.Exit(1)
	}
	if err := outboxScheduler.Start(ctx); err != nil {
		l.Error("worker failed to start outbox scheduler", "error", err)
		os.Exit(1)
	}
	l.Info("outbox scheduler started successfully")

	// 8. Initialize RabbitMQ Consumer for Following Feed Fanout
	deduplicator := rabbitmq.NewDeduplicator(db)

	// consumed_events retention: a marker is only useful for as long as the broker might redeliver
	// the same message. Older rows are dead weight that grows without bound.
	go func() {
		const (
			retention     = 7 * 24 * time.Hour
			sweepInterval = time.Hour
		)
		purge := func() {
			purgeCtx, purgeCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer purgeCancel()
			removed, err := deduplicator.PurgeConsumedBefore(purgeCtx, time.Now().Add(-retention))
			if err != nil {
				l.Warn("failed to purge consumed_events", "error", err)
				return
			}
			if removed > 0 {
				l.Info("purged consumed_events rows past retention", "rows", removed, "retention", retention.String())
			}
		}

		purge()
		purgeTicker := time.NewTicker(sweepInterval)
		defer purgeTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				l.Info("consumed_events purger loop exiting")
				return
			case <-purgeTicker.C:
				purge()
			}
		}
	}()

	relationRepo := persistencerelation.NewRepository(db)
	followingRepo := persistencefeed.NewFollowingStore(rdb, db)
	fanoutSvc, err := servicefeed.NewFanoutService(relationRepo, followingRepo, deduplicator)
	if err != nil {
		l.Error("worker failed to initialize fanout service", "error", err)
		os.Exit(1)
	}

	followingConsumer, err := rabbitmq.NewConsumer(
		mqConn,
		rabbitmq.ExchangeEvents,
		rabbitmq.QueueFollowingFeed,
		[]string{rabbitmq.RoutingKeyVideoPublished},
		20,
	)
	if err != nil {
		l.Error("worker failed to initialize following consumer", "error", err)
		os.Exit(1)
	}

	go func() {
		l.Info("starting following feed consumer", "queue", rabbitmq.QueueFollowingFeed)
		err := followingConsumer.Start(ctx, func(msgCtx context.Context, d amqp.Delivery) error {
			eventID := d.MessageId
			if eventID == "" {
				if val, ok := d.Headers["event_id"].(string); ok {
					eventID = val
				}
			}
			return fanoutSvc.HandleVideoPublished(msgCtx, eventID, d.Body)
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			l.Error("following feed consumer encountered error", "error", err)
		}
	}()

	// 8.1 Initialize RabbitMQ Consumer for Real-Time Hot Rank Engagement Scoring
	hotStore := persistencefeed.NewHotStore(rdb)
	hotScoreConsumer, err := servicefeed.NewHotScoreConsumer(hotStore, deduplicator)
	if err != nil {
		l.Error("worker failed to initialize hot score consumer", "error", err)
		os.Exit(1)
	}

	hotRankConsumer, err := rabbitmq.NewConsumer(
		mqConn,
		rabbitmq.ExchangeEvents,
		rabbitmq.QueueHotRank,
		[]string{
			rabbitmq.RoutingKeyInteractionLiked,
			rabbitmq.RoutingKeyInteractionUnliked,
			rabbitmq.RoutingKeyInteractionCommented,
			rabbitmq.RoutingKeyInteractionCommentDelete,
			rabbitmq.RoutingKeyVideoViewed,
			rabbitmq.RoutingKeyVideoCompleted,
			rabbitmq.RoutingKeyInteractionFavorited,
			rabbitmq.RoutingKeyInteractionUnfavorited,
		},
		20,
	)
	if err != nil {
		l.Error("worker failed to initialize hot rank consumer", "error", err)
		os.Exit(1)
	}

	go func() {
		l.Info("starting hot rank consumer", "queue", rabbitmq.QueueHotRank)
		err := hotRankConsumer.Start(ctx, func(msgCtx context.Context, d amqp.Delivery) error {
			eventID := d.MessageId
			if eventID == "" {
				if val, ok := d.Headers["event_id"].(string); ok {
					eventID = val
				}
			}
			return hotScoreConsumer.HandleEngagementEvent(msgCtx, eventID, d.RoutingKey, d.Body)
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			l.Error("hot rank consumer encountered error", "error", err)
		}
	}()

	// 8.2 Initialize RabbitMQ Consumer for System Notifications & Redis Pub/Sub Broadcast
	notifBroadcaster, err := redis.NewRedisBroadcaster(rdb, redis.ChannelNotifications)
	if err != nil {
		l.Error("worker failed to initialize notification redis broadcaster", "error", err)
		os.Exit(1)
	}
	accountRepo := persistenceaccount.NewRepository(db)
	notifRepo := persistencenotification.NewRepository(db)
	notifConsumer, err := servicenotification.NewConsumer(notifRepo, notifBroadcaster, deduplicator, accountRepo)
	if err != nil {
		l.Error("worker failed to initialize notification consumer", "error", err)
		os.Exit(1)
	}

	notifMQConsumer, err := rabbitmq.NewConsumer(
		mqConn,
		rabbitmq.ExchangeEvents,
		rabbitmq.QueueNotification,
		[]string{
			rabbitmq.RoutingKeyInteractionLiked,
			rabbitmq.RoutingKeyInteractionCommented,
			rabbitmq.RoutingKeyRelationFollowed,
		},
		20,
	)
	if err != nil {
		l.Error("worker failed to initialize notification rabbitmq consumer", "error", err)
		os.Exit(1)
	}

	go func() {
		l.Info("starting notification consumer", "queue", rabbitmq.QueueNotification)
		err := notifMQConsumer.Start(ctx, func(msgCtx context.Context, d amqp.Delivery) error {
			eventID := d.MessageId
			if eventID == "" {
				if val, ok := d.Headers["event_id"].(string); ok {
					eventID = val
				}
			}
			return notifConsumer.HandleEvent(msgCtx, eventID, d.RoutingKey, d.Body)
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			l.Error("notification consumer encountered error", "error", err)
		}
	}()

	// 8.3 Initialize Creator Center Delayed Scheduler
	draftRepo := persistencecreator.NewDraftRepository(db)
	taskRepo := persistencecreator.NewScheduledTaskRepository(db)
	videoRepo := persistencevideo.NewRepository(db)
	seqGen := redis.NewSequenceGenerator(rdb)
	var maxPublishSeq int64
	_ = db.Table("videos").Select("COALESCE(MAX(publish_seq), 0)").Scan(&maxPublishSeq).Error
	_ = seqGen.InitializeSequence(ctx, maxPublishSeq)
	delayedScheduler := infrascheduler.NewDelayedScheduler(rdb, taskRepo, videoRepo, draftRepo, seqGen, timelineRepo, pageCache, nil)
	delayedScheduler.Start(ctx)
	l.Info("creator delayed scheduler started successfully")

	l.Info("Worker service running and ready for event dispatching and consumption")

	// 9. Signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	l.Info("worker received termination signal", "signal", sig.String())

	// 10. Stop worker processing and outbox scheduler
	stopWorker()
	delayedScheduler.Stop()
	outboxScheduler.Stop()

	metricsCtx, metricsCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer metricsCancel()
	if err := metricsSrv.Shutdown(metricsCtx); err != nil {
		l.Warn("error shutting down worker metrics server", "error", err)
	}

	// 11. Cleanup resources
	if err := notifBroadcaster.Close(); err != nil {
		l.Warn("error closing notification broadcaster", "error", err)
	}
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
