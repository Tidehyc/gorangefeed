package feed_test

import (
	"context"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
)

type mockInteractionRepoForFeed struct {
	likedMap map[int64]bool
}

func (m *mockInteractionRepoForFeed) LikeVideo(ctx context.Context, userID, videoID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	return true, nil
}
func (m *mockInteractionRepoForFeed) UnlikeVideo(ctx context.Context, userID, videoID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	return true, nil
}
func (m *mockInteractionRepoForFeed) IsLiked(ctx context.Context, userID, videoID int64) (bool, error) {
	return m.likedMap[videoID], nil
}
func (m *mockInteractionRepoForFeed) BatchIsLiked(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error) {
	res := make(map[int64]bool)
	for _, vid := range videoIDs {
		res[vid] = m.likedMap[vid]
	}
	return res, nil
}
func (m *mockInteractionRepoForFeed) ListMyLikedVideoIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error) {
	return nil, 0, false, nil
}
func (m *mockInteractionRepoForFeed) CreateComment(ctx context.Context, comment *domaininteraction.Comment, outboxEvent *domainoutbox.Event) (*domaininteraction.Comment, bool, error) {
	return nil, false, nil
}
func (m *mockInteractionRepoForFeed) FindCommentByIdempotencyKey(ctx context.Context, userID int64, key string) (*domaininteraction.Comment, error) {
	return nil, nil
}
func (m *mockInteractionRepoForFeed) FindCommentByID(ctx context.Context, commentID int64) (*domaininteraction.Comment, error) {
	return nil, nil
}
func (m *mockInteractionRepoForFeed) DeleteComment(ctx context.Context, commentID, userID int64, outboxEvent *domainoutbox.Event) error {
	return nil
}
func (m *mockInteractionRepoForFeed) ListCommentsByTime(ctx context.Context, videoID int64, cursor int64, limit int) ([]*domaininteraction.Comment, int64, bool, error) {
	return nil, 0, false, nil
}
func (m *mockInteractionRepoForFeed) ListCommentsByHot(ctx context.Context, videoID int64, cursorLikeCount int64, cursorID int64, limit int) ([]*domaininteraction.Comment, int64, int64, bool, error) {
	return nil, 0, 0, false, nil
}
func (m *mockInteractionRepoForFeed) LikeComment(ctx context.Context, userID, commentID int64) (bool, error) {
	return true, nil
}
func (m *mockInteractionRepoForFeed) UnlikeComment(ctx context.Context, userID, commentID int64) (bool, error) {
	return true, nil
}
func (m *mockInteractionRepoForFeed) BatchIsCommentLiked(ctx context.Context, userID int64, commentIDs []int64) (map[int64]bool, error) {
	return nil, nil
}

type mockRelationRepoForFeed struct {
	followedAuthors map[int64]bool
}

func (m *mockRelationRepoForFeed) Follow(ctx context.Context, followerID, vloggerID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	return true, nil
}
func (m *mockRelationRepoForFeed) Unfollow(ctx context.Context, followerID, vloggerID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	return true, nil
}
func (m *mockRelationRepoForFeed) GetRelation(ctx context.Context, userID, targetID int64) (*domainrelation.Friendship, error) {
	return nil, nil
}
func (m *mockRelationRepoForFeed) BatchIsFollowed(ctx context.Context, followerID int64, authorIDs []int64) (map[int64]bool, error) {
	res := make(map[int64]bool)
	for _, aid := range authorIDs {
		res[aid] = m.followedAuthors[aid]
	}
	return res, nil
}
func (m *mockRelationRepoForFeed) ListFollowingIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error) {
	return nil, 0, false, nil
}
func (m *mockRelationRepoForFeed) ListFollowerIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error) {
	return nil, 0, false, nil
}
func (m *mockRelationRepoForFeed) GetStats(ctx context.Context, userID int64) (*domainrelation.RelationStats, error) {
	return &domainrelation.RelationStats{UserID: userID}, nil
}

type mockLikeRankDB struct {
	likeRankVideos []*domainfeed.LikeRankVideo
	cards          map[int64]*domainfeed.FeedCard
	stats          map[int64]*domainfeed.FeedStat
}

func (m *mockLikeRankDB) ListColdVideos(ctx context.Context, cursor int64, limit int) ([]*domainfeed.FeedCard, []*domainfeed.FeedStat, int64, bool, error) {
	return nil, nil, 0, false, nil
}
func (m *mockLikeRankDB) GetCardsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	res := make(map[int64]*domainfeed.FeedCard)
	for _, id := range videoIDs {
		if c, ok := m.cards[id]; ok {
			res[id] = c
		}
	}
	return res, nil
}
func (m *mockLikeRankDB) GetStatsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	res := make(map[int64]*domainfeed.FeedStat)
	for _, id := range videoIDs {
		if s, ok := m.stats[id]; ok {
			res[id] = s
		}
	}
	return res, nil
}
func (m *mockLikeRankDB) GetVideosStatusByIDs(ctx context.Context, videoIDs []int64) (map[int64]int8, error) {
	return nil, nil
}
func (m *mockLikeRankDB) CalculateFloor(ctx context.Context, duration time.Duration) (int64, error) {
	return 0, nil
}
func (m *mockLikeRankDB) IncrViewCount(ctx context.Context, videoID int64) error {
	return nil
}
func (m *mockLikeRankDB) ListLikeRankVideos(ctx context.Context, cursorLikeCount, cursorVideoID int64, limit int) ([]*domainfeed.LikeRankVideo, error) {
	var res []*domainfeed.LikeRankVideo
	for _, v := range m.likeRankVideos {
		if cursorLikeCount > 0 || cursorVideoID > 0 {
			if v.LikeCount > cursorLikeCount {
				continue
			}
			if v.LikeCount == cursorLikeCount && v.VideoID >= cursorVideoID {
				continue
			}
		}
		res = append(res, v)
		if len(res) >= limit {
			break
		}
	}
	return res, nil
}

func TestFeedService_GetLikeRankFeed(t *testing.T) {
	db := &mockLikeRankDB{
		likeRankVideos: []*domainfeed.LikeRankVideo{
			{VideoID: 101, LikeCount: 500},
			{VideoID: 103, LikeCount: 300},
			{VideoID: 102, LikeCount: 300},
			{VideoID: 104, LikeCount: 100},
		},
		cards: map[int64]*domainfeed.FeedCard{
			101: {VideoID: 101, Title: "Top Video 1", AuthorID: 201},
			102: {VideoID: 102, Title: "Second Video", AuthorID: 202},
			103: {VideoID: 103, Title: "Third Video", AuthorID: 203},
			104: {VideoID: 104, Title: "Fourth Video", AuthorID: 204},
		},
		stats: map[int64]*domainfeed.FeedStat{
			101: {VideoID: 101, LikeCount: 500},
			102: {VideoID: 102, LikeCount: 300},
			103: {VideoID: 103, LikeCount: 300},
			104: {VideoID: 104, LikeCount: 100},
		},
	}

	interactionRepo := &mockInteractionRepoForFeed{
		likedMap: map[int64]bool{101: true},
	}
	relationRepo := &mockRelationRepoForFeed{
		followedAuthors: map[int64]bool{201: true},
	}

	svc := servicefeed.NewService(
		nil, nil, nil, nil, nil, db,
		servicefeed.WithFollowingDependencies(nil, relationRepo, interactionRepo),
	)

	ctx := context.Background()

	// 1. Initial page (limit = 2)
	page, err := svc.GetLikeRankFeed(ctx, 999, "", 2)
	if err != nil {
		t.Fatalf("GetLikeRankFeed page 1 failed: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page.Items))
	}
	if page.Items[0].ID != 101 || page.Items[1].ID != 103 {
		t.Errorf("unexpected items on page 1: %v, %v", page.Items[0].ID, page.Items[1].ID)
	}
	if !page.HasMore {
		t.Errorf("expected hasMore=true")
	}
	if page.NextCursor != "300_103" {
		t.Errorf("expected nextCursor=300_103, got %s", page.NextCursor)
	}

	// 2. Second page using nextCursor
	page2, err := svc.GetLikeRankFeed(ctx, 999, page.NextCursor, 2)
	if err != nil {
		t.Fatalf("GetLikeRankFeed page 2 failed: %v", err)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(page2.Items))
	}
	if page2.Items[0].ID != 102 || page2.Items[1].ID != 104 {
		t.Errorf("unexpected items on page 2: %v, %v", page2.Items[0].ID, page2.Items[1].ID)
	}
	if page2.HasMore {
		t.Errorf("expected page 2 hasMore=false")
	}
	_ = interactionRepo
}

func TestFeedService_GetGlobalFeed_PublicSkeleton_DynamicHydration(t *testing.T) {
	pageCache := &mockPageCacheForSkeleton{
		pages: make(map[string]*domainfeed.CachedPage),
	}
	cardCache := &mockCardCacheForSkeleton{
		cards: map[int64]*domainfeed.FeedCard{
			10: {VideoID: 10, Title: "Video 10", AuthorID: 501},
			20: {VideoID: 20, Title: "Video 20", AuthorID: 502},
		},
	}
	statCache := &mockStatCacheForSkeleton{
		stats: map[int64]*domainfeed.FeedStat{
			10: {VideoID: 10, LikeCount: 100},
			20: {VideoID: 20, LikeCount: 200},
		},
	}
	dbRepo := &mockDBRepoForSkeleton{
		cards: cardCache.cards,
		stats: statCache.stats,
	}

	interactionRepo := &mockInteractionRepoForFeed{
		likedMap: map[int64]bool{10: true}, // user likes video 10
	}
	relationRepo := &mockRelationRepoForFeed{
		followedAuthors: map[int64]bool{502: true}, // user follows author of video 20
	}

	timelineRepo := &mockTimelineForSkeleton{
		ids: []int64{20, 10},
	}

	svc := servicefeed.NewService(
		timelineRepo, pageCache, cardCache, statCache, nil, dbRepo,
		servicefeed.WithFollowingDependencies(nil, relationRepo, interactionRepo),
	)

	ctx := context.Background()

	// 1. Anonymous visitor loads global feed -> cache miss -> populates public L1 skeleton
	anonPage, err := svc.GetGlobalFeed(ctx, 0, 0, 10)
	if err != nil {
		t.Fatalf("anonymous GetGlobalFeed failed: %v", err)
	}
	if len(anonPage.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(anonPage.Items))
	}
	if anonPage.Items[0].IsLiked || anonPage.Items[0].IsFollowed {
		t.Errorf("expected anon item to have is_liked=false, is_followed=false")
	}

	// Verify that the L1 page cache was keyed with "public", NOT "anon" or "uid"
	publicL1Key := "feed:page:global:public:0:10:v1"
	if _, ok := pageCache.pages[publicL1Key]; !ok {
		t.Fatalf("expected L1 page cache key %s to exist, cached keys: %v", publicL1Key, pageCache.pages)
	}

	// 2. Logged-in user loads global feed -> cache HIT on the EXACT same public L1 key!
	loggedInPage, err := svc.GetGlobalFeed(ctx, 999, 0, 10)
	if err != nil {
		t.Fatalf("logged in GetGlobalFeed failed: %v", err)
	}
	if len(loggedInPage.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(loggedInPage.Items))
	}

	// Dynamic hydration should apply to the returned items!
	// Video 20: author 502 is followed -> IsFollowed = true
	if !loggedInPage.Items[0].IsFollowed {
		t.Errorf("expected video 20 to have IsFollowed=true for logged in user")
	}
}

type mockPageCacheForSkeleton struct {
	pages map[string]*domainfeed.CachedPage
}

func (m *mockPageCacheForSkeleton) GetPage(ctx context.Context, key string) (*domainfeed.CachedPage, bool, error) {
	p, ok := m.pages[key]
	return p, ok, nil
}
func (m *mockPageCacheForSkeleton) SetPage(ctx context.Context, key string, page *domainfeed.CachedPage, ttl time.Duration) error {
	m.pages[key] = page
	return nil
}
func (m *mockPageCacheForSkeleton) DeletePage(ctx context.Context, key string) error {
	delete(m.pages, key)
	return nil
}

type mockCardCacheForSkeleton struct {
	cards map[int64]*domainfeed.FeedCard
}

func (m *mockCardCacheForSkeleton) GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	res := make(map[int64]*domainfeed.FeedCard)
	for _, id := range videoIDs {
		if c, ok := m.cards[id]; ok {
			res[id] = c
		}
	}
	return res, nil
}
func (m *mockCardCacheForSkeleton) SetCards(ctx context.Context, cards []*domainfeed.FeedCard, ttl time.Duration) error {
	return nil
}

type mockStatCacheForSkeleton struct {
	stats map[int64]*domainfeed.FeedStat
}

func (m *mockStatCacheForSkeleton) GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	res := make(map[int64]*domainfeed.FeedStat)
	for _, id := range videoIDs {
		if s, ok := m.stats[id]; ok {
			res[id] = s
		}
	}
	return res, nil
}
func (m *mockStatCacheForSkeleton) SetStats(ctx context.Context, stats []*domainfeed.FeedStat, ttl time.Duration) error {
	return nil
}
func (m *mockStatCacheForSkeleton) InvalidateStat(ctx context.Context, videoID int64) error {
	return nil
}

type mockTimelineForSkeleton struct {
	ids []int64
}

func (m *mockTimelineForSkeleton) GetFloor(ctx context.Context) (int64, error) {
	return 1, nil
}
func (m *mockTimelineForSkeleton) SetFloor(ctx context.Context, floor int64) error {
	return nil
}
func (m *mockTimelineForSkeleton) Append(ctx context.Context, videoID int64) error {
	return nil
}
func (m *mockTimelineForSkeleton) AppendWithSeq(ctx context.Context, videoID int64, seq int64) error {
	return nil
}
func (m *mockTimelineForSkeleton) Trim(ctx context.Context, floor int64) error {
	return nil
}
func (m *mockTimelineForSkeleton) ListByCursor(ctx context.Context, cursor int64, limit int) ([]int64, error) {
	var res []int64
	for _, id := range m.ids {
		if cursor > 0 && id >= cursor {
			continue
		}
		res = append(res, id)
		if len(res) >= limit {
			break
		}
	}
	return res, nil
}
func (m *mockTimelineForSkeleton) Rebuild(ctx context.Context, videoIDs []int64, floor int64) error {
	return nil
}

type mockDBRepoForSkeleton struct {
	cards map[int64]*domainfeed.FeedCard
	stats map[int64]*domainfeed.FeedStat
}

func (m *mockDBRepoForSkeleton) ListColdVideos(ctx context.Context, cursor int64, limit int) ([]*domainfeed.FeedCard, []*domainfeed.FeedStat, int64, bool, error) {
	return nil, nil, 0, false, nil
}
func (m *mockDBRepoForSkeleton) GetCardsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	res := make(map[int64]*domainfeed.FeedCard)
	for _, id := range videoIDs {
		if c, ok := m.cards[id]; ok {
			res[id] = c
		}
	}
	return res, nil
}
func (m *mockDBRepoForSkeleton) GetStatsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	res := make(map[int64]*domainfeed.FeedStat)
	for _, id := range videoIDs {
		if s, ok := m.stats[id]; ok {
			res[id] = s
		}
	}
	return res, nil
}
func (m *mockDBRepoForSkeleton) GetVideosStatusByIDs(ctx context.Context, videoIDs []int64) (map[int64]int8, error) {
	res := make(map[int64]int8)
	for _, id := range videoIDs {
		res[id] = 1
	}
	return res, nil
}
func (m *mockDBRepoForSkeleton) CalculateFloor(ctx context.Context, duration time.Duration) (int64, error) {
	return 1, nil
}
func (m *mockDBRepoForSkeleton) IncrViewCount(ctx context.Context, videoID int64) error {
	return nil
}
func (m *mockDBRepoForSkeleton) ListLikeRankVideos(ctx context.Context, cursorLikeCount, cursorVideoID int64, limit int) ([]*domainfeed.LikeRankVideo, error) {
	return nil, nil
}
