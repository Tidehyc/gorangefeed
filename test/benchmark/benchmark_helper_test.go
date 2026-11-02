package benchmark_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpaccount "github.com/Tidehyc/gorangefeed/internal/http/account"
	httpfeed "github.com/Tidehyc/gorangefeed/internal/http/feed"
	httpinteraction "github.com/Tidehyc/gorangefeed/internal/http/interaction"
	httpnotification "github.com/Tidehyc/gorangefeed/internal/http/notification"
	httpplayback "github.com/Tidehyc/gorangefeed/internal/http/playback"
	httprelation "github.com/Tidehyc/gorangefeed/internal/http/relation"
	httpupload "github.com/Tidehyc/gorangefeed/internal/http/upload"
	httpvideo "github.com/Tidehyc/gorangefeed/internal/http/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/migration"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceaccount "github.com/Tidehyc/gorangefeed/internal/infra/persistence/account"
	persistencefeed "github.com/Tidehyc/gorangefeed/internal/infra/persistence/feed"
	persistenceinteraction "github.com/Tidehyc/gorangefeed/internal/infra/persistence/interaction"
	persistencenotification "github.com/Tidehyc/gorangefeed/internal/infra/persistence/notification"
	persistenceoutbox "github.com/Tidehyc/gorangefeed/internal/infra/persistence/outbox"
	persistencerelation "github.com/Tidehyc/gorangefeed/internal/infra/persistence/relation"
	persistenceupload "github.com/Tidehyc/gorangefeed/internal/infra/persistence/upload"
	persistencevideo "github.com/Tidehyc/gorangefeed/internal/infra/persistence/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/redis"
	"github.com/Tidehyc/gorangefeed/internal/infra/router"
	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
	serviceaccount "github.com/Tidehyc/gorangefeed/internal/service/account"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
	serviceinteraction "github.com/Tidehyc/gorangefeed/internal/service/interaction"
	servicenotification "github.com/Tidehyc/gorangefeed/internal/service/notification"
	serviceplayback "github.com/Tidehyc/gorangefeed/internal/service/playback"
	servicerelation "github.com/Tidehyc/gorangefeed/internal/service/relation"
	serviceupload "github.com/Tidehyc/gorangefeed/internal/service/upload"
	servicevideo "github.com/Tidehyc/gorangefeed/internal/service/video"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type benchmarkCtx struct {
	Engine         *gin.Engine
	DB             *gorm.DB
	RDB            *goredis.Client
	JWTMgr         *jwt.TokenManager
	TokenStore     domainaccount.TokenStore
	FeedSvc        servicefeed.Service
	HotStore       domainfeed.HotTimelineStore
	InteractionSvc *serviceinteraction.Service
	RelationSvc    *servicerelation.Service
	VideoSvc       servicevideo.Service
	OutboxRepo     domainoutbox.Repository
	VideoRepo      domainvideo.Repository
	AccountRepo    domainaccount.Repository
	PlaybackSvc    serviceplayback.Service
}

func setupBenchmark(b *testing.B) (*benchmarkCtx, func()) {
	b.Helper()

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		for _, cand := range []string{
			"../../configs/config.yaml",
			"../../../configs/config.yaml",
			"configs/config.yaml",
			"../configs/config.yaml",
		} {
			if _, err := os.Stat(cand); err == nil {
				configPath = cand
				break
			}
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		b.Skipf("skipping benchmark: failed to load config (%v)", err)
		return nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		b.Skipf("skipping benchmark: MySQL not reachable (%v)", err)
		return nil, nil
	}

	migrationsDir := "../../migrations"
	if _, err := os.Stat(migrationsDir); err != nil {
		for _, cand := range []string{"migrations", "../migrations", "../../../migrations"} {
			if _, err := os.Stat(cand); err == nil {
				migrationsDir = cand
				break
			}
		}
	}
	if _, _, err := migration.RunUp(cfg.MySQL.DSN(), migrationsDir); err != nil {
		b.Fatalf("failed to run migrations for benchmark: %v", err)
	}

	rdb, err := redis.New(&cfg.Redis)
	if err != nil {
		b.Skipf("skipping benchmark: Redis not reachable (%v)", err)
		return nil, nil
	}

	tempDir, err := os.MkdirTemp("", "gorangefeed_benchmark_*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}

	avatarStorage, _ := storage.NewAvatarStorage(tempDir)
	repo := persistenceaccount.NewRepository(db)
	tokenStore := persistenceaccount.NewTokenStore(rdb)
	jwtMgr := jwt.NewTokenManager(cfg.JWT)
	svc := serviceaccount.NewService(repo, tokenStore, jwtMgr, cfg.JWT)
	accountHandler := httpaccount.NewHandler(svc, jwtMgr, avatarStorage)

	videoStorage, _ := storage.NewVideoStorage(tempDir)
	videoRepo := persistencevideo.NewRepository(db)
	videoSvc := servicevideo.NewService(videoRepo, repo)
	videoHandler := httpvideo.NewHandler(videoSvc, videoStorage)

	uploadSessionRepo := persistenceupload.NewSessionRepository(db)
	uploadBitmapStore := persistenceupload.NewBitmapStore(rdb)
	uploadMergeLocker := persistenceupload.NewMergeLocker(rdb)
	chunkStorage, _ := storage.NewChunkStorage(tempDir)
	uploadSvc := serviceupload.NewService(uploadSessionRepo, uploadBitmapStore, uploadMergeLocker, chunkStorage)
	uploadHandler := httpupload.NewHandler(uploadSvc)

	interactionRepo := persistenceinteraction.NewRepository(db)
	relationRepo := persistencerelation.NewRepository(db)
	followingRepo := persistencefeed.NewFollowingStore(rdb, db)
	interactionSvc := serviceinteraction.NewService(interactionRepo, videoRepo, repo)
	relationSvc := servicerelation.NewService(relationRepo, repo)
	interactionHandler := httpinteraction.NewHandler(interactionSvc)
	relationHandler := httprelation.NewHandler(relationSvc)

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

	videoHandler.AddPublishHook(func(ctx context.Context, videoID int64) {
		_ = feedSvc.AppendTimeline(ctx, videoID)
	})

	gin.SetMode(gin.ReleaseMode)
	engine := router.NewRouter(router.DependencyChecks{
		CheckMySQL: func(ctx context.Context) error { return mysql.Ping(ctx, db) },
		CheckRedis: func(ctx context.Context) error { return redis.Ping(ctx, rdb) },
	})

	router.MountMediaRoutes(engine, avatarStorage.BaseDir())
	httpaccount.RegisterRoutes(engine, accountHandler, jwtMgr, tokenStore, rdb)
	httpvideo.RegisterRoutes(engine, videoHandler, jwtMgr, tokenStore, rdb)
	httpupload.RegisterRoutes(engine, uploadHandler, jwtMgr, tokenStore, rdb)
	httpfeed.RegisterRoutes(engine, feedHandler, jwtMgr, tokenStore, rdb)
	httpinteraction.RegisterRoutes(engine, interactionHandler, jwtMgr, tokenStore, rdb)
	httprelation.RegisterRoutes(engine, relationHandler, jwtMgr, tokenStore, rdb)

	notifRepo := persistencenotification.NewRepository(db)
	notifBroadcaster, _ := redis.NewRedisBroadcaster(rdb, redis.ChannelNotifications)
	notifSvc, _ := servicenotification.NewService(notifRepo, notifBroadcaster)
	sseHub, _ := servicenotification.NewSSEHub(notifRepo, notifBroadcaster, rdb)
	notifHandler := httpnotification.NewHandler(notifSvc, sseHub)
	httpnotification.RegisterRoutes(engine, notifHandler, jwtMgr, tokenStore, rdb)

	playbackSvc := serviceplayback.NewService()
	playbackHandler := httpplayback.NewHandler(playbackSvc)
	httpplayback.RegisterRoutes(engine, playbackHandler, jwtMgr, tokenStore, rdb)

	outboxRepo := persistenceoutbox.NewRepository(db)

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
		_ = rdb.Close()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	return &benchmarkCtx{
		Engine:         engine,
		DB:             db,
		RDB:            rdb,
		JWTMgr:         jwtMgr,
		TokenStore:     tokenStore,
		FeedSvc:        feedSvc,
		HotStore:       hotStore,
		InteractionSvc: interactionSvc,
		RelationSvc:    relationSvc,
		VideoSvc:       videoSvc,
		OutboxRepo:     outboxRepo,
		VideoRepo:      videoRepo,
		AccountRepo:    repo,
		PlaybackSvc:    playbackSvc,
	}, cleanup
}

// prewarmFeedData seeds test videos and warms caches for benchmark runs.
func prewarmFeedData(ctx context.Context, bCtx *benchmarkCtx, count int) ([]int64, int64) {
	suffix := time.Now().Format("150405_000")
	author := &domainaccount.User{
		Username:     fmt.Sprintf("bench_user_%s", suffix),
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuvwxyz1234567890",
		Nickname:     "Bench Author",
		Status:       domainaccount.UserStatusActive,
	}
	_ = bCtx.AccountRepo.Create(ctx, author)

	videoIDs := make([]int64, 0, count)
	now := time.Now().UTC()

	for i := 1; i <= count; i++ {
		v := &domainvideo.Video{
			AuthorID:    author.ID,
			Title:       fmt.Sprintf("Benchmark Video %d", i),
			Description: "Benchmark payload",
			PlayURL:     fmt.Sprintf("/media/videos/bench_%d.mp4", i),
			DurationMs:  15000,
			Status:      domainvideo.VideoStatusPublished,
			PublishedAt: now.Add(time.Duration(i) * time.Second),
			CreatedAt:   now.Add(time.Duration(i) * time.Second),
			UpdatedAt:   now.Add(time.Duration(i) * time.Second),
		}
		stats := &domainvideo.VideoStats{}
		_ = bCtx.VideoRepo.CreateWithStats(ctx, v, stats)
		videoIDs = append(videoIDs, v.ID)
		_ = bCtx.FeedSvc.AppendTimeline(ctx, v.ID)
	}

	return videoIDs, author.ID
}
