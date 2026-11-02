package benchmark_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	persistenceoutbox "github.com/Tidehyc/gorangefeed/internal/infra/persistence/outbox"
	"github.com/google/uuid"
)

// BenchmarkInteraction_ConcurrentLikes_SameVideo_Parallel benchmarks high-concurrency
// write contention on a single hot video, measuring video_stats row lock latency and Outbox transaction commit throughput.
func BenchmarkInteraction_ConcurrentLikes_SameVideo_Parallel(b *testing.B) {
	bCtx, cleanup := setupBenchmark(b)
	if cleanup != nil {
		defer cleanup()
	}

	ctx := context.Background()
	videoIDs, _ := prewarmFeedData(ctx, bCtx, 1)
	if len(videoIDs) == 0 {
		b.Fatalf("failed to seed video")
	}
	hotVideoID := videoIDs[0]

	var uidGen int64 = 100000

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			userID := atomic.AddInt64(&uidGen, 1)
			// Alternating like and unlike to keep database state bounded while exercising full transaction
			err := bCtx.InteractionSvc.LikeVideo(ctx, userID, hotVideoID)
			if err != nil {
				b.Fatalf("concurrent like failed: %v", err)
			}
		}
	})
}

// BenchmarkOutbox_ClaimLease_And_Dispatch_Parallel benchmarks Outbox CAS lease acquisition
// under competing worker threads to evaluate scheduling contention and throughput.
func BenchmarkOutbox_ClaimLease_And_Dispatch_Parallel(b *testing.B) {
	bCtx, cleanup := setupBenchmark(b)
	if cleanup != nil {
		defer cleanup()
	}

	ctx := context.Background()

	// Seed 2,000 pending outbox events
	now := time.Now().UTC()
	bCtx.DB.Exec("DELETE FROM outbox WHERE status = 'pending'")
	defer func() {
		bCtx.DB.Exec("DELETE FROM outbox WHERE event_type = 'benchmark.event'")
	}()
	for i := 0; i < 2000; i++ {
		eventID := uuid.New().String()
		model := &persistenceoutbox.OutboxModel{
			EventID:     eventID,
			EventType:   "benchmark.event",
			Payload:     []byte(`{"video_id":123,"user_id":456}`),
			Status:      string(domainoutbox.StatusPending),
			NextRetryAt: now.Add(-10 * time.Second),
			CreatedAt:   now,
		}
		_ = bCtx.DB.Create(model).Error
	}

	var workerIDGen int64 = 1

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		workerToken := fmt.Sprintf("bench-worker-%d", atomic.AddInt64(&workerIDGen, 1))
		for pb.Next() {
			claimed, err := bCtx.OutboxRepo.ClaimLease(ctx, workerToken, 10*time.Second, 20)
			if err != nil {
				b.Fatalf("claim lease failed: %v", err)
			}
			// Simulate immediate acknowledge if events claimed
			for _, e := range claimed {
				_ = bCtx.OutboxRepo.MarkSent(ctx, e.EventID)
			}
		}
	})
}
