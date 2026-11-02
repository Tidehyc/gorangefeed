package feed_test

import (
	"context"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	persistencefeed "github.com/Tidehyc/gorangefeed/internal/infra/persistence/feed"
)

func TestHotStore_IncrScoreAndSnapshotPagination(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	if rdb == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	store := persistencefeed.NewHotStore(rdb)

	baseTime := time.Date(2026, 11, 2, 10, 30, 0, 0, time.UTC)
	min0 := baseTime
	min1 := baseTime.Add(-1 * time.Minute)
	min2 := baseTime.Add(-2 * time.Minute)

	// Clean up keys after test
	defer func() {
		_ = rdb.Del(ctx,
			persistencefeed.MinuteBucketKey(min0),
			persistencefeed.MinuteBucketKey(min1),
			persistencefeed.MinuteBucketKey(min2),
			persistencefeed.WindowSnapshotKey(baseTime),
		).Err()
	}()

	// 1. Video 101: In min0 gets +3 (like), in min1 gets +5 (comment) -> Total = 8.0
	if err := store.IncrScore(ctx, 101, domainfeed.ScoreLikeDelta, min0); err != nil {
		t.Fatalf("failed to incr score for video 101 min0: %v", err)
	}
	if err := store.IncrScore(ctx, 101, domainfeed.ScoreCommentDelta, min1); err != nil {
		t.Fatalf("failed to incr score for video 101 min1: %v", err)
	}

	// 2. Video 102: In min0 gets +3 (like), in min1 gets +2 (complete), in min2 gets +5 (comment) -> Total = 10.0
	if err := store.IncrScore(ctx, 102, domainfeed.ScoreLikeDelta, min0); err != nil {
		t.Fatalf("failed to incr score for video 102 min0: %v", err)
	}
	if err := store.IncrScore(ctx, 102, domainfeed.ScoreCompleteDelta, min1); err != nil {
		t.Fatalf("failed to incr score for video 102 min1: %v", err)
	}
	if err := store.IncrScore(ctx, 102, domainfeed.ScoreCommentDelta, min2); err != nil {
		t.Fatalf("failed to incr score for video 102 min2: %v", err)
	}

	// 3. Video 103: In min0 gets +3 (like), then gets -3 (unlike) -> Total = 0.0 (Should be purged by ZRemRangeByScore)
	if err := store.IncrScore(ctx, 103, domainfeed.ScoreLikeDelta, min0); err != nil {
		t.Fatalf("failed to incr score for video 103: %v", err)
	}
	if err := store.IncrScore(ctx, 103, domainfeed.ScoreUnlikeDelta, min0); err != nil {
		t.Fatalf("failed to incr score for video 103: %v", err)
	}

	// 4. Video 104: In min1 gets -3.0 (negative net score) -> Total < 0 (Should be purged)
	if err := store.IncrScore(ctx, 104, domainfeed.ScoreUnlikeDelta, min1); err != nil {
		t.Fatalf("failed to incr score for video 104: %v", err)
	}

	// 5. Video 105: In min2 gets +0.5 (play) -> Total = 0.5
	if err := store.IncrScore(ctx, 105, domainfeed.ScorePlayDelta, min2); err != nil {
		t.Fatalf("failed to incr score for video 105: %v", err)
	}

	// 6. Test MergeWindow
	snapshotKey, err := store.MergeWindow(ctx, baseTime, 60)
	if err != nil {
		t.Fatalf("failed to merge window: %v", err)
	}
	expectedSnapshotKey := persistencefeed.WindowSnapshotKey(baseTime)
	if snapshotKey != expectedSnapshotKey {
		t.Fatalf("snapshot key mismatch: got %s, want %s", snapshotKey, expectedSnapshotKey)
	}

	// 7. Verify valid count (Only 102 with 10.0, 101 with 8.0, 105 with 0.5; 103 and 104 are purged)
	total, err := store.GetSnapshotTotal(ctx, baseTime)
	if err != nil {
		t.Fatalf("failed to get snapshot total: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3 valid positive items in hot window, got %d", total)
	}

	// 8. List first page: offset=0, limit=2 -> should return [102, 101]
	page1, err := store.ListBySnapshot(ctx, baseTime, 0, 2)
	if err != nil {
		t.Fatalf("failed to list page 1: %v", err)
	}
	if len(page1) != 2 || page1[0] != 102 || page1[1] != 101 {
		t.Fatalf("expected page1 [102, 101], got %v", page1)
	}

	// 9. List second page: offset=2, limit=2 -> should return [105]
	page2, err := store.ListBySnapshot(ctx, baseTime, 2, 2)
	if err != nil {
		t.Fatalf("failed to list page 2: %v", err)
	}
	if len(page2) != 1 || page2[0] != 105 {
		t.Fatalf("expected page2 [105], got %v", page2)
	}

	// 10. List beyond total: offset=3, limit=2 -> should return empty slice
	page3, err := store.ListBySnapshot(ctx, baseTime, 3, 2)
	if err != nil {
		t.Fatalf("failed to list page 3: %v", err)
	}
	if len(page3) != 0 {
		t.Fatalf("expected page3 to be empty, got %v", page3)
	}
}

func TestHotStore_InvalidInputs(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	if rdb == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	store := persistencefeed.NewHotStore(rdb)

	// Invalid video ID
	err := store.IncrScore(ctx, 0, 3.0, time.Now())
	if err != domainfeed.ErrInvalidVideoID {
		t.Fatalf("expected ErrInvalidVideoID, got %v", err)
	}

	// Delta == 0 should be no-op (return nil)
	err = store.IncrScore(ctx, 100, 0, time.Now())
	if err != nil {
		t.Fatalf("expected nil for 0 delta, got %v", err)
	}
}
