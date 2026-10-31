package feed

import (
	"context"
	"errors"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
)

type mockFeedDBForFollowing struct {
	cardsMap map[int64]*domainfeed.FeedCard
	statsMap map[int64]*domainfeed.FeedStat
}

func (m *mockFeedDBForFollowing) ListColdVideos(ctx context.Context, cursor int64, limit int) ([]*domainfeed.FeedCard, []*domainfeed.FeedStat, int64, bool, error) {
	return nil, nil, 0, false, nil
}
func (m *mockFeedDBForFollowing) GetCardsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	res := make(map[int64]*domainfeed.FeedCard)
	for _, id := range videoIDs {
		if c, ok := m.cardsMap[id]; ok {
			res[id] = c
		}
	}
	return res, nil
}
func (m *mockFeedDBForFollowing) GetStatsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	res := make(map[int64]*domainfeed.FeedStat)
	for _, id := range videoIDs {
		if s, ok := m.statsMap[id]; ok {
			res[id] = s
		}
	}
	return res, nil
}
func (m *mockFeedDBForFollowing) GetVideosStatusByIDs(ctx context.Context, videoIDs []int64) (map[int64]int8, error) {
	return nil, nil
}
func (m *mockFeedDBForFollowing) CalculateFloor(ctx context.Context, duration time.Duration) (int64, error) {
	return 0, nil
}
func (m *mockFeedDBForFollowing) IncrViewCount(ctx context.Context, videoID int64) error {
	return nil
}

func TestService_ListFollowingFeed_Validation(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)

	ctx := context.Background()

	// 1. ViewerID <= 0 should fail with ErrInvalidViewerID
	_, err := svc.ListFollowingFeed(ctx, 0, 0, 10)
	if !errors.Is(err, domainfeed.ErrInvalidViewerID) {
		t.Errorf("expected ErrInvalidViewerID for viewerID=0, got: %v", err)
	}

	// 2. Cursor < 0 should fail with ErrInvalidCursor
	_, err = svc.ListFollowingFeed(ctx, 101, -1, 10)
	if !errors.Is(err, domainfeed.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor for cursor=-1, got: %v", err)
	}
}

func TestService_ListFollowingFeed_MultiWayMerge(t *testing.T) {
	viewerID := int64(101)
	normalAuthorID := int64(201)
	bigVAuthorID := int64(301)

	relationRepo := &mockRelationRepoForFanout{
		followingsMap: map[int64][]int64{
			viewerID: {normalAuthorID, bigVAuthorID},
		},
		statsMap: map[int64]*domainrelation.RelationStats{
			normalAuthorID: {UserID: normalAuthorID, FollowerCount: 100},
			bigVAuthorID:   {UserID: bigVAuthorID, FollowerCount: 10000},
		},
	}

	followingRepo := &mockFollowingRepoForFanout{
		inboxExists: map[int64]bool{
			viewerID: true,
		},
		inboxZSets: map[int64][]int64{
			viewerID: {50, 30, 10}, // Videos from normal author in inbox
		},
		authorZSets: map[int64][]int64{
			bigVAuthorID: {60, 40, 20}, // Videos from big V author
		},
	}

	dbRepo := &mockFeedDBForFollowing{
		cardsMap: map[int64]*domainfeed.FeedCard{
			60: {VideoID: 60, Title: "V60", AuthorID: bigVAuthorID},
			50: {VideoID: 50, Title: "V50", AuthorID: normalAuthorID},
			40: {VideoID: 40, Title: "V40", AuthorID: bigVAuthorID},
			30: {VideoID: 30, Title: "V30", AuthorID: normalAuthorID},
			20: {VideoID: 20, Title: "V20", AuthorID: bigVAuthorID},
			10: {VideoID: 10, Title: "V10", AuthorID: normalAuthorID},
		},
		statsMap: map[int64]*domainfeed.FeedStat{
			60: {VideoID: 60, LikeCount: 6},
			50: {VideoID: 50, LikeCount: 5},
			40: {VideoID: 40, LikeCount: 4},
			30: {VideoID: 30, LikeCount: 3},
			20: {VideoID: 20, LikeCount: 2},
			10: {VideoID: 10, LikeCount: 1},
		},
	}

	svc := NewService(
		nil, nil, nil, nil, nil, dbRepo,
		WithFollowingDependencies(followingRepo, relationRepo, nil),
	)

	ctx := context.Background()

	// First page: limit 4
	page, err := svc.ListFollowingFeed(ctx, viewerID, 0, 4)
	if err != nil {
		t.Fatalf("ListFollowingFeed failed: %v", err)
	}

	// Multi-way merge should produce: [60, 50, 40, 30]
	if len(page.Items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(page.Items))
	}
	expectedIDs := []int64{60, 50, 40, 30}
	for i, item := range page.Items {
		if item.ID != expectedIDs[i] {
			t.Errorf("item %d: expected id %d, got %d", i, expectedIDs[i], item.ID)
		}
		if !item.IsFollowed {
			t.Errorf("expected item %d to have IsFollowed=true", item.ID)
		}
	}
	if !page.HasMore {
		t.Errorf("expected hasMore=true")
	}
	if page.NextCursor != 30 {
		t.Errorf("expected nextCursor=30, got %d", page.NextCursor)
	}

	// Second page: cursor = 30, limit 4
	page2, err := svc.ListFollowingFeed(ctx, viewerID, 30, 4)
	if err != nil {
		t.Fatalf("ListFollowingFeed page 2 failed: %v", err)
	}
	if len(page2.Items) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(page2.Items))
	}
	if page2.Items[0].ID != 20 || page2.Items[1].ID != 10 {
		t.Errorf("unexpected page 2 items: %v, %v", page2.Items[0].ID, page2.Items[1].ID)
	}
	if page2.HasMore {
		t.Errorf("expected page 2 hasMore=false")
	}
}

func TestService_ListFollowingFeed_ColdStartFallback(t *testing.T) {
	viewerID := int64(102)
	normalAuthorID := int64(202)

	relationRepo := &mockRelationRepoForFanout{
		followingsMap: map[int64][]int64{
			viewerID: {normalAuthorID},
		},
		statsMap: map[int64]*domainrelation.RelationStats{
			normalAuthorID: {UserID: normalAuthorID, FollowerCount: 50},
		},
	}

	followingRepo := &mockFollowingRepoForFanout{
		inboxExists: map[int64]bool{
			viewerID: false, // Cold start: inbox does NOT exist in Redis!
		},
		dbVideos: []int64{99, 98}, // MySQL returns published videos
	}

	dbRepo := &mockFeedDBForFollowing{
		cardsMap: map[int64]*domainfeed.FeedCard{
			99: {VideoID: 99, Title: "V99", AuthorID: normalAuthorID},
			98: {VideoID: 98, Title: "V98", AuthorID: normalAuthorID},
		},
		statsMap: map[int64]*domainfeed.FeedStat{
			99: {VideoID: 99},
			98: {VideoID: 98},
		},
	}

	svc := NewService(
		nil, nil, nil, nil, nil, dbRepo,
		WithFollowingDependencies(followingRepo, relationRepo, nil),
	)

	ctx := context.Background()
	page, err := svc.ListFollowingFeed(ctx, viewerID, 0, 10)
	if err != nil {
		t.Fatalf("ListFollowingFeed cold start failed: %v", err)
	}

	if len(page.Items) != 2 {
		t.Fatalf("expected 2 fallback items, got %d", len(page.Items))
	}
	if page.Items[0].ID != 99 || page.Items[1].ID != 98 {
		t.Errorf("unexpected fallback items: %v, %v", page.Items[0].ID, page.Items[1].ID)
	}

	// Give a moment for asynchronous inbox prewarm goroutine
	time.Sleep(50 * time.Millisecond)

	followingRepo.mu.Lock()
	prewarmCount := len(followingRepo.prewarmCalls)
	followingRepo.mu.Unlock()

	if prewarmCount == 0 {
		t.Errorf("expected inbox prewarm to be triggered on cold start")
	}
}
