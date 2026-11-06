package feed_test

import (
	"context"
	"testing"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
)

type stubTagTimeline struct {
	calls    int
	lastName string
	lastCur  int64
	ids      []int64
	next     int64
	hasMore  bool
}

func (s *stubTagTimeline) ListVideoIDsByTag(ctx context.Context, tagName string, cursor int64, limit int) ([]int64, int64, bool, error) {
	s.calls++
	s.lastName = tagName
	s.lastCur = cursor
	return s.ids, s.next, s.hasMore, nil
}

type stubTagLoader struct {
	tags   map[int64][]string
	called bool
}

func (s *stubTagLoader) ListVideoTags(ctx context.Context, videoIDs []int64) (map[int64][]string, error) {
	s.called = true
	return s.tags, nil
}

func newTagFeedService(loader domainfeed.VideoTagLoader, timeline domainfeed.TagTimelineRepository) servicefeed.Service {
	cardCache := &mockCardCache{
		cards: map[int64]*domainfeed.FeedCard{
			201: {VideoID: 201, Title: "Tagged video", AuthorID: 9},
		},
	}
	statCache := &mockStatCache{
		stats: map[int64]*domainfeed.FeedStat{
			201: {VideoID: 201, LikeCount: 3},
		},
	}
	svc := servicefeed.NewService(
		&mockTimelineRepo{},
		&mockPageCache{},
		cardCache,
		statCache,
		&mockExposureRepo{},
		&mockDBRepo{},
		servicefeed.WithTagDependencies(loader, timeline),
	)
	return svc
}

func TestGetTagFeed_NormalizesTagAndAttachesChips(t *testing.T) {
	ctx := context.Background()
	timeline := &stubTagTimeline{ids: []int64{201}, next: 201, hasMore: true}
	loader := &stubTagLoader{tags: map[int64][]string{201: {"旅行", "海边"}}}
	svc := newTagFeedService(loader, timeline)

	page, err := svc.GetTagFeed(ctx, 0, "#旅行", 0, 10)
	if err != nil {
		t.Fatalf("GetTagFeed failed: %v", err)
	}

	// The handler passes whatever the URL carried; the service must normalize before lookup.
	if timeline.lastName != "旅行" {
		t.Fatalf("expected the tag to be normalized to %q, got %q", "旅行", timeline.lastName)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	if got := page.Items[0].Tags; len(got) != 2 || got[0] != "旅行" {
		t.Fatalf("expected tag chips on the item, got %+v", got)
	}
	if !page.HasMore || page.NextCursor != 201 {
		t.Fatalf("expected the cursor to be forwarded, got next=%d hasMore=%v", page.NextCursor, page.HasMore)
	}
}

func TestGetTagFeed_EmptyTagSkipsTheTimeline(t *testing.T) {
	ctx := context.Background()
	timeline := &stubTagTimeline{ids: []int64{201}}
	svc := newTagFeedService(&stubTagLoader{}, timeline)

	page, err := svc.GetTagFeed(ctx, 0, "  #  ", 0, 10)
	if err != nil {
		t.Fatalf("GetTagFeed failed: %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected an empty page for a blank tag, got %d items", len(page.Items))
	}
	if timeline.calls != 0 {
		t.Fatalf("a blank tag must not reach the database, got %d calls", timeline.calls)
	}
}

func TestGetTagFeed_WithoutTimelineReturnsEmptyPage(t *testing.T) {
	ctx := context.Background()
	svc := newTagFeedService(nil, nil)

	page, err := svc.GetTagFeed(ctx, 0, "旅行", 0, 10)
	if err != nil {
		t.Fatalf("a service without tag dependencies must not fail, got %v", err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("expected an empty page, got %d items", len(page.Items))
	}
}
