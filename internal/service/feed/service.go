package feed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	"golang.org/x/sync/singleflight"
)

// Service defines application use cases for feed querying and telemetry.
type Service interface {
	// GetGlobalFeed retrieves a page of the global video feed with multi-tier caching and cold-hot stitching.
	GetGlobalFeed(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error)
	// ListFollowingFeed retrieves a page of the following feed using hybrid push-pull multi-way merge sort.
	ListFollowingFeed(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error)
	// GetLikeRankFeed retrieves videos ordered by historical like count using composite cursor.
	GetLikeRankFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.LikeRankPage, error)
	// GetHotFeed retrieves a page of the hot video feed based on 60-minute sliding window aggregation and as_of snapshot.
	GetHotFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.HotFeedPage, error)
	// ListHotFeed is an alias for GetHotFeed.
	ListHotFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.HotFeedPage, error)
	// GetRecommendFeed retrieves personalized or guest recommended videos via multi-source recall, exposure filtering and author diversification.
	GetRecommendFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.RecommendFeedPage, error)
	// ListRecommendFeed is an alias for GetRecommendFeed.
	ListRecommendFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.RecommendFeedPage, error)
	// RecordView reports a video playback or exposure impression, incrementing view count and updating exposure history.
	RecordView(ctx context.Context, viewerID int64, videoID int64, event domainfeed.ViewEvent) error
	// AppendTimeline appends a newly published video to the Redis timeline.
	AppendTimeline(ctx context.Context, videoID int64) error
	// AppendTimelineWithSeq appends a newly published video with monotonic publish sequence.
	AppendTimelineWithSeq(ctx context.Context, videoID int64, seq int64) error
	// SyncFloorAndTrim computes the 72h floor from MySQL and trims the Redis timeline atomically.
	SyncFloorAndTrim(ctx context.Context) (int64, error)
}

type service struct {
	timelineRepo    domainfeed.TimelineRepository
	pageCache       domainfeed.PageCacheRepository
	cardCache       domainfeed.CardCacheRepository
	statCache       domainfeed.StatCacheRepository
	exposureRepo    domainfeed.ExposureRepository
	dbRepo          domainfeed.FeedDBRepository
	followingRepo   domainfeed.FollowingFeedRepository
	relationRepo    domainrelation.Repository
	interactionRepo domaininteraction.Repository
	hotStore        domainfeed.HotTimelineStore
	recallEngine    domainfeed.RecallEngine
	diversifier     domainfeed.AuthorDiversifier
	sfGroup         singleflight.Group
}

// Option modifies service configuration.
type Option func(*service)

// WithFollowingDependencies injects dependencies for following feed support.
func WithFollowingDependencies(
	followingRepo domainfeed.FollowingFeedRepository,
	relationRepo domainrelation.Repository,
	interactionRepo domaininteraction.Repository,
) Option {
	return func(s *service) {
		s.followingRepo = followingRepo
		s.relationRepo = relationRepo
		s.interactionRepo = interactionRepo
	}
}

// WithHotTimelineStore injects the hot timeline store for sliding hot feed.
func WithHotTimelineStore(hotStore domainfeed.HotTimelineStore) Option {
	return func(s *service) {
		s.hotStore = hotStore
	}
}

// WithRecommendDependencies injects recall engine and author diversifier for recommendation feed.
func WithRecommendDependencies(recallEngine domainfeed.RecallEngine, diversifier domainfeed.AuthorDiversifier) Option {
	return func(s *service) {
		s.recallEngine = recallEngine
		s.diversifier = diversifier
	}
}


// NewService creates a new Feed Service instance.
func NewService(
	timelineRepo domainfeed.TimelineRepository,
	pageCache domainfeed.PageCacheRepository,
	cardCache domainfeed.CardCacheRepository,
	statCache domainfeed.StatCacheRepository,
	exposureRepo domainfeed.ExposureRepository,
	dbRepo domainfeed.FeedDBRepository,
	opts ...Option,
) Service {
	s := &service{
		timelineRepo: timelineRepo,
		pageCache:    pageCache,
		cardCache:    cardCache,
		statCache:    statCache,
		exposureRepo: exposureRepo,
		dbRepo:       dbRepo,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

	// M4-09 Architecture Evolution: L1 page cache stores shared public pure-ID skeleton.
	// All visitors (anon and logged-in) share the exact same L1 key and Singleflight across platform.
	cacheKey := fmt.Sprintf("feed:page:global:public:%d:%d:v1", cursor, limit)

	// 1. Try reading from L1 Page Cache
	if s.pageCache != nil {
		cachedPage, found, err := s.pageCache.GetPage(ctx, cacheKey)
		if err == nil && found && cachedPage != nil {
			now := time.Now()
			if !cachedPage.IsExpired(now) {
				// L1 Cache Hit: Return immediately with dynamic hydrated items
				return s.hydrateFeedPage(ctx, cachedPage, viewerID)
			}
			if cachedPage.CanServeStale(now) {
				// L1 Stale Hit: Asynchronously trigger refresh via singleflight, serve stale page immediately
				go func() {
					bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_, _, _ = s.sfGroup.Do(cacheKey, func() (interface{}, error) {
						return s.buildAndCachePage(bgCtx, cacheKey, cursor, limit)
					})
				}()
				return s.hydrateFeedPage(ctx, cachedPage, viewerID)
			}
		}
	}

	// 2. L1 Cache Miss: Execute singleflight deduplication across concurrent requests
	val, err, _ := s.sfGroup.Do(cacheKey, func() (interface{}, error) {
		return s.buildAndCachePage(ctx, cacheKey, cursor, limit)
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

func (s *service) buildAndCachePage(ctx context.Context, cacheKey string, cursor int64, limit int) (*domainfeed.CachedPage, error) {
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

func (s *service) ListFollowingFeed(ctx context.Context, viewerID int64, cursor int64, limit int) (*domainfeed.FeedPage, error) {
	if viewerID <= 0 {
		return nil, domainfeed.ErrInvalidViewerID
	}
	if cursor < 0 {
		return nil, domainfeed.ErrInvalidCursor
	}
	if limit <= 0 {
		limit = domainfeed.DefaultLimit
	}
	if limit > domainfeed.MaxLimit {
		limit = domainfeed.MaxLimit
	}

	if s.relationRepo == nil || s.followingRepo == nil {
		return nil, errors.New("following feed dependencies not configured")
	}

	// 1. Get all authors viewer follows (cursor 0, limit 1000)
	vloggerIDs, _, _, err := s.relationRepo.ListFollowingIDs(ctx, viewerID, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list following IDs: %w", err)
	}
	if len(vloggerIDs) == 0 {
		return &domainfeed.FeedPage{
			Items:      []*domainfeed.FeedItem{},
			NextCursor: 0,
			HasMore:    false,
		}, nil
	}

	// 2. Partition followed authors into normal authors and Big V authors (followers >= 5000)
	var normalAuthors []int64
	var bigVAuthors []int64

	for _, aid := range vloggerIDs {
		stats, err := s.relationRepo.GetStats(ctx, aid)
		if err == nil && stats != nil && stats.FollowerCount >= BigVThreshold {
			bigVAuthors = append(bigVAuthors, aid)
		} else {
			normalAuthors = append(normalAuthors, aid)
		}
	}

	// 3. Multi-source candidate video gathering
	candidateMap := make(map[int64]struct{})
	fetchLimit := limit + 1

	// 3a. Read from followed Big V authors' Outbox ZSets
	for _, bigVAid := range bigVAuthors {
		outboxIDs, err := s.followingRepo.GetAuthorOutboxVideoIDs(ctx, bigVAid, cursor, fetchLimit)
		if err == nil {
			for _, id := range outboxIDs {
				candidateMap[id] = struct{}{}
			}
		}
	}

	// 3b. Read normal authors' videos from viewer's Inbox ZSet or fallback to DB
	if len(normalAuthors) > 0 {
		inboxExists, _ := s.followingRepo.CheckInboxExists(ctx, viewerID)
		if inboxExists {
			inboxIDs, err := s.followingRepo.GetInboxVideoIDs(ctx, viewerID, cursor, fetchLimit)
			if err == nil {
				for _, id := range inboxIDs {
					candidateMap[id] = struct{}{}
				}
			}
		} else {
			// Cold start / Redis eviction: Fallback to MySQL query
			dbIDs, err := s.followingRepo.ListFollowingVideosFromDB(ctx, normalAuthors, cursor, fetchLimit)
			if err == nil {
				for _, id := range dbIDs {
					candidateMap[id] = struct{}{}
				}
			}
			// Trigger asynchronous inbox pre-warming
			go func(vID int64, nAuthors []int64) {
				bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				recentIDs, err := s.followingRepo.ListFollowingVideosFromDB(bgCtx, nAuthors, 0, 100)
				if err == nil && len(recentIDs) > 0 {
					_ = s.followingRepo.PrewarmInbox(bgCtx, vID, recentIDs, 800)
				}
			}(viewerID, normalAuthors)
		}
	}

	if len(candidateMap) == 0 {
		return &domainfeed.FeedPage{
			Items:      []*domainfeed.FeedItem{},
			NextCursor: 0,
			HasMore:    false,
		}, nil
	}

	// 4. Multi-way merge sort & cursor pagination
	sortedIDs := make([]int64, 0, len(candidateMap))
	for id := range candidateMap {
		if cursor > 0 && id >= cursor {
			continue
		}
		sortedIDs = append(sortedIDs, id)
	}

	sort.Slice(sortedIDs, func(i, j int) bool {
		return sortedIDs[i] > sortedIDs[j]
	})

	hasMore := false
	var nextCursor int64 = 0
	if len(sortedIDs) > limit {
		hasMore = true
		nextCursor = sortedIDs[limit-1]
		sortedIDs = sortedIDs[:limit]
	}

	// 5. Hydrate items (IsFollowed = true)
	items, err := s.hydrateVideoIDs(ctx, sortedIDs, viewerID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate following feed items: %w", err)
	}

	return &domainfeed.FeedPage{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *service) hydrateFeedPage(ctx context.Context, cachedPage *domainfeed.CachedPage, viewerID int64) (*domainfeed.FeedPage, error) {
	if cachedPage == nil || len(cachedPage.VideoIDs) == 0 {
		return &domainfeed.FeedPage{
			Items:      []*domainfeed.FeedItem{},
			NextCursor: cachedPage.NextCursor,
			HasMore:    cachedPage.HasMore,
		}, nil
	}

	items, err := s.hydrateVideoIDs(ctx, cachedPage.VideoIDs, viewerID, false)
	if err != nil {
		return nil, err
	}

	return &domainfeed.FeedPage{
		Items:      items,
		NextCursor: cachedPage.NextCursor,
		HasMore:    cachedPage.HasMore,
	}, nil
}

func (s *service) hydrateVideoIDs(ctx context.Context, videoIDs []int64, viewerID int64, isFollowed bool) ([]*domainfeed.FeedItem, error) {
	if len(videoIDs) == 0 {
		return []*domainfeed.FeedItem{}, nil
	}

	// 1. Fetch L3 Cards
	cardMap := make(map[int64]*domainfeed.FeedCard)
	if s.cardCache != nil {
		cardMap, _ = s.cardCache.GetCards(ctx, videoIDs)
	}

	missingCardIDs := make([]int64, 0)
	for _, id := range videoIDs {
		if _, ok := cardMap[id]; !ok {
			missingCardIDs = append(missingCardIDs, id)
		}
	}

	if len(missingCardIDs) > 0 && s.dbRepo != nil {
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
		statMap, _ = s.statCache.GetStats(ctx, videoIDs)
	}

	missingStatIDs := make([]int64, 0)
	for _, id := range videoIDs {
		if _, ok := statMap[id]; !ok {
			missingStatIDs = append(missingStatIDs, id)
		}
	}

	if len(missingStatIDs) > 0 && s.dbRepo != nil {
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

	// 3. Query viewer interaction (IsLiked & IsFollowed) if available
	likedMap := make(map[int64]bool)
	if viewerID > 0 && s.interactionRepo != nil {
		if m, err := s.interactionRepo.BatchIsLiked(ctx, viewerID, videoIDs); err == nil {
			likedMap = m
		}
	}

	followedMap := make(map[int64]bool)
	if viewerID > 0 && s.relationRepo != nil && !isFollowed {
		authorIDs := make([]int64, 0, len(videoIDs))
		for _, id := range videoIDs {
			if c, ok := cardMap[id]; ok && c != nil {
				authorIDs = append(authorIDs, c.AuthorID)
			}
		}
		if len(authorIDs) > 0 {
			if m, err := s.relationRepo.BatchIsFollowed(ctx, viewerID, authorIDs); err == nil {
				followedMap = m
			}
		}
	}

	// 4. Assemble final FeedItems in original order
	items := make([]*domainfeed.FeedItem, 0, len(videoIDs))
	for _, id := range videoIDs {
		card, ok := cardMap[id]
		if !ok || card == nil {
			continue
		}

		stat := domainfeed.FeedStat{VideoID: id}
		if st, ok := statMap[id]; ok && st != nil {
			stat = *st
		}

		itemFollowed := isFollowed
		if !itemFollowed && viewerID > 0 {
			itemFollowed = followedMap[card.AuthorID]
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
			IsLiked:     likedMap[id],
			IsFollowed:  itemFollowed,
		})
	}

	return items, nil
}

func (s *service) GetLikeRankFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.LikeRankPage, error) {
	cursorLikeCount, cursorVideoID, err := domainfeed.DecodeLikeRankCursor(cursor)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = domainfeed.DefaultLimit
	}
	if limit > domainfeed.MaxLimit {
		limit = domainfeed.MaxLimit
	}

	if s.dbRepo == nil {
		return nil, errors.New("db repository not configured")
	}

	videos, err := s.dbRepo.ListLikeRankVideos(ctx, cursorLikeCount, cursorVideoID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("failed to query like rank feed: %w", err)
	}

	hasMore := len(videos) > limit
	var nextCursor string
	if hasMore {
		nextItem := videos[limit-1]
		nextCursor = domainfeed.EncodeLikeRankCursor(nextItem.LikeCount, nextItem.VideoID)
		videos = videos[:limit]
	}

	videoIDs := make([]int64, len(videos))
	for i, v := range videos {
		videoIDs[i] = v.VideoID
	}

	items, err := s.hydrateVideoIDs(ctx, videoIDs, viewerID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate like rank items: %w", err)
	}

	return &domainfeed.LikeRankPage{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
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
	return s.AppendTimelineWithSeq(ctx, videoID, videoID)
}

func (s *service) AppendTimelineWithSeq(ctx context.Context, videoID int64, seq int64) error {
	if videoID <= 0 {
		return domainfeed.ErrInvalidVideoID
	}
	if s.timelineRepo != nil {
		if err := s.timelineRepo.AppendWithSeq(ctx, videoID, seq); err != nil {
			return err
		}
	}
	if s.pageCache != nil {
		_ = s.pageCache.DeletePage(ctx, "feed:page:global:public:0:10:v1")
		_ = s.pageCache.DeletePage(ctx, "feed:page:global:public:0:3:v1")
		_ = s.pageCache.DeletePage(ctx, "feed:page:global:public:0:2:v1")
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

func (s *service) ListHotFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.HotFeedPage, error) {
	return s.GetHotFeed(ctx, viewerID, cursor, limit)
}

func (s *service) GetHotFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.HotFeedPage, error) {
	asOf, offset, err := domainfeed.DecodeHotCursor(cursor)
	if err != nil {
		return nil, domainfeed.ErrInvalidHotCursor
	}

	if limit <= 0 {
		limit = domainfeed.DefaultLimit
	}
	if limit > domainfeed.MaxLimit {
		limit = domainfeed.MaxLimit
	}

	// First page initialization: align as_of to current minute, trigger window aggregation
	if asOf == 0 {
		asOfTime := time.Now().Truncate(time.Minute)
		asOf = asOfTime.Unix()
		offset = 0

		if s.hotStore != nil {
			if _, err := s.hotStore.MergeWindow(ctx, asOfTime, 60); err != nil {
				slog.Warn("failed to merge hot window, continuing with best effort", "error", err)
			}
		}
	}

	asOfTime := time.Unix(asOf, 0)
	var videoIDs []int64

	if s.hotStore != nil {
		ids, err := s.hotStore.ListBySnapshot(ctx, asOfTime, offset, limit)
		if err == nil {
			videoIDs = ids
		} else {
			slog.Warn("failed to list hot snapshot, fallback to db", "error", err)
		}
	}

	// Cold start or Redis miss fallback to DB high-like ranking
	if len(videoIDs) == 0 && offset == 0 && s.dbRepo != nil {
		likeVideos, err := s.dbRepo.ListLikeRankVideos(ctx, 0, 0, limit)
		if err == nil {
			for _, v := range likeVideos {
				videoIDs = append(videoIDs, v.VideoID)
			}
		}
	}

	if len(videoIDs) == 0 {
		return &domainfeed.HotFeedPage{
			Items:      []*domainfeed.FeedItem{},
			NextCursor: "",
			HasMore:    false,
		}, nil
	}

	// Determine pagination has_more
	hasMore := len(videoIDs) >= limit
	if s.hotStore != nil {
		total, err := s.hotStore.GetSnapshotTotal(ctx, asOfTime)
		if err == nil && total > 0 {
			hasMore = int64(offset+len(videoIDs)) < total
		}
	}

	var nextCursor string
	if hasMore {
		nextCursor = domainfeed.EncodeHotCursor(asOf, offset+len(videoIDs))
	}

	items, err := s.hydrateVideoIDs(ctx, videoIDs, viewerID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate hot feed items: %w", err)
	}

	return &domainfeed.HotFeedPage{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *service) ListRecommendFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.RecommendFeedPage, error) {
	return s.GetRecommendFeed(ctx, viewerID, cursor, limit)
}

func (s *service) GetRecommendFeed(ctx context.Context, viewerID int64, cursor string, limit int) (*domainfeed.RecommendFeedPage, error) {
	page, seed, err := domainfeed.DecodeRecommendCursor(cursor)
	if err != nil {
		return nil, domainfeed.ErrInvalidRecommendCursor
	}

	if limit <= 0 {
		limit = domainfeed.DefaultLimit
	}
	if limit > domainfeed.MaxLimit {
		limit = domainfeed.MaxLimit
	}

	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	// 1. Multi-source candidate recall
	targetRecall := limit * 3
	if targetRecall < 30 {
		targetRecall = 30
	}

	var candidates []int64
	if s.recallEngine != nil {
		candidates, _ = s.recallEngine.Recall(ctx, targetRecall, seed)
	}

	// Fallback to cold videos from DB if recall engine is not set or returns empty
	if len(candidates) == 0 && s.dbRepo != nil {
		coldCards, _, _, _, err := s.dbRepo.ListColdVideos(ctx, 0, targetRecall)
		if err == nil {
			for _, card := range coldCards {
				candidates = append(candidates, card.VideoID)
			}
		}
	}

	// 2. Exposure filter for logged-in users (viewerID > 0)
	if viewerID > 0 && s.exposureRepo != nil && len(candidates) > 0 {
		filtered, err := s.exposureRepo.FilterExposed(ctx, viewerID, candidates)
		if err == nil {
			candidates = filtered
		}
	}

	// 3. Batch fetch author IDs for sliding window diversification
	authorMap := make(map[int64]int64)
	if len(candidates) > 0 {
		var cardMap map[int64]*domainfeed.FeedCard
		if s.cardCache != nil {
			cardMap, _ = s.cardCache.GetCards(ctx, candidates)
		}
		missingIDs := make([]int64, 0)
		for _, id := range candidates {
			if c, ok := cardMap[id]; ok && c != nil {
				authorMap[id] = c.AuthorID
			} else {
				missingIDs = append(missingIDs, id)
			}
		}
		if len(missingIDs) > 0 && s.dbRepo != nil {
			dbCards, err := s.dbRepo.GetCardsByIDs(ctx, missingIDs)
			if err == nil {
				for id, c := range dbCards {
					authorMap[id] = c.AuthorID
				}
			}
		}
	}

	// 4. Sliding window author diversification
	var diversified []int64
	if s.diversifier != nil && len(candidates) > 0 {
		diversified = s.diversifier.Diversify(candidates, authorMap, limit, 3)
	} else {
		diversified = candidates
		if len(diversified) > limit {
			diversified = diversified[:limit]
		}
	}

	// 5. FillCheck fallback: If remaining items less than limit, supplement from timeline or DB
	if len(diversified) < limit {
		seen := make(map[int64]bool)
		for _, id := range diversified {
			seen[id] = true
		}

		if s.timelineRepo != nil {
			recentIDs, err := s.timelineRepo.ListByCursor(ctx, 0, limit*2)
			if err == nil {
				for _, id := range recentIDs {
					if !seen[id] {
						seen[id] = true
						diversified = append(diversified, id)
						if len(diversified) >= limit {
							break
						}
					}
				}
			}
		}

		if len(diversified) < limit && s.dbRepo != nil {
			coldCards, _, _, _, err := s.dbRepo.ListColdVideos(ctx, 0, limit*2)
			if err == nil {
				for _, card := range coldCards {
					if !seen[card.VideoID] {
						seen[card.VideoID] = true
						diversified = append(diversified, card.VideoID)
						if len(diversified) >= limit {
							break
						}
					}
				}
			}
		}
	}

	if len(diversified) == 0 {
		return &domainfeed.RecommendFeedPage{
			Items:      []*domainfeed.FeedItem{},
			NextCursor: "",
			HasMore:    false,
		}, nil
	}

	// 6. Dynamic Hydration
	items, err := s.hydrateVideoIDs(ctx, diversified, viewerID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to hydrate recommend feed items: %w", err)
	}

	hasMore := len(diversified) >= limit
	var nextCursor string
	if hasMore {
		nextCursor = domainfeed.EncodeRecommendCursor(page+1, seed)
	}

	return &domainfeed.RecommendFeedPage{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

