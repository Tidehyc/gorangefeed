package benchmark_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// BenchmarkFeed_GlobalLatest_L1Hit_Parallel measures throughput of global feed
// when L1 page cache is hit under massive parallel access (simulating 10,000 video scale).
func BenchmarkFeed_GlobalLatest_L1Hit_Parallel(b *testing.B) {
	bCtx, cleanup := setupBenchmark(b)
	if cleanup != nil {
		defer cleanup()
	}

	ctx := context.Background()
	_, _ = prewarmFeedData(ctx, bCtx, 50)

	// Pre-warm L1 page cache
	_, err := bCtx.FeedSvc.GetGlobalFeed(ctx, 0, 0, 10)
	if err != nil {
		b.Fatalf("failed to prewarm L1 cache: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			page, err := bCtx.FeedSvc.GetGlobalFeed(ctx, 0, 0, 10)
			if err != nil || len(page.Items) == 0 {
				b.Fatalf("global feed fetch failed: %v", err)
			}
		}
	})
}

// BenchmarkFeed_GlobalLatest_Hydrated_Parallel measures throughput of global feed
// when logged-in viewer dynamically hydrates interactive states (is_liked, is_followed).
func BenchmarkFeed_GlobalLatest_Hydrated_Parallel(b *testing.B) {
	bCtx, cleanup := setupBenchmark(b)
	if cleanup != nil {
		defer cleanup()
	}

	ctx := context.Background()
	videoIDs, _ := prewarmFeedData(ctx, bCtx, 50)

	// Simulate viewer who liked some videos
	viewerID := int64(999001)
	for i := 0; i < 5 && i < len(videoIDs); i++ {
		_ = bCtx.InteractionSvc.LikeVideo(ctx, viewerID, videoIDs[i])
	}

	// Warm L1 public skeleton
	_, _ = bCtx.FeedSvc.GetGlobalFeed(ctx, 0, 0, 10)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			page, err := bCtx.FeedSvc.GetGlobalFeed(ctx, viewerID, 0, 10)
			if err != nil || len(page.Items) == 0 {
				b.Fatalf("hydrated global feed fetch failed: %v", err)
			}
		}
	})
}

// BenchmarkFeed_HotWindow_Aggregation_Parallel measures sliding 60-minute window
// ZUNIONSTORE aggregation and snapshot offset pagination.
func BenchmarkFeed_HotWindow_Aggregation_Parallel(b *testing.B) {
	bCtx, cleanup := setupBenchmark(b)
	if cleanup != nil {
		defer cleanup()
	}

	ctx := context.Background()
	videoIDs, _ := prewarmFeedData(ctx, bCtx, 50)

	// Seed hot score minute buckets for the past 60 minutes
	now := time.Now().UTC()
	for m := 0; m < 30; m++ {
		t := now.Add(-time.Duration(m) * time.Minute)
		for _, vid := range videoIDs {
			score := float64(rand.Intn(10) + 1)
			_ = bCtx.HotStore.IncrScore(ctx, vid, score, t)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			page, err := bCtx.FeedSvc.ListHotFeed(ctx, 0, "", 10)
			if err != nil || len(page.Items) == 0 {
				b.Fatalf("hot feed fetch failed: %v", err)
			}
		}
	})
}

// BenchmarkFeed_Recommend_MultiSourceRecall_Parallel measures multi-source recall
// (hot + latest + random) and author window diversification pipeline.
func BenchmarkFeed_Recommend_MultiSourceRecall_Parallel(b *testing.B) {
	bCtx, cleanup := setupBenchmark(b)
	if cleanup != nil {
		defer cleanup()
	}

	ctx := context.Background()
	videoIDs, _ := prewarmFeedData(ctx, bCtx, 50)

	viewerID := int64(888001)
	// Seed some exposure records in Redis Set
	for i := 0; i < 5 && i < len(videoIDs); i++ {
		_ = bCtx.RDB.SAdd(ctx, fmt.Sprintf("reco:exposed:%d", viewerID), videoIDs[i]).Err()
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			page, err := bCtx.FeedSvc.ListRecommendFeed(ctx, viewerID, "", 10)
			if err != nil || len(page.Items) == 0 {
				b.Fatalf("recommend feed fetch failed: %v", err)
			}
		}
	})
}
