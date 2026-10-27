package feed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	"golang.org/x/sync/singleflight"
)

// Service defines application use cases for feed querying and telemetry.
type Service interface {
	// GetGlobalFeed retrieves a page of the global video feed with multi-tier caching and cold-hot stitching.
	GetGlobalFeed(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error)
	// RecordView reports a video playback or exposure impression, incrementing view count and updating exposure history.
	RecordView(ctx context.Context, viewerID int64, videoID int64, event domainfeed.ViewEvent) error
	// AppendTimeline appends a newly published video to the Redis timeline.
	AppendTimeline(ctx context.Context, videoID int64) error
	// SyncFloorAndTrim computes the 72h floor from MySQL and trims the Redis timeline atomically.
	SyncFloorAndTrim(ctx context.Context) (int64, error)
}

type service struct {
	timelineRepo domainfeed.TimelineRepository
	pageCache    domainfeed.PageCacheRepository
	cardCache    domainfeed.CardCacheRepository
	statCache    domainfeed.StatCacheRepository
	exposureRepo domainfeed.ExposureRepository
	dbRepo       domainfeed.FeedDBRepository
	sfGroup      singleflight.Group
}

// NewService creates a new Feed Service instance.
func NewService(
	timelineRepo domainfeed.TimelineRepository,
	pageCache domainfeed.PageCacheRepository,
	cardCache domainfeed.CardCacheRepository,
	statCache domainfeed.StatCacheRepository,
	exposureRepo domainfeed.ExposureRepository,
	dbRepo domainfeed.FeedDBRepository,
) Service {
	return &service{
		timelineRepo: timelineRepo,
		pageCache:    pageCache,
		cardCache:    cardCache,
		statCache:    statCache,
		exposureRepo: exposureRepo,
		dbRepo:       dbRepo,
	}
}

func (s *service) GetGlobalFeed(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error) {
	if cursor < 0 {
		return nil, domainfeed.ErrInvalidCursor
	}
	if limit <= 0 {
		limit = domainfeed.DefaultLimit
	}
	if limit > domainfeed.MaxLimit {
		limit = domainfeed.MaxLimit
	}

	viewerKey := "anon"
	if viewerID > 0 {
		viewerKey = fmt.Sprintf("uid:%d", viewerID)
	}
	cacheKey := fmt.Sprintf("feed:page:global:%s:%d:%d:v1", viewerKey, cursor, limit)

	// 1. Try reading from L1 Page Cache
	if s.pageCache != nil {
		cachedPage, found, err := s.pageCache.GetPage(ctx, cacheKey)
		if err == nil && found && cachedPage != nil {
			now := time.Now()
			if !cachedPage.IsExpired(now) {
				// L1 Cache Hit: Return immediately with hydrated items
				return s.hydrateFeedPage(ctx, cachedPage, viewerID)
			}
			if cachedPage.CanServeStale(now) {
				// L1 Stale Hit: Asynchronously trigger refresh via singleflight, serve stale page immediately
				go func() {
					bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_, _, _ = s.sfGroup.Do(cacheKey, func() (interface{}, error) {
						return s.buildAndCachePage(bgCtx, viewerKey, cacheKey, cursor, limit)
					})
				}()
				return s.hydrateFeedPage(ctx, cachedPage, viewerID)
			}
		}
	}

	// 2. L1 Cache Miss: Execute singleflight deduplication across concurrent requests
	val, err, _ := s.sfGroup.Do(cacheKey, func() (interface{}, error) {
		return s.buildAndCachePage(ctx, viewerKey, cacheKey, cursor, limit)
	})
	if err != nil {
		return nil, err
	}

	cachedPage, ok := val.(*domainfeed.CachedPage)
	if !ok || cachedPage == nil {
		return nil, domainfeed.ErrFeedEmpty
	}

	return s.hydrateFeedPage(ctx, cachedPage, viewerID)
}

func (s *service) buildAndCachePage(ctx context.Context, viewerKey, cacheKey string, cursor int64, limit int) (*domainfeed.CachedPage, error) {
	// Retrieve floor boundary
	var floor int64
	var floorErr error
	if s.timelineRepo != nil {
		floor, floorErr = s.timelineRepo.GetFloor(ctx)
	}

	// If Redis is unreachable, fallback to MySQL cold path
	if floorErr != nil {
		slog.Warn("failed to get timeline floor from redis, falling back to mysql", "error", floorErr)
		return s.buildColdPage(ctx, cacheKey, cursor, limit)
	}

	// Initialize floor if zero
	if floor == 0 {
		floor, _ = s.dbRepo.CalculateFloor(ctx, 72*time.Hour)
		if floor > 0 && s.timelineRepo != nil {
			_ = s.timelineRepo.SetFloor(ctx, floor)
		}
	}

	// Case A: Pure cold request (cursor is older than or equal to floor)
	if cursor > 0 && floor > 0 && cursor <= floor {
		return s.buildColdPage(ctx, cacheKey, cursor, limit)
	}

	// Case B: Hot request (or hot-to-cold boundary crossing)
	if s.timelineRepo == nil {
		return s.buildColdPage(ctx, cacheKey, cursor, limit)
	}

	validVideoIDs := make([]int64, 0, limit)
	scanCursor := cursor
	totalScanned := 0
	lastScannedID := int64(0)
	hitZSetEnd := false

	// Scan Redis ZSet with deleted video skipping
	for len(validVideoIDs) < limit && totalScanned < domainfeed.MaxScanLimit {
		batchSize := limit * 2
		if batchSize < 50 {
			batchSize = 50
		}

		ids, err := s.timelineRepo.ListByCursor(ctx, scanCursor, batchSize)
		if err != nil {
			slog.Warn("timeline scan failed, falling back to db", "error", err)
			return s.buildColdPage(ctx, cacheKey, cursor, limit)
		}

		if len(ids) == 0 {
			hitZSetEnd = true
			break
		}

		// Verify publication status to eliminate taken-down videos
		statusMap, err := s.dbRepo.GetVideosStatusByIDs(ctx, ids)
		if err != nil {
			return s.buildColdPage(ctx, cacheKey, cursor, limit)
		}

		for _, id := range ids {
			totalScanned++
			lastScannedID = id

			if statusMap[id] == 1 { // published
				validVideoIDs = append(validVideoIDs, id)
				if len(validVideoIDs) >= limit {
					break
				}
			}
			if totalScanned >= domainfeed.MaxScanLimit {
				break
			}
		}

		if len(validVideoIDs) >= limit || totalScanned >= domainfeed.MaxScanLimit {
			break
		}

		scanCursor = lastScannedID
		if len(ids) < batchSize {
			hitZSetEnd = true
			break
		}
	}

	// Case C: Hot/Cold Splicing (热区不足时回源 MySQL 补齐差额)
	if len(validVideoIDs) < limit && (hitZSetEnd || (lastScannedID > 0 && floor > 0 && lastScannedID <= floor)) {
		remainLimit := limit - len(validVideoIDs)
		coldCursor := lastScannedID
		if coldCursor == 0 {
			coldCursor = cursor
		}

		coldCards, coldStats, coldNextCursor, _, err := s.dbRepo.ListColdVideos(ctx, coldCursor, remainLimit)
		if err == nil && len(coldCards) > 0 {
			for _, card := range coldCards {
				validVideoIDs = append(validVideoIDs, card.VideoID)
			}
			lastScannedID = coldNextCursor

			// Asynchronously populate L3/L4 caches with cold results
			go func(cards []*domainfeed.FeedCard, stats []*domainfeed.FeedStat) {
				if s.cardCache != nil && len(cards) > 0 {
					_ = s.cardCache.SetCards(context.Background(), cards, domainfeed.L3CardTTL)
				}
				if s.statCache != nil && len(stats) > 0 {
					_ = s.statCache.SetStats(context.Background(), stats, domainfeed.L4StatTTL)
				}
			}(coldCards, coldStats)
		}
	}

	hasMore := len(validVideoIDs) >= limit
	if totalScanned >= domainfeed.MaxScanLimit {
		hasMore = true
	}
	nextCursor := lastScannedID
	if len(validVideoIDs) == 0 {
		hasMore = false
		nextCursor = 0
	}

	// Determine cache expiration
	ttl := domainfeed.L1PaginationTTL
	if cursor == 0 {
		ttl = domainfeed.L1FirstPageTTL
	}

	cachedPage := &domainfeed.CachedPage{
		VideoIDs:   validVideoIDs,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		ExpireAt:   time.Now().Add(ttl),
	}

	if s.pageCache != nil {
		_ = s.pageCache.SetPage(ctx, cacheKey, cachedPage, ttl+domainfeed.StaleCacheTolerance)
	}

	return cachedPage, nil
}

func (s *service) buildColdPage(ctx context.Context, cacheKey string, cursor int64, limit int) (*domainfeed.CachedPage, error) {
	cards, stats, nextCursor, hasMore, err := s.dbRepo.ListColdVideos(ctx, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list cold videos from db error: %w", err)
	}

	videoIDs := make([]int64, len(cards))
	for i, c := range cards {
		videoIDs[i] = c.VideoID
	}

	// Asynchronously backfill L3 and L4
	go func(c []*domainfeed.FeedCard, st []*domainfeed.FeedStat) {
		if s.cardCache != nil && len(c) > 0 {
			_ = s.cardCache.SetCards(context.Background(), c, domainfeed.L3CardTTL)
		}
		if s.statCache != nil && len(st) > 0 {
			_ = s.statCache.SetStats(context.Background(), st, domainfeed.L4StatTTL)
		}
	}(cards, stats)

	ttl := domainfeed.L1PaginationTTL
	if cursor == 0 {
		ttl = domainfeed.L1FirstPageTTL
	}

	cachedPage := &domainfeed.CachedPage{
		VideoIDs:   videoIDs,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		ExpireAt:   time.Now().Add(ttl),
	}

	if s.pageCache != nil {
		_ = s.pageCache.SetPage(ctx, cacheKey, cachedPage, ttl+domainfeed.StaleCacheTolerance)
	}

	return cachedPage, nil
}

func (s *service) hydrateFeedPage(ctx context.Context, cachedPage *domainfeed.CachedPage, viewerID int64) (*domainfeed.FeedPage, error) {
	if len(cachedPage.VideoIDs) == 0 {
		return &domainfeed.FeedPage{
			Items:      []*domainfeed.FeedItem{},
			NextCursor: cachedPage.NextCursor,
			HasMore:    cachedPage.HasMore,
		}, nil
	}

	// 1. Fetch L3 Cards
	cardMap := make(map[int64]*domainfeed.FeedCard)
	if s.cardCache != nil {
		cardMap, _ = s.cardCache.GetCards(ctx, cachedPage.VideoIDs)
	}

	missingCardIDs := make([]int64, 0)
	for _, id := range cachedPage.VideoIDs {
		if _, ok := cardMap[id]; !ok {
			missingCardIDs = append(missingCardIDs, id)
		}
	}

	if len(missingCardIDs) > 0 {
		dbCards, err := s.dbRepo.GetCardsByIDs(ctx, missingCardIDs)
		if err == nil {
			cardsToBackfill := make([]*domainfeed.FeedCard, 0, len(dbCards))
			for id, card := range dbCards {
				cardMap[id] = card
				cardsToBackfill = append(cardsToBackfill, card)
			}
			if s.cardCache != nil && len(cardsToBackfill) > 0 {
				go func(cards []*domainfeed.FeedCard) {
					_ = s.cardCache.SetCards(context.Background(), cards, domainfeed.L3CardTTL)
				}(cardsToBackfill)
			}
		}
	}

	// 2. Fetch L4 Stats
	statMap := make(map[int64]*domainfeed.FeedStat)
	if s.statCache != nil {
		statMap, _ = s.statCache.GetStats(ctx, cachedPage.VideoIDs)
	}

	missingStatIDs := make([]int64, 0)
	for _, id := range cachedPage.VideoIDs {
		if _, ok := statMap[id]; !ok {
			missingStatIDs = append(missingStatIDs, id)
		}
	}

	if len(missingStatIDs) > 0 {
		dbStats, err := s.dbRepo.GetStatsByIDs(ctx, missingStatIDs)
		if err == nil {
			statsToBackfill := make([]*domainfeed.FeedStat, 0, len(dbStats))
			for id, stat := range dbStats {
				statMap[id] = stat
				statsToBackfill = append(statsToBackfill, stat)
			}
			if s.statCache != nil && len(statsToBackfill) > 0 {
				go func(stats []*domainfeed.FeedStat) {
					_ = s.statCache.SetStats(context.Background(), stats, domainfeed.L4StatTTL)
				}(statsToBackfill)
			}
		}
	}

	// 3. Assemble final FeedItems in original order
	items := make([]*domainfeed.FeedItem, 0, len(cachedPage.VideoIDs))
	for _, id := range cachedPage.VideoIDs {
		card, ok := cardMap[id]
		if !ok || card == nil {
			continue
		}

		stat := domainfeed.FeedStat{VideoID: id}
		if s, ok := statMap[id]; ok && s != nil {
			stat = *s
		}

		items = append(items, &domainfeed.FeedItem{
			ID:          id,
			AuthorID:    card.AuthorID,
			Title:       card.Title,
			Description: card.Description,
			CoverURL:    card.CoverURL,
			PlayURL:     card.PlayURL,
			DurationMs:  card.DurationMs,
			PublishedAt: card.PublishedAt,
			Author:      card.Author,
			Stats:       stat,
			IsLiked:     false, // M4 foundation
		})
	}

	return &domainfeed.FeedPage{
		Items:      items,
		NextCursor: cachedPage.NextCursor,
		HasMore:    cachedPage.HasMore,
	}, nil
}

func (s *service) RecordView(ctx context.Context, viewerID int64, videoID int64, event domainfeed.ViewEvent) error {
	if videoID <= 0 {
		return domainfeed.ErrInvalidVideoID
	}
	if err := event.Validate(); err != nil {
		return err
	}

	// 1. Atomically increment video_stats.view_count in MySQL
	if err := s.dbRepo.IncrViewCount(ctx, videoID); err != nil {
		return err
	}

	// 2. Invalidate L4 stat cache so next read retrieves fresh count
	if s.statCache != nil {
		_ = s.statCache.InvalidateStat(ctx, videoID)
	}

	// 3. Record impression in Redis user exposure history for recommend deduplication
	if viewerID > 0 && s.exposureRepo != nil {
		_ = s.exposureRepo.RecordExposures(ctx, viewerID, []int64{videoID})
	}

	return nil
}

func (s *service) AppendTimeline(ctx context.Context, videoID int64) error {
	if videoID <= 0 {
		return domainfeed.ErrInvalidVideoID
	}
	if s.timelineRepo != nil {
		return s.timelineRepo.Append(ctx, videoID)
	}
	return nil
}

func (s *service) SyncFloorAndTrim(ctx context.Context) (int64, error) {
	floor, err := s.dbRepo.CalculateFloor(ctx, 72*time.Hour)
	if err != nil {
		return 0, fmt.Errorf("calculate floor error: %w", err)
	}
	if floor > 0 && s.timelineRepo != nil {
		if err := s.timelineRepo.Trim(ctx, floor); err != nil {
			return floor, fmt.Errorf("trim timeline error: %w", err)
		}
	}
	return floor, nil
}
