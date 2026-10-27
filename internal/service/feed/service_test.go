package feed_test

import (
	"context"
	"errors"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
)

// MockTimelineRepo implements domainfeed.TimelineRepository
type mockTimelineRepo struct {
	floor    int64
	videoIDs []int64
	appendFn func(id int64) error
	trimFn   func(floor int64) error
	listErr  error
}

func (m *mockTimelineRepo) GetFloor(ctx context.Context) (int64, error) {
	return m.floor, nil
}
func (m *mockTimelineRepo) SetFloor(ctx context.Context, floor int64) error {
	m.floor = floor
	return nil
}
func (m *mockTimelineRepo) Append(ctx context.Context, videoID int64) error {
	if m.appendFn != nil {
		return m.appendFn(videoID)
	}
	m.videoIDs = append(m.videoIDs, videoID)
	return nil
}
func (m *mockTimelineRepo) Trim(ctx context.Context, floor int64) error {
	if m.trimFn != nil {
		return m.trimFn(floor)
	}
	m.floor = floor
	return nil
}
func (m *mockTimelineRepo) ListByCursor(ctx context.Context, cursor int64, limit int) ([]int64, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	res := make([]int64, 0)
	for _, id := range m.videoIDs {
		if cursor <= 0 || id < cursor {
			res = append(res, id)
			if len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}
func (m *mockTimelineRepo) Rebuild(ctx context.Context, videoIDs []int64, floor int64) error {
	m.videoIDs = videoIDs
	m.floor = floor
	return nil
}

// MockPageCache implements domainfeed.PageCacheRepository
type mockPageCache struct {
	pages map[string]*domainfeed.CachedPage
}

func (m *mockPageCache) GetPage(ctx context.Context, key string) (*domainfeed.CachedPage, bool, error) {
	p, ok := m.pages[key]
	return p, ok, nil
}
func (m *mockPageCache) SetPage(ctx context.Context, key string, page *domainfeed.CachedPage, ttl time.Duration) error {
	if m.pages == nil {
		m.pages = make(map[string]*domainfeed.CachedPage)
	}
	m.pages[key] = page
	return nil
}
func (m *mockPageCache) DeletePage(ctx context.Context, key string) error {
	delete(m.pages, key)
	return nil
}

// MockCardCache implements domainfeed.CardCacheRepository
type mockCardCache struct {
	cards map[int64]*domainfeed.FeedCard
}

func (m *mockCardCache) GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	res := make(map[int64]*domainfeed.FeedCard)
	for _, id := range videoIDs {
		if c, ok := m.cards[id]; ok {
			res[id] = c
		}
	}
	return res, nil
}
func (m *mockCardCache) SetCards(ctx context.Context, cards []*domainfeed.FeedCard, ttl time.Duration) error {
	if m.cards == nil {
		m.cards = make(map[int64]*domainfeed.FeedCard)
	}
	for _, c := range cards {
		m.cards[c.VideoID] = c
	}
	return nil
}

// MockStatCache implements domainfeed.StatCacheRepository
type mockStatCache struct {
	stats           map[int64]*domainfeed.FeedStat
	invalidatedStat []int64
}

func (m *mockStatCache) GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	res := make(map[int64]*domainfeed.FeedStat)
	for _, id := range videoIDs {
		if s, ok := m.stats[id]; ok {
			res[id] = s
		}
	}
	return res, nil
}
func (m *mockStatCache) SetStats(ctx context.Context, stats []*domainfeed.FeedStat, ttl time.Duration) error {
	if m.stats == nil {
		m.stats = make(map[int64]*domainfeed.FeedStat)
	}
	for _, s := range stats {
		m.stats[s.VideoID] = s
	}
	return nil
}
func (m *mockStatCache) InvalidateStat(ctx context.Context, videoID int64) error {
	m.invalidatedStat = append(m.invalidatedStat, videoID)
	delete(m.stats, videoID)
	return nil
}

// MockExposureRepo implements domainfeed.ExposureRepository
type mockExposureRepo struct {
	exposures map[int64][]int64
}

func (m *mockExposureRepo) RecordExposures(ctx context.Context, viewerID int64, videoIDs []int64) error {
	if m.exposures == nil {
		m.exposures = make(map[int64][]int64)
	}
	m.exposures[viewerID] = append(m.exposures[viewerID], videoIDs...)
	return nil
}
func (m *mockExposureRepo) IsExposed(ctx context.Context, viewerID int64, videoID int64) (bool, error) {
	for _, id := range m.exposures[viewerID] {
		if id == videoID {
			return true, nil
		}
	}
	return false, nil
}

// MockDBRepo implements domainfeed.FeedDBRepository
type mockDBRepo struct {
	coldCards  []*domainfeed.FeedCard
	coldStats  []*domainfeed.FeedStat
	cardsMap   map[int64]*domainfeed.FeedCard
	statsMap   map[int64]*domainfeed.FeedStat
	statusMap  map[int64]int8
	floor      int64
	viewCounts map[int64]int64
}

func (m *mockDBRepo) ListColdVideos(ctx context.Context, cursor int64, limit int) ([]*domainfeed.FeedCard, []*domainfeed.FeedStat, int64, bool, error) {
	resCards := make([]*domainfeed.FeedCard, 0)
	resStats := make([]*domainfeed.FeedStat, 0)
	for i, c := range m.coldCards {
		if cursor <= 0 || c.VideoID < cursor {
			resCards = append(resCards, c)
			resStats = append(resStats, m.coldStats[i])
			if len(resCards) >= limit {
				break
			}
		}
	}
	hasMore := len(resCards) >= limit
	nextCursor := int64(0)
	if len(resCards) > 0 {
		nextCursor = resCards[len(resCards)-1].VideoID
	}
	return resCards, resStats, nextCursor, hasMore, nil
}
func (m *mockDBRepo) GetCardsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	res := make(map[int64]*domainfeed.FeedCard)
	for _, id := range videoIDs {
		if c, ok := m.cardsMap[id]; ok {
			res[id] = c
		}
	}
	return res, nil
}
func (m *mockDBRepo) GetStatsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	res := make(map[int64]*domainfeed.FeedStat)
	for _, id := range videoIDs {
		if s, ok := m.statsMap[id]; ok {
			res[id] = s
		}
	}
	return res, nil
}
func (m *mockDBRepo) GetVideosStatusByIDs(ctx context.Context, videoIDs []int64) (map[int64]int8, error) {
	res := make(map[int64]int8)
	for _, id := range videoIDs {
		if st, ok := m.statusMap[id]; ok {
			res[id] = st
		} else {
			res[id] = 1 // default published
		}
	}
	return res, nil
}
func (m *mockDBRepo) CalculateFloor(ctx context.Context, duration time.Duration) (int64, error) {
	return m.floor, nil
}
func (m *mockDBRepo) IncrViewCount(ctx context.Context, videoID int64) error {
	if m.viewCounts == nil {
		m.viewCounts = make(map[int64]int64)
	}
	m.viewCounts[videoID]++
	return nil
}

func TestFeedService_GetGlobalFeed_L1HitAndStale(t *testing.T) {
	ctx := context.Background()

	pageCache := &mockPageCache{
		pages: map[string]*domainfeed.CachedPage{
			"feed:page:global:anon:0:10:v1": {
				VideoIDs:   []int64{101, 102},
				NextCursor: 102,
				HasMore:    true,
				ExpireAt:   time.Now().Add(10 * time.Second), // fresh
			},
		},
	}
	cardCache := &mockCardCache{
		cards: map[int64]*domainfeed.FeedCard{
			101: {VideoID: 101, Title: "Card 101"},
			102: {VideoID: 102, Title: "Card 102"},
		},
	}
	statCache := &mockStatCache{
		stats: map[int64]*domainfeed.FeedStat{
			101: {VideoID: 101, ViewCount: 15},
			102: {VideoID: 102, ViewCount: 20},
		},
	}

	svc := servicefeed.NewService(
		&mockTimelineRepo{},
		pageCache,
		cardCache,
		statCache,
		&mockExposureRepo{},
		&mockDBRepo{},
	)

	// 1. Fresh L1 Hit
	page, err := svc.GetGlobalFeed(ctx, 0, 0, 10)
	if err != nil {
		t.Fatalf("expected no error on L1 hit, got %v", err)
	}
	if len(page.Items) != 2 || page.Items[0].Title != "Card 101" || page.Items[1].Stats.ViewCount != 20 {
		t.Fatalf("unexpected items on L1 hit: %+v", page.Items)
	}

	// 2. Stale L1 Hit
	pageCache.pages["feed:page:global:anon:0:10:v1"].ExpireAt = time.Now().Add(-5 * time.Second) // soft expired, within 30s
	pageStale, err := svc.GetGlobalFeed(ctx, 0, 0, 10)
	if err != nil {
		t.Fatalf("expected no error on stale L1 hit, got %v", err)
	}
	if len(pageStale.Items) != 2 {
		t.Fatalf("expected stale page to be served immediately")
	}
}

func TestFeedService_GetGlobalFeed_DeletedVideoSkipAndCursorAdvance(t *testing.T) {
	ctx := context.Background()

	// Timeline has IDs in descending order: 205, 204 (hidden), 203 (published), 202 (hidden), 201 (published)
	timeline := &mockTimelineRepo{
		videoIDs: []int64{205, 204, 203, 202, 201},
		floor:    100,
	}
	db := &mockDBRepo{
		statusMap: map[int64]int8{
			205: 1, // published
			204: 2, // hidden!
			203: 1, // published
			202: 2, // hidden!
			201: 1, // published
		},
		cardsMap: map[int64]*domainfeed.FeedCard{
			205: {VideoID: 205, Title: "Video 205"},
			203: {VideoID: 203, Title: "Video 203"},
			201: {VideoID: 201, Title: "Video 201"},
		},
		statsMap: map[int64]*domainfeed.FeedStat{
			205: {VideoID: 205},
			203: {VideoID: 203},
			201: {VideoID: 201},
		},
	}

	svc := servicefeed.NewService(
		timeline,
		&mockPageCache{},
		&mockCardCache{},
		&mockStatCache{},
		&mockExposureRepo{},
		db,
	)

	// Ask for limit = 2.
	// Scans 205 (published -> valid #1), 204 (hidden -> skipped!), 203 (published -> valid #2).
	// Loop terminates as we reached limit 2.
	// Crucial check: nextCursor must be advanced to 203 (the last scanned item)!
	page, err := svc.GetGlobalFeed(ctx, 0, 0, 2)
	if err != nil {
		t.Fatalf("failed to get feed: %v", err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected 2 published items, got %d", len(page.Items))
	}
	if page.Items[0].ID != 205 || page.Items[1].ID != 203 {
		t.Fatalf("expected items [205, 203], got [%d, %d]", page.Items[0].ID, page.Items[1].ID)
	}
	if page.NextCursor != 203 {
		t.Fatalf("expected NextCursor advanced to 203, got %d", page.NextCursor)
	}
}

func TestFeedService_GetGlobalFeed_ColdHotSplicing(t *testing.T) {
	ctx := context.Background()

	// Hot zone only has 1 video: 300
	timeline := &mockTimelineRepo{
		videoIDs: []int64{300},
		floor:    300,
	}
	// Cold zone has videos: 250, 240, 230
	db := &mockDBRepo{
		floor: 300,
		statusMap: map[int64]int8{
			300: 1,
		},
		cardsMap: map[int64]*domainfeed.FeedCard{
			300: {VideoID: 300, Title: "Hot Video 300"},
			250: {VideoID: 250, Title: "Cold Video 250"},
			240: {VideoID: 240, Title: "Cold Video 240"},
		},
		coldCards: []*domainfeed.FeedCard{
			{VideoID: 250, Title: "Cold Video 250"},
			{VideoID: 240, Title: "Cold Video 240"},
		},
		coldStats: []*domainfeed.FeedStat{
			{VideoID: 250},
			{VideoID: 240},
		},
	}

	svc := servicefeed.NewService(
		timeline,
		&mockPageCache{},
		&mockCardCache{},
		&mockStatCache{},
		&mockExposureRepo{},
		db,
	)

	// Ask for limit = 3. Hot provides 1, Cold should splice 2 items!
	page, err := svc.GetGlobalFeed(ctx, 0, 0, 3)
	if err != nil {
		t.Fatalf("failed to get spliced feed: %v", err)
	}

	if len(page.Items) != 3 {
		t.Fatalf("expected 3 spliced items, got %d", len(page.Items))
	}
	if page.Items[0].ID != 300 || page.Items[1].ID != 250 || page.Items[2].ID != 240 {
		t.Fatalf("expected items [300, 250, 240], got [%d, %d, %d]", page.Items[0].ID, page.Items[1].ID, page.Items[2].ID)
	}
	if page.NextCursor != 240 {
		t.Fatalf("expected NextCursor 240, got %d", page.NextCursor)
	}
}

func TestFeedService_RecordView(t *testing.T) {
	ctx := context.Background()

	db := &mockDBRepo{}
	statCache := &mockStatCache{
		stats: map[int64]*domainfeed.FeedStat{
			501: {VideoID: 501, ViewCount: 10},
		},
	}
	expoRepo := &mockExposureRepo{}

	svc := servicefeed.NewService(
		&mockTimelineRepo{},
		&mockPageCache{},
		&mockCardCache{},
		statCache,
		expoRepo,
		db,
	)

	event := domainfeed.ViewEvent{
		VideoID:         501,
		ViewerID:        999,
		EventType:       domainfeed.ViewEventPlay,
		WatchDurationMs: 1500,
		Timestamp:       time.Now(),
	}

	if err := svc.RecordView(ctx, 999, 501, event); err != nil {
		t.Fatalf("RecordView failed: %v", err)
	}

	// 1. Verify view count incremented in DB
	if db.viewCounts[501] != 1 {
		t.Fatalf("expected db view count 1, got %d", db.viewCounts[501])
	}

	// 2. Verify L4 stat invalidated
	if len(statCache.invalidatedStat) != 1 || statCache.invalidatedStat[0] != 501 {
		t.Fatalf("expected L4 stat 501 invalidated, got %v", statCache.invalidatedStat)
	}

	// 3. Verify exposure recorded
	exposed, _ := expoRepo.IsExposed(ctx, 999, 501)
	if !exposed {
		t.Fatalf("expected video 501 recorded in exposure history")
	}

	// 4. Invalid videoID error check
	err := svc.RecordView(ctx, 999, 0, event)
	if !errors.Is(err, domainfeed.ErrInvalidVideoID) {
		t.Fatalf("expected ErrInvalidVideoID, got %v", err)
	}
}
