package feed_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	persistencefeed "github.com/Tidehyc/gorangefeed/internal/infra/persistence/feed"
	"github.com/Tidehyc/gorangefeed/internal/infra/redis"
	goredis "github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) (*goredis.Client, func()) {
	t.Helper()

	var configPath string
	for _, candidate := range []string{
		"../../../../configs/config.yaml",
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
		t.Skipf("skipping redis test: config load failed (%v)", err)
		return nil, nil
	}

	rdb, err := redis.New(&cfg.Redis)
	if err != nil {
		t.Skipf("skipping redis test: Redis connection failed (%v)", err)
		return nil, nil
	}

	cleanup := func() {
		_ = rdb.Close()
	}
	return rdb, cleanup
}

func TestTimelineStore_AppendListAndTrim(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	if rdb == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	testID := fmt.Sprintf("test_timeline_%d", time.Now().UnixNano())
	globalKey := "timeline:global:" + testID
	floorKey := "timeline:floor:" + testID

	store := persistencefeed.NewCustomTimelineStore(rdb, globalKey, floorKey)

	defer func() {
		_ = rdb.Del(ctx, globalKey, floorKey).Err()
	}()

	// 1. Initial floor check
	floor, err := store.GetFloor(ctx)
	if err != nil {
		t.Fatalf("unexpected GetFloor error: %v", err)
	}
	if floor != 0 {
		t.Fatalf("expected floor 0, got %d", floor)
	}

	// 2. Append videos: 100, 105, 110, 115, 120
	videoIDs := []int64{100, 105, 110, 115, 120}
	for _, id := range videoIDs {
		if err := store.Append(ctx, id); err != nil {
			t.Fatalf("failed to append video %d: %v", id, err)
		}
	}

	// 3. List with cursor = 0 (top page, limit 3) -> should return 120, 115, 110
	top3, err := store.ListByCursor(ctx, 0, 3)
	if err != nil {
		t.Fatalf("failed to list by cursor 0: %v", err)
	}
	if len(top3) != 3 || top3[0] != 120 || top3[1] != 115 || top3[2] != 110 {
		t.Fatalf("expected [120, 115, 110], got %v", top3)
	}

	// 4. Paginate strictly below cursor 110 (limit 2) -> should return 105, 100
	nextPage, err := store.ListByCursor(ctx, 110, 2)
	if err != nil {
		t.Fatalf("failed to list by cursor 110: %v", err)
	}
	if len(nextPage) != 2 || nextPage[0] != 105 || nextPage[1] != 100 {
		t.Fatalf("expected [105, 100], got %v", nextPage)
	}

	// 5. Trim below floor = 110 (removes 100, 105; keeps >= 110)
	if err := store.Trim(ctx, 110); err != nil {
		t.Fatalf("failed to trim timeline: %v", err)
	}

	floor, err = store.GetFloor(ctx)
	if err != nil {
		t.Fatalf("failed to get floor after trim: %v", err)
	}
	if floor != 110 {
		t.Fatalf("expected floor 110, got %d", floor)
	}

	// Verify remaining items in ZSet: should only have 120, 115, 110
	remaining, err := store.ListByCursor(ctx, 0, 10)
	if err != nil {
		t.Fatalf("failed to list remaining: %v", err)
	}
	if len(remaining) != 3 || remaining[0] != 120 || remaining[1] != 115 || remaining[2] != 110 {
		t.Fatalf("expected remaining [120, 115, 110], got %v", remaining)
	}

	// 6. Test Rebuild
	rebuildIDs := []int64{201, 202, 203}
	if err := store.Rebuild(ctx, rebuildIDs, 201); err != nil {
		t.Fatalf("failed to rebuild timeline: %v", err)
	}

	rebuilt, err := store.ListByCursor(ctx, 0, 10)
	if err != nil {
		t.Fatalf("failed to list rebuilt: %v", err)
	}
	if len(rebuilt) != 3 || rebuilt[0] != 203 || rebuilt[1] != 202 || rebuilt[2] != 201 {
		t.Fatalf("expected rebuilt [203, 202, 201], got %v", rebuilt)
	}

	newFloor, err := store.GetFloor(ctx)
	if err != nil {
		t.Fatalf("failed to get rebuilt floor: %v", err)
	}
	if newFloor != 201 {
		t.Fatalf("expected floor 201, got %d", newFloor)
	}
}
