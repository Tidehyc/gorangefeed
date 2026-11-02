package feed_test

import (
	"context"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
)

type mockHotStoreForService struct {
	snapshotIDs   []int64
	mergedWindows []time.Time
}

func (m *mockHotStoreForService) IncrScore(ctx context.Context, videoID int64, delta float64, occurredAt time.Time) error {
	return nil
}

func (m *mockHotStoreForService) MergeWindow(ctx context.Context, asOf time.Time, windowMinutes int) (string, error) {
	m.mergedWindows = append(m.mergedWindows, asOf)
	return "hot_key", nil
}

func (m *mockHotStoreForService) ListBySnapshot(ctx context.Context, asOf time.Time, offset int, limit int) ([]int64, error) {
	if offset >= len(m.snapshotIDs) {
		return []int64{}, nil
	}
	end := offset + limit
	if end > len(m.snapshotIDs) {
		end = len(m.snapshotIDs)
	}
	return m.snapshotIDs[offset:end], nil
}

func (m *mockHotStoreForService) GetSnapshotTotal(ctx context.Context, asOf time.Time) (int64, error) {
	return int64(len(m.snapshotIDs)), nil
}

func TestService_GetHotFeed_PaginationAndFallback(t *testing.T) {
	ctx := context.Background()

	// 1. Prepare Hot Store with 15 videos
	hotStore := &mockHotStoreForService{
		snapshotIDs: []int64{101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115},
	}

	cardMap := make(map[int64]*domainfeed.FeedCard)
	statMap := make(map[int64]*domainfeed.FeedStat)
	for _, id := range hotStore.snapshotIDs {
		cardMap[id] = &domainfeed.FeedCard{
			VideoID:  id,
			AuthorID: id*10 + 1,
			Title:    "Hot Video",
		}
		statMap[id] = &domainfeed.FeedStat{
			VideoID:   id,
			LikeCount: 100,
		}
	}
	cardMap[999] = &domainfeed.FeedCard{
		VideoID:  999,
		AuthorID: 9990,
		Title:    "Top Liked Fallback Video",
	}
	statMap[999] = &domainfeed.FeedStat{
		VideoID:   999,
		LikeCount: 500,
	}

	cardCache := &mockCardCache{cards: cardMap}

	statCache := &mockStatCache{stats: statMap}
	exposureRepo := &mockExposureRepo{}
	timelineRepo := &mockTimelineRepo{}
	pageCache := &mockPageCache{}
	dbRepo := &mockDBRepo{
		cardsMap: cardMap,
		statsMap: statMap,
		likeVideos: []*domainfeed.LikeRankVideo{
			{VideoID: 999, LikeCount: 500},
		},
	}

	svc := servicefeed.NewService(
		timelineRepo,
		pageCache,
		cardCache,
		statCache,
		exposureRepo,
		dbRepo,
		servicefeed.WithHotTimelineStore(hotStore),
	)

	// Case 1: First page request (cursor = "")
	page1, err := svc.GetHotFeed(ctx, 10001, "", 10)
	if err != nil {
		t.Fatalf("unexpected error on page 1: %v", err)
	}
	if len(page1.Items) != 10 {
		t.Fatalf("expected 10 items on page 1, got %d", len(page1.Items))
	}
	if !page1.HasMore {
		t.Fatalf("expected has_more=true on page 1")
	}
	if page1.NextCursor == "" {
		t.Fatalf("expected next_cursor on page 1")
	}

	asOf1, offset1, err := domainfeed.DecodeHotCursor(page1.NextCursor)
	if err != nil || asOf1 <= 0 || offset1 != 10 {
		t.Fatalf("invalid next_cursor format: asOf=%d, offset=%d, err=%v", asOf1, offset1, err)
	}

	// Case 2: Second page request with stable as_of snapshot cursor
	page2, err := svc.GetHotFeed(ctx, 10001, page1.NextCursor, 10)
	if err != nil {
		t.Fatalf("unexpected error on page 2: %v", err)
	}
	if len(page2.Items) != 5 {
		t.Fatalf("expected remaining 5 items on page 2, got %d", len(page2.Items))
	}
	if page2.HasMore {
		t.Fatalf("expected has_more=false on page 2")
	}
	if page2.NextCursor != "" {
		t.Fatalf("expected empty next_cursor when has_more=false, got %s", page2.NextCursor)
	}

	// Case 3: Empty hot store fallback to DB ListLikeRankVideos
	emptyHotStore := &mockHotStoreForService{snapshotIDs: []int64{}}
	svcFallback := servicefeed.NewService(
		timelineRepo,
		pageCache,
		cardCache,
		statCache,
		exposureRepo,
		dbRepo,
		servicefeed.WithHotTimelineStore(emptyHotStore),
	)

	fallbackPage, err := svcFallback.GetHotFeed(ctx, 0, "", 10)
	if err != nil {
		t.Fatalf("unexpected error on fallback: %v", err)
	}
	if len(fallbackPage.Items) != 1 || fallbackPage.Items[0].ID != 999 {
		t.Fatalf("expected fallback to DB like rank video 999, got: %+v", fallbackPage.Items)
	}

	// Case 4: Invalid cursor
	_, err = svc.GetHotFeed(ctx, 0, "bad-cursor-base64!!", 10)
	if err != domainfeed.ErrInvalidHotCursor {
		t.Fatalf("expected ErrInvalidHotCursor, got: %v", err)
	}
}

type mockRecallEngineForService struct {
	recalledIDs []int64
}

func (m *mockRecallEngineForService) Recall(ctx context.Context, targetCount int, seed int64) ([]int64, error) {
	return m.recalledIDs, nil
}

func TestService_GetRecommendFeed_ExposureAndDiversification(t *testing.T) {
	ctx := context.Background()

	// Candidates:
	// Video 1 (Author 100)
	// Video 2 (Author 100)
	// Video 3 (Author 100)
	// Video 4 (Author 200)
	// Video 5 (Author 300)
	// Video 6 (Author 400)
	// Video 7 (Author 500)
	candidateIDs := []int64{1, 2, 3, 4, 5, 6, 7}

	cardMap := map[int64]*domainfeed.FeedCard{
		1: {VideoID: 1, AuthorID: 100, Title: "V1"},
		2: {VideoID: 2, AuthorID: 100, Title: "V2"},
		3: {VideoID: 3, AuthorID: 100, Title: "V3"},
		4: {VideoID: 4, AuthorID: 200, Title: "V4"},
		5: {VideoID: 5, AuthorID: 300, Title: "V5"},
		6: {VideoID: 6, AuthorID: 400, Title: "V6"},
		7: {VideoID: 7, AuthorID: 500, Title: "V7"},
	}
	statMap := map[int64]*domainfeed.FeedStat{
		1: {VideoID: 1}, 2: {VideoID: 2}, 3: {VideoID: 3},
		4: {VideoID: 4}, 5: {VideoID: 5}, 6: {VideoID: 6}, 7: {VideoID: 7},
	}

	cardCache := &mockCardCache{cards: cardMap}
	statCache := &mockStatCache{stats: statMap}
	exposureRepo := &mockExposureRepo{
		exposures: map[int64][]int64{
			8888: {1}, // User 8888 has already seen Video 1
		},
	}
	timelineRepo := &mockTimelineRepo{}
	pageCache := &mockPageCache{}
	dbRepo := &mockDBRepo{
		cardsMap: cardMap,
		statsMap: statMap,
	}

	recallEngine := &mockRecallEngineForService{
		recalledIDs: candidateIDs,
	}
	diversifier := servicefeed.NewAuthorDiversifier()

	svc := servicefeed.NewService(
		timelineRepo,
		pageCache,
		cardCache,
		statCache,
		exposureRepo,
		dbRepo,
		servicefeed.WithRecommendDependencies(recallEngine, diversifier),
	)

	// Case 1: Anonymous visitor (viewerID = 0)
	// Should see V1 because exposure filtering is skipped for guests
	// Diversification should separate Author 100: V1 (Author 100), V4 (Author 200), V5 (Author 300), V6 (Author 400)
	guestPage, err := svc.GetRecommendFeed(ctx, 0, "", 4)
	if err != nil {
		t.Fatalf("unexpected error for guest recommend: %v", err)
	}
	if len(guestPage.Items) != 4 {
		t.Fatalf("expected 4 items for guest, got %d", len(guestPage.Items))
	}
	expectedGuestOrder := []int64{1, 4, 5, 6}
	for i, expectedID := range expectedGuestOrder {
		if guestPage.Items[i].ID != expectedID {
			t.Errorf("guest item[%d] = %d, want %d", i, guestPage.Items[i].ID, expectedID)
		}
	}
	if guestPage.Items[0].IsLiked || guestPage.Items[0].IsFollowed {
		t.Fatalf("guest items must have is_liked=false and is_followed=false")
	}

	// Case 2: Logged-in user 8888 who has viewed V1
	// V1 must be filtered out by ExposureRepository
	// Remaining candidates: 2 (Author 100), 3 (Author 100), 4 (Author 200), 5 (Author 300), 6 (Author 400), 7 (Author 500)
	// Diversification selects 2 (Author 100), 4 (Author 200), 5 (Author 300), 6 (Author 400)
	userPage, err := svc.GetRecommendFeed(ctx, 8888, "", 4)
	if err != nil {
		t.Fatalf("unexpected error for logged-in user: %v", err)
	}
	if len(userPage.Items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(userPage.Items))
	}
	for _, item := range userPage.Items {
		if item.ID == 1 {
			t.Fatalf("video 1 should have been filtered by exposure repository")
		}
	}
	expectedUserOrder := []int64{2, 4, 5, 6}
	for i, expectedID := range expectedUserOrder {
		if userPage.Items[i].ID != expectedID {
			t.Errorf("user item[%d] = %d, want %d", i, userPage.Items[i].ID, expectedID)
		}
	}

	// Case 3: Invalid cursor
	_, err = svc.GetRecommendFeed(ctx, 0, "not-a-valid-recommend-cursor***", 10)
	if err != domainfeed.ErrInvalidRecommendCursor {
		t.Fatalf("expected ErrInvalidRecommendCursor, got %v", err)
	}
}
