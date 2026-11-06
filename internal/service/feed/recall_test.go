package feed_test

import (
	"context"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
)

type mockHotStoreForRecall struct {
	snapshotIDs []int64
}

func (m *mockHotStoreForRecall) IncrScore(ctx context.Context, videoID int64, delta float64, occurredAt time.Time) error {
	return nil
}

// ApplyEngagementOnce is part of the store contract but belongs to the write path; these tests
// only exercise recall.
func (m *mockHotStoreForRecall) ApplyEngagementOnce(ctx context.Context, eventID string, videoID int64, delta float64, occurredAt time.Time) (bool, error) {
	return true, nil
}

func (m *mockHotStoreForRecall) MergeWindow(ctx context.Context, asOf time.Time, windowMinutes int) (string, error) {
	return "test_snapshot_key", nil
}

func (m *mockHotStoreForRecall) ListBySnapshot(ctx context.Context, asOf time.Time, offset int, limit int) ([]int64, error) {
	if offset >= len(m.snapshotIDs) {
		return []int64{}, nil
	}
	end := offset + limit
	if end > len(m.snapshotIDs) {
		end = len(m.snapshotIDs)
	}
	return m.snapshotIDs[offset:end], nil
}

func (m *mockHotStoreForRecall) GetSnapshotTotal(ctx context.Context, asOf time.Time) (int64, error) {
	return int64(len(m.snapshotIDs)), nil
}

type mockTimelineRepoForRecall struct {
	timelineIDs []int64
}

func (m *mockTimelineRepoForRecall) Append(ctx context.Context, videoID int64) error {
	return nil
}

func (m *mockTimelineRepoForRecall) AppendWithSeq(ctx context.Context, videoID int64, seq int64) error {
	return nil
}

func (m *mockTimelineRepoForRecall) Remove(ctx context.Context, videoID int64) error {
	return nil
}

func (m *mockTimelineRepoForRecall) GetFloor(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *mockTimelineRepoForRecall) SetFloor(ctx context.Context, floor int64) error {
	return nil
}


func (m *mockTimelineRepoForRecall) ListByCursor(ctx context.Context, cursor int64, limit int) ([]int64, error) {
	var res []int64
	for _, id := range m.timelineIDs {
		if cursor == 0 || id < cursor {
			res = append(res, id)
			if len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

func (m *mockTimelineRepoForRecall) Trim(ctx context.Context, floor int64) error {
	return nil
}

func (m *mockTimelineRepoForRecall) Rebuild(ctx context.Context, videoIDs []int64, floor int64) error {
	return nil
}



type mockDBRepoForRecall struct {
	coldCards  []*domainfeed.FeedCard
	likeVideos []*domainfeed.LikeRankVideo
}

func (m *mockDBRepoForRecall) CalculateFloor(ctx context.Context, duration time.Duration) (int64, error) {
	return 0, nil
}



func (m *mockDBRepoForRecall) ListColdVideos(ctx context.Context, cursor int64, limit int) ([]*domainfeed.FeedCard, []*domainfeed.FeedStat, int64, bool, error) {
	var resCards []*domainfeed.FeedCard
	var resStats []*domainfeed.FeedStat
	for _, card := range m.coldCards {
		if cursor == 0 || card.VideoID < cursor {
			resCards = append(resCards, card)
			resStats = append(resStats, &domainfeed.FeedStat{VideoID: card.VideoID})
			if len(resCards) >= limit {
				break
			}
		}
	}
	hasMore := len(resCards) == limit
	var nextCursor int64 = 0
	if len(resCards) > 0 {
		nextCursor = resCards[len(resCards)-1].VideoID
	}
	return resCards, resStats, nextCursor, hasMore, nil
}

func (m *mockDBRepoForRecall) GetCardsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	res := make(map[int64]*domainfeed.FeedCard)
	for _, id := range videoIDs {
		for _, card := range m.coldCards {
			if card.VideoID == id {
				res[id] = card
			}
		}
	}
	return res, nil
}

func (m *mockDBRepoForRecall) GetStatsByIDs(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	res := make(map[int64]*domainfeed.FeedStat)
	for _, id := range videoIDs {
		res[id] = &domainfeed.FeedStat{VideoID: id}
	}
	return res, nil
}

func (m *mockDBRepoForRecall) GetVideosStatusByIDs(ctx context.Context, videoIDs []int64) (map[int64]int8, error) {
	res := make(map[int64]int8)
	for _, id := range videoIDs {
		res[id] = 1
	}
	return res, nil
}

func (m *mockDBRepoForRecall) IncrViewCount(ctx context.Context, videoID int64) error {
	return nil
}

func (m *mockDBRepoForRecall) ListLikeRankVideos(ctx context.Context, cursorLikeCount, cursorVideoID int64, limit int) ([]*domainfeed.LikeRankVideo, error) {
	if limit > len(m.likeVideos) {
		limit = len(m.likeVideos)
	}
	return m.likeVideos[:limit], nil
}

func TestMultiSourceRecallEngine_Recall(t *testing.T) {
	ctx := context.Background()

	hotStore := &mockHotStoreForRecall{
		snapshotIDs: []int64{101, 102, 103, 104, 105},
	}
	timelineRepo := &mockTimelineRepoForRecall{
		timelineIDs: []int64{201, 202, 203, 204},
	}
	coldCards := make([]*domainfeed.FeedCard, 0)
	for id := int64(301); id <= 320; id++ {
		coldCards = append(coldCards, &domainfeed.FeedCard{VideoID: id, Title: "Explore video"})
	}
	dbRepo := &mockDBRepoForRecall{
		coldCards: coldCards,
		likeVideos: []*domainfeed.LikeRankVideo{
			{VideoID: 901, LikeCount: 100},
			{VideoID: 902, LikeCount: 90},
		},
	}

	engine := servicefeed.NewMultiSourceRecallEngine(hotStore, timelineRepo, dbRepo)

	// Test 1: Full multi-source recall
	candidates, err := engine.Recall(ctx, 0, 10, 123456)
	if err != nil {
		t.Fatalf("unexpected recall error: %v", err)
	}
	if len(candidates) < 10 {
		t.Fatalf("expected at least 10 candidates, got %d: %v", len(candidates), candidates)
	}

	// Verify deduplication
	seen := make(map[int64]bool)
	for _, id := range candidates {
		if seen[id] {
			t.Fatalf("duplicate candidate found: %d", id)
		}
		seen[id] = true
	}

	// Test 2: Hot store fallback to DB like rank
	emptyHotStore := &mockHotStoreForRecall{snapshotIDs: []int64{}}
	engineWithEmptyHot := servicefeed.NewMultiSourceRecallEngine(emptyHotStore, timelineRepo, dbRepo)
	fallbackCandidates, err := engineWithEmptyHot.Recall(ctx, 0, 10, 123456)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fallbackCandidates) < 10 {
		t.Fatalf("expected at least 10 candidates after fallback, got %d", len(fallbackCandidates))
	}
	// Verify that 901 or 902 was recalled as hot fallback
	foundLikeRank := false
	for _, id := range fallbackCandidates {
		if id == 901 || id == 902 {
			foundLikeRank = true
			break
		}
	}
	if !foundLikeRank {
		t.Fatalf("expected like rank fallback candidate in result: %v", fallbackCandidates)
	}

	// Test 3: Insufficient candidates with smooth compensation
	sparseDB := &mockDBRepoForRecall{
		coldCards: []*domainfeed.FeedCard{
			{VideoID: 401},
			{VideoID: 402},
		},
	}
	sparseEngine := servicefeed.NewMultiSourceRecallEngine(nil, nil, sparseDB)
	sparseCandidates, err := sparseEngine.Recall(ctx, 0, 5, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sparseCandidates) != 2 {
		t.Fatalf("expected 2 candidates when total available is 2, got %d", len(sparseCandidates))
	}
}
