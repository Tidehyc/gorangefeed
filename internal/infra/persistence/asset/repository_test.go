package asset_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceasset "github.com/Tidehyc/gorangefeed/internal/infra/persistence/asset"
	persistencevideo "github.com/Tidehyc/gorangefeed/internal/infra/persistence/video"
	infraredis "github.com/Tidehyc/gorangefeed/internal/infra/redis"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func setupTestInfra(t *testing.T) (*gorm.DB, *redis.Client, domainasset.FavoriteRepository, domainasset.HistoryRepository) {
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
		t.Skipf("skipping asset persistence test: config load failed (%v)", err)
		return nil, nil, nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping asset persistence test: MySQL connection failed (%v)", err)
		return nil, nil, nil, nil
	}

	rdb, err := infraredis.New(&cfg.Redis)
	if err != nil {
		t.Skipf("skipping asset persistence test: Redis connection failed (%v)", err)
		return nil, nil, nil, nil
	}

	favoriteRepo := persistenceasset.NewFavoriteRepository(db)
	historyRepo := persistenceasset.NewHistoryRepository(db, rdb)
	return db, rdb, favoriteRepo, historyRepo
}

func createTestVideoWithStats(t *testing.T, db *gorm.DB, authorID int64) *domainvideo.Video {
	t.Helper()
	videoRepo := persistencevideo.NewRepository(db)
	ctx := context.Background()

	v := &domainvideo.Video{
		AuthorID:   authorID,
		Title:      fmt.Sprintf("Asset Test Video %d", time.Now().UnixNano()),
		PlayURL:    "https://cdn.example.com/videos/asset_test.mp4",
		DurationMs: 60000,
		Status:     domainvideo.VideoStatusPublished,
	}
	stats := &domainvideo.VideoStats{
		LikeCount:     0,
		CommentCount:  0,
		ViewCount:     0,
		FavoriteCount: 0,
	}

	if err := videoRepo.CreateWithStats(ctx, v, stats); err != nil {
		t.Fatalf("failed to create test video: %v", err)
	}
	return v
}

func TestFavoriteRepository_DefaultFolderAutoInitAndConcurrency(t *testing.T) {
	_, _, favoriteRepo, _ := setupTestInfra(t)
	ctx := context.Background()
	userID := time.Now().UnixNano()

	// 1. Initial retrieval creates default folder
	folder1, err := favoriteRepo.GetOrCreateDefaultFolder(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get or create default folder: %v", err)
	}
	if folder1.ID <= 0 {
		t.Fatalf("expected positive folder ID, got %d", folder1.ID)
	}
	if !folder1.IsDefault {
		t.Fatalf("expected folder to be default")
	}
	if folder1.Name != domainasset.DefaultFolderName {
		t.Fatalf("expected name %s, got %s", domainasset.DefaultFolderName, folder1.Name)
	}

	// 2. Subsequent call returns identical default folder
	folder2, err := favoriteRepo.GetOrCreateDefaultFolder(ctx, userID)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if folder2.ID != folder1.ID {
		t.Fatalf("expected identical folder ID %d, got %d", folder1.ID, folder2.ID)
	}

	// 3. Concurrent requests for same user return same default folder
	concurrentUserID := userID + 1
	var wg sync.WaitGroup
	errCh := make(chan error, 10)
	folderIDs := make([]int64, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			f, err := favoriteRepo.GetOrCreateDefaultFolder(ctx, concurrentUserID)
			if err != nil {
				errCh <- err
				return
			}
			folderIDs[idx] = f.ID
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent default folder init error: %v", err)
	}

	firstID := folderIDs[0]
	for _, id := range folderIDs {
		if id != firstID {
			t.Fatalf("expected all concurrent routines to get folder ID %d, got %d", firstID, id)
		}
	}
}

func TestFavoriteRepository_FolderCRUDAndRules(t *testing.T) {
	_, _, favoriteRepo, _ := setupTestInfra(t)
	ctx := context.Background()
	userID := time.Now().UnixNano()

	// 1. Get default folder
	defaultFolder, err := favoriteRepo.GetOrCreateDefaultFolder(ctx, userID)
	if err != nil {
		t.Fatalf("failed to create default folder: %v", err)
	}

	// 2. Default folder cannot be deleted
	if err := favoriteRepo.DeleteFolder(ctx, userID, defaultFolder.ID); err != domainasset.ErrCannotDeleteDefaultFolder {
		t.Fatalf("expected ErrCannotDeleteDefaultFolder, got %v", err)
	}

	// 3. Create custom folder
	customFolder := &domainasset.FavoriteFolder{
		UserID:      userID,
		Name:        "My Custom Folder",
		Description: "Special favorites",
		IsPublic:    true,
	}
	if err := favoriteRepo.CreateFolder(ctx, customFolder); err != nil {
		t.Fatalf("failed to create custom folder: %v", err)
	}
	if customFolder.ID <= 0 {
		t.Fatalf("expected positive custom folder ID, got %d", customFolder.ID)
	}
	if customFolder.IsDefault {
		t.Fatalf("custom folder must not be marked default")
	}

	// 4. FindFolderByID
	found, err := favoriteRepo.FindFolderByID(ctx, customFolder.ID)
	if err != nil {
		t.Fatalf("failed to find folder by ID: %v", err)
	}
	if found.Name != customFolder.Name {
		t.Fatalf("expected name %s, got %s", customFolder.Name, found.Name)
	}

	// 5. ListFolders (default folder must be first)
	folders, err := favoriteRepo.ListFolders(ctx, userID)
	if err != nil {
		t.Fatalf("failed to list folders: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(folders))
	}
	if !folders[0].IsDefault {
		t.Fatalf("expected first folder in list to be default folder")
	}

	// 6. Delete permission check: another user cannot delete this folder
	otherUserID := userID + 999
	if err := favoriteRepo.DeleteFolder(ctx, otherUserID, customFolder.ID); err != domainasset.ErrFolderAccessDenied {
		t.Fatalf("expected ErrFolderAccessDenied, got %v", err)
	}

	// 7. Delete custom folder succeeds
	if err := favoriteRepo.DeleteFolder(ctx, userID, customFolder.ID); err != nil {
		t.Fatalf("failed to delete custom folder: %v", err)
	}
	_, err = favoriteRepo.FindFolderByID(ctx, customFolder.ID)
	if err != domainasset.ErrFolderNotFound {
		t.Fatalf("expected ErrFolderNotFound after deletion, got %v", err)
	}
}

func TestFavoriteRepository_ItemFavoritesAndStatsAtomicCounting(t *testing.T) {
	db, _, favoriteRepo, _ := setupTestInfra(t)
	ctx := context.Background()
	userID := time.Now().UnixNano()

	// Setup video and folder
	video := createTestVideoWithStats(t, db, userID)
	folder, err := favoriteRepo.GetOrCreateDefaultFolder(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get default folder: %v", err)
	}

	// 1. Initial IsFavorited check: false
	isFav, err := favoriteRepo.IsFavorited(ctx, userID, video.ID)
	if err != nil {
		t.Fatalf("failed to check isFavorited: %v", err)
	}
	if isFav {
		t.Fatalf("expected initially not favorited")
	}

	// 2. Add video to folder
	item := &domainasset.FavoriteItem{
		FavoriteID: folder.ID,
		VideoID:    video.ID,
		UserID:     userID,
	}
	inserted, err := favoriteRepo.AddItem(ctx, item, nil)
	if err != nil {
		t.Fatalf("failed to add item: %v", err)
	}
	if !inserted {
		t.Fatalf("expected item to be newly inserted")
	}

	// 3. Verify folder item_count incremented to 1
	refreshedFolder, _ := favoriteRepo.FindFolderByID(ctx, folder.ID)
	if refreshedFolder.ItemCount != 1 {
		t.Fatalf("expected folder item_count 1, got %d", refreshedFolder.ItemCount)
	}

	// 4. Verify video_stats.favorite_count incremented to 1
	var stat persistencevideo.VideoStatModel
	if err := db.Where("video_id = ?", video.ID).First(&stat).Error; err != nil {
		t.Fatalf("failed to query video_stats: %v", err)
	}
	if stat.FavoriteCount != 1 {
		t.Fatalf("expected video_stats.favorite_count 1, got %d", stat.FavoriteCount)
	}

	// 5. Duplicate add is idempotent: returns false, counts unchanged
	insertedAgain, err := favoriteRepo.AddItem(ctx, item, nil)
	if err != nil {
		t.Fatalf("second add failed: %v", err)
	}
	if insertedAgain {
		t.Fatalf("expected duplicate add to return false")
	}

	// 6. Check IsFavorited and BatchIsFavorited
	isFav, _ = favoriteRepo.IsFavorited(ctx, userID, video.ID)
	if !isFav {
		t.Fatalf("expected IsFavorited to be true")
	}

	batchMap, err := favoriteRepo.BatchIsFavorited(ctx, userID, []int64{video.ID, 999999})
	if err != nil {
		t.Fatalf("batch check failed: %v", err)
	}
	if !batchMap[video.ID] || batchMap[999999] {
		t.Fatalf("unexpected batch favorite result: %+v", batchMap)
	}

	// 7. ListItems pagination
	items, nextCursor, hasMore, err := favoriteRepo.ListItems(ctx, folder.ID, 0, 10)
	if err != nil {
		t.Fatalf("failed to list items: %v", err)
	}
	if len(items) != 1 || items[0].VideoID != video.ID {
		t.Fatalf("unexpected items list: %+v", items)
	}
	if hasMore || nextCursor != 0 {
		t.Fatalf("expected hasMore false and nextCursor 0")
	}

	// 8. RemoveItem
	removed, err := favoriteRepo.RemoveItem(ctx, folder.ID, video.ID, nil)
	if err != nil {
		t.Fatalf("failed to remove item: %v", err)
	}
	if !removed {
		t.Fatalf("expected remove to succeed")
	}

	// 9. Verify folder item_count decremented to 0
	refreshedFolder, _ = favoriteRepo.FindFolderByID(ctx, folder.ID)
	if refreshedFolder.ItemCount != 0 {
		t.Fatalf("expected folder item_count 0, got %d", refreshedFolder.ItemCount)
	}

	// 10. Verify video_stats.favorite_count decremented to 0
	if err := db.Where("video_id = ?", video.ID).First(&stat).Error; err != nil {
		t.Fatalf("failed to query video_stats: %v", err)
	}
	if stat.FavoriteCount != 0 {
		t.Fatalf("expected video_stats.favorite_count 0, got %d", stat.FavoriteCount)
	}

	// 11. Remove again: returns false, count never drops below 0
	removedAgain, err := favoriteRepo.RemoveItem(ctx, folder.ID, video.ID, nil)
	if err != nil {
		t.Fatalf("duplicate remove error: %v", err)
	}
	if removedAgain {
		t.Fatalf("expected duplicate remove to return false")
	}
	if err := db.Where("video_id = ?", video.ID).First(&stat).Error; err != nil {
		t.Fatalf("failed to query video_stats: %v", err)
	}
	if stat.FavoriteCount != 0 {
		t.Fatalf("favorite_count must remain 0, got %d", stat.FavoriteCount)
	}
}

func TestFavoriteRepository_FolderDeletionCascading(t *testing.T) {
	db, _, favoriteRepo, _ := setupTestInfra(t)
	ctx := context.Background()
	userID := time.Now().UnixNano()

	v1 := createTestVideoWithStats(t, db, userID)
	v2 := createTestVideoWithStats(t, db, userID)

	folder := &domainasset.FavoriteFolder{
		UserID:   userID,
		Name:     "Folder To Delete",
		IsPublic: false,
	}
	if err := favoriteRepo.CreateFolder(ctx, folder); err != nil {
		t.Fatalf("failed to create folder: %v", err)
	}

	_, _ = favoriteRepo.AddItem(ctx, &domainasset.FavoriteItem{FavoriteID: folder.ID, VideoID: v1.ID, UserID: userID}, nil)
	_, _ = favoriteRepo.AddItem(ctx, &domainasset.FavoriteItem{FavoriteID: folder.ID, VideoID: v2.ID, UserID: userID}, nil)

	// Check stats before deletion
	var s1, s2 persistencevideo.VideoStatModel
	_ = db.Where("video_id = ?", v1.ID).First(&s1).Error
	_ = db.Where("video_id = ?", v2.ID).First(&s2).Error
	if s1.FavoriteCount != 1 || s2.FavoriteCount != 1 {
		t.Fatalf("expected both favorite_counts to be 1")
	}

	// Delete folder
	if err := favoriteRepo.DeleteFolder(ctx, userID, folder.ID); err != nil {
		t.Fatalf("failed to delete folder: %v", err)
	}

	// Verify stats decremented
	_ = db.Where("video_id = ?", v1.ID).First(&s1).Error
	_ = db.Where("video_id = ?", v2.ID).First(&s2).Error
	if s1.FavoriteCount != 0 || s2.FavoriteCount != 0 {
		t.Fatalf("expected both favorite_counts to decrement to 0, got %d, %d", s1.FavoriteCount, s2.FavoriteCount)
	}
}

func TestHistoryRepository_RedisSlidingWindowAndTrimming(t *testing.T) {
	db, rdb, _, _ := setupTestInfra(t)
	ctx := context.Background()
	userID := time.Now().UnixNano()

	// Use custom small retention for exact sliding window assertion
	const testRetention = 5
	customRepo := persistenceasset.NewHistoryRepositoryWithRetention(db, rdb, testRetention)
	_ = customRepo.ClearHistory(ctx, userID)

	// Record 8 watch progress entries sequentially
	now := time.Now()
	for i := 1; i <= 8; i++ {
		hist := &domainasset.WatchHistory{
			UserID:         userID,
			VideoID:        int64(i * 100),
			LastPositionMs: i * 1000,
			DurationMs:     10000,
			WatchedAt:      now.Add(time.Duration(i) * time.Second),
		}
		if err := customRepo.RecordProgressSync(ctx, hist); err != nil {
			t.Fatalf("failed to record progress sync for i=%d: %v", i, err)
		}
	}

	// Assert Redis ZSet strictly trimmed to testRetention (5)
	zKey := fmt.Sprintf("asset:history:zset:%d", userID)
	dKey := fmt.Sprintf("asset:history:detail:%d", userID)

	card, err := rdb.ZCard(ctx, zKey).Result()
	if err != nil {
		t.Fatalf("failed to check zcard: %v", err)
	}
	if card != testRetention {
		t.Fatalf("expected ZCard %d, got %d", testRetention, card)
	}

	hashLen, err := rdb.HLen(ctx, dKey).Result()
	if err != nil {
		t.Fatalf("failed to check hlen: %v", err)
	}
	if hashLen != testRetention {
		t.Fatalf("expected HLen %d, got %d", testRetention, hashLen)
	}

	// Verify the oldest entries (100, 200, 300) are trimmed from Redis Hash
	oldVal, err := rdb.HGet(ctx, dKey, "100").Result()
	if !errors.Is(err, redis.Nil) {
		t.Fatalf("expected video 100 to be trimmed, got %v", oldVal)
	}

	// Verify newest entry (800) is present
	newVal, err := rdb.HGet(ctx, dKey, "800").Result()
	if err != nil || newVal == "" {
		t.Fatalf("expected video 800 to be present in Redis")
	}

	_ = customRepo.ClearHistory(ctx, userID)
}

func TestHistoryRepository_BreakpointResumeAndCacheFallback(t *testing.T) {
	_, rdb, _, historyRepo := setupTestInfra(t)
	ctx := context.Background()
	userID := time.Now().UnixNano()
	videoID := int64(8888)

	_ = historyRepo.ClearHistory(ctx, userID)

	hist := &domainasset.WatchHistory{
		UserID:         userID,
		VideoID:        videoID,
		LastPositionMs: 25400,
		DurationMs:     60000,
		WatchedAt:      time.Now(),
	}

	// 1. Record sync
	if err := historyRepo.RecordProgressSync(ctx, hist); err != nil {
		t.Fatalf("failed to record progress sync: %v", err)
	}

	// 2. Breakpoint resume lookup from Redis
	got, err := historyRepo.GetProgress(ctx, userID, videoID)
	if err != nil {
		t.Fatalf("failed to get progress: %v", err)
	}
	if got == nil || got.LastPositionMs != 25400 {
		t.Fatalf("expected last position 25400, got %+v", got)
	}

	// 3. Simulate Redis cache eviction: DEL Redis keys
	zKey := fmt.Sprintf("asset:history:zset:%d", userID)
	dKey := fmt.Sprintf("asset:history:detail:%d", userID)
	_ = rdb.Del(ctx, zKey, dKey).Err()

	// 4. Fallback lookup from MySQL: should succeed and warm up Redis
	recovered, err := historyRepo.GetProgress(ctx, userID, videoID)
	if err != nil {
		t.Fatalf("failed to get progress after cache eviction: %v", err)
	}
	if recovered == nil || recovered.LastPositionMs != 25400 {
		t.Fatalf("expected recovered last position 25400 from MySQL, got %+v", recovered)
	}

	// 5. Verify Redis was rewarmed
	cachedVal, err := rdb.HGet(ctx, dKey, fmt.Sprintf("%d", videoID)).Result()
	if err != nil || cachedVal == "" {
		t.Fatalf("expected Redis to be rewarmed with video %d", videoID)
	}

	// 6. Test ListHistory
	list, err := historyRepo.ListHistory(ctx, userID, 10)
	if err != nil {
		t.Fatalf("failed to list history: %v", err)
	}
	if len(list) != 1 || list[0].VideoID != videoID {
		t.Fatalf("unexpected history list: %+v", list)
	}

	// 7. ClearHistory
	if err := historyRepo.ClearHistory(ctx, userID); err != nil {
		t.Fatalf("failed to clear history: %v", err)
	}
	cleared, _ := historyRepo.GetProgress(ctx, userID, videoID)
	if cleared != nil {
		t.Fatalf("expected nil progress after clear, got %+v", cleared)
	}
}

func TestFavoriteRepository_OutboxEventInTransaction(t *testing.T) {
	db, _, favoriteRepo, _ := setupTestInfra(t)
	ctx := context.Background()
	userID := time.Now().UnixNano()

	video := createTestVideoWithStats(t, db, userID)
	folder, err := favoriteRepo.GetOrCreateDefaultFolder(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get default folder: %v", err)
	}

	eventID := fmt.Sprintf("test_fav_evt_%d", time.Now().UnixNano())
	outboxEvent := &domainoutbox.Event{
		EventID:   eventID,
		EventType: "interaction.favorited",
		Payload:   []byte(fmt.Sprintf(`{"user_id":%d,"video_id":%d}`, userID, video.ID)),
	}

	item := &domainasset.FavoriteItem{
		FavoriteID: folder.ID,
		VideoID:    video.ID,
		UserID:     userID,
	}

	inserted, err := favoriteRepo.AddItem(ctx, item, outboxEvent)
	if err != nil {
		t.Fatalf("failed to add item with outbox: %v", err)
	}
	if !inserted {
		t.Fatalf("expected item to be inserted")
	}

	// Verify outbox row exists in MySQL
	var count int64
	if err := db.Table("outbox").Where("event_id = ?", eventID).Count(&count).Error; err != nil {
		t.Fatalf("failed to query outbox: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 outbox event, got %d", count)
	}

	// Remove with outbox event
	unfavEventID := fmt.Sprintf("test_unfav_evt_%d", time.Now().UnixNano())
	unfavOutboxEvent := &domainoutbox.Event{
		EventID:   unfavEventID,
		EventType: "interaction.unfavorited",
		Payload:   []byte(fmt.Sprintf(`{"user_id":%d,"video_id":%d}`, userID, video.ID)),
	}

	removed, err := favoriteRepo.RemoveItem(ctx, folder.ID, video.ID, unfavOutboxEvent)
	if err != nil {
		t.Fatalf("failed to remove item with outbox: %v", err)
	}
	if !removed {
		t.Fatalf("expected remove to succeed")
	}

	if err := db.Table("outbox").Where("event_id = ?", unfavEventID).Count(&count).Error; err != nil {
		t.Fatalf("failed to query outbox for unfavorite: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 unfavorite outbox event, got %d", count)
	}
}
