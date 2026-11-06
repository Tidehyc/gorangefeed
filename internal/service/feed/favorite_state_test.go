package feed_test

import (
	"context"
	"errors"
	"testing"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
)

type stubFavoriteChecker struct {
	calls     int
	lastUID   int64
	lastIDs   []int64
	favorited map[int64]bool
	err       error
}

func (s *stubFavoriteChecker) BatchIsFavorited(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error) {
	s.calls++
	s.lastUID = userID
	s.lastIDs = videoIDs
	if s.err != nil {
		return nil, s.err
	}
	return s.favorited, nil
}

// newFavoriteStateService builds a one-video tag feed so the test exercises hydrateVideoIDs
// without dragging the timeline caches into the assertion.
func newFavoriteStateService(checker domainfeed.FavoriteChecker) servicefeed.Service {
	cardCache := &mockCardCache{
		cards: map[int64]*domainfeed.FeedCard{
			301: {VideoID: 301, Title: "Collectable", AuthorID: 12},
		},
	}
	statCache := &mockStatCache{
		stats: map[int64]*domainfeed.FeedStat{
			301: {VideoID: 301, LikeCount: 4, FavoriteCount: 7},
		},
	}
	return servicefeed.NewService(
		&mockTimelineRepo{},
		&mockPageCache{},
		cardCache,
		statCache,
		&mockExposureRepo{},
		&mockDBRepo{},
		servicefeed.WithTagDependencies(&stubTagLoader{}, &stubTagTimeline{ids: []int64{301}}),
		servicefeed.WithFavoriteChecker(checker),
	)
}

func TestFeedItem_CarriesFavoriteStateAndCount(t *testing.T) {
	ctx := context.Background()
	checker := &stubFavoriteChecker{favorited: map[int64]bool{301: true}}
	svc := newFavoriteStateService(checker)

	page, err := svc.GetTagFeed(ctx, 7, "旅行", 0, 10)
	if err != nil {
		t.Fatalf("GetTagFeed failed: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}

	item := page.Items[0]
	if !item.IsFavorited {
		t.Fatal("expected is_favorited=true for a viewer who collected the video")
	}
	if item.Stats.FavoriteCount != 7 {
		t.Fatalf("expected favorite_count=7 to survive hydration, got %d", item.Stats.FavoriteCount)
	}
	if checker.lastUID != 7 {
		t.Fatalf("expected the checker to be called with the viewer id, got %d", checker.lastUID)
	}
	if len(checker.lastIDs) != 1 || checker.lastIDs[0] != 301 {
		t.Fatalf("expected a batch lookup for [301], got %v", checker.lastIDs)
	}
}

func TestFeedItem_GuestsSkipTheFavoriteLookup(t *testing.T) {
	ctx := context.Background()
	checker := &stubFavoriteChecker{favorited: map[int64]bool{301: true}}
	svc := newFavoriteStateService(checker)

	page, err := svc.GetTagFeed(ctx, 0, "旅行", 0, 10)
	if err != nil {
		t.Fatalf("GetTagFeed failed: %v", err)
	}
	if page.Items[0].IsFavorited {
		t.Fatal("a guest must not be reported as having collected anything")
	}
	// A guest has no user id, so asking the database would be a wasted query per page.
	if checker.calls != 0 {
		t.Fatalf("expected no favorite lookup for a guest, got %d calls", checker.calls)
	}
}

func TestFeedItem_FavoriteLookupFailureKeepsTheFeed(t *testing.T) {
	ctx := context.Background()
	checker := &stubFavoriteChecker{err: errors.New("redis down")}
	svc := newFavoriteStateService(checker)

	page, err := svc.GetTagFeed(ctx, 7, "旅行", 0, 10)
	if err != nil {
		t.Fatalf("a favorite lookup failure must not fail the feed, got %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected the page to survive the failure, got %d items", len(page.Items))
	}
	if page.Items[0].IsFavorited {
		t.Fatal("a failed lookup degrades to not favorited, not to a guess")
	}
}
