package redis_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	infraredis "github.com/Tidehyc/gorangefeed/internal/infra/redis"
)

func TestSequenceGenerator_MonotonicAndConcurrency(t *testing.T) {
	var configPath string
	for _, candidate := range []string{
		"../../../configs/config.yaml",
		"../../configs/config.yaml",
		"configs/config.yaml",
	} {
		if _, err := os.Stat(candidate); err == nil {
			configPath = candidate
			break
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Skipf("skipping redis sequence test: config load failed (%v)", err)
		return
	}

	rdb, err := infraredis.New(&cfg.Redis)
	if err != nil {
		t.Skipf("skipping redis sequence test: redis connection failed (%v)", err)
		return
	}
	defer rdb.Close()

	ctx := context.Background()
	testKey := fmt.Sprintf("test:seq:video:%d", time.Now().UnixNano())
	defer rdb.Del(ctx, testKey)

	gen := infraredis.NewCustomSequenceGenerator(rdb, testKey)

	// 1. Initial seq should be 0
	initial, err := gen.GetCurrentSeq(ctx)
	if err != nil || initial != 0 {
		t.Fatalf("expected initial seq 0, got %d, err: %v", initial, err)
	}

	// 2. Sequential increments
	s1, err := gen.NextPublishSeq(ctx)
	if err != nil || s1 != 1 {
		t.Fatalf("expected seq 1, got %d, err: %v", s1, err)
	}
	s2, err := gen.NextPublishSeq(ctx)
	if err != nil || s2 != 2 {
		t.Fatalf("expected seq 2, got %d, err: %v", s2, err)
	}

	current, err := gen.GetCurrentSeq(ctx)
	if err != nil || current != 2 {
		t.Fatalf("expected current seq 2, got %d, err: %v", current, err)
	}

	// 3. Concurrency test with 50 goroutines
	const numGoroutines = 50
	results := make([]int64, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			seq, seqErr := gen.NextPublishSeq(ctx)
			if seqErr != nil {
				t.Errorf("goroutine %d error: %v", idx, seqErr)
				return
			}
			results[idx] = seq
		}(i)
	}
	wg.Wait()

	// Verify all 50 values are distinct and fall between 3 and 52
	seen := make(map[int64]bool)
	for _, val := range results {
		if val < 3 || val > 2+numGoroutines {
			t.Fatalf("seq %d out of expected range [3, %d]", val, 2+numGoroutines)
		}
		if seen[val] {
			t.Fatalf("duplicate sequence number detected: %d", val)
		}
		seen[val] = true
	}

	if len(seen) != numGoroutines {
		t.Fatalf("expected %d unique sequences, got %d", numGoroutines, len(seen))
	}
}
