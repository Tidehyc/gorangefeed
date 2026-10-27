package feed_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	persistencefeed "github.com/Tidehyc/gorangefeed/internal/infra/persistence/feed"
)

func TestCacheRepositories(t *testing.T) {
	rdb, cleanup := setupTestRedis(t)
	if rdb == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	testUID := time.Now().UnixNano()

	// 1. Page Cache
	pageRepo := persistencefeed.NewPageCacheRepository(rdb)
	pageKey := fmt.Sprintf("test:feed:page:%d", testUID)
	defer func() { _ = rdb.Del(ctx, pageKey).Err() }()

	cachedPage := &domainfeed.CachedPage{
		VideoIDs:   []int64{101, 102, 103},
		NextCursor: 101,
		HasMore:    true,
		ExpireAt:   time.Now().Add(10 * time.Second),
	}

	if err := pageRepo.SetPage(ctx, pageKey, cachedPage, 20*time.Second); err != nil {
		t.Fatalf("failed to set page cache: %v", err)
	}

	gotPage, found, err := pageRepo.GetPage(ctx, pageKey)
	if err != nil {
		t.Fatalf("failed to get page cache: %v", err)
	}
	if !found || gotPage == nil {
		t.Fatalf("expected page to be found")
	}
	if len(gotPage.VideoIDs) != 3 || gotPage.NextCursor != 101 || !gotPage.HasMore {
		t.Fatalf("unexpected cached page: %+v", gotPage)
	}

	if err := pageRepo.DeletePage(ctx, pageKey); err != nil {
		t.Fatalf("failed to delete page cache: %v", err)
	}
	_, foundAfterDel, _ := pageRepo.GetPage(ctx, pageKey)
	if foundAfterDel {
		t.Fatalf("expected deleted page to not be found")
	}

	// 2. Card Cache
	cardRepo := persistencefeed.NewCardCacheRepository(rdb)
	v1ID := testUID
	v2ID := testUID + 1
	defer func() {
		_ = rdb.Del(ctx, fmt.Sprintf("feed:card:%d", v1ID), fmt.Sprintf("feed:card:%d", v2ID)).Err()
	}()

	cards := []*domainfeed.FeedCard{
		{
			VideoID:     v1ID,
			AuthorID:    99,
			Title:       "Test Video 1",
			Description: "Desc 1",
			PlayURL:     "/media/videos/1.mp4",
			DurationMs:  12000,
			PublishedAt: time.Now().UTC().Truncate(time.Millisecond),
			Author: domainfeed.FeedAuthor{
				ID:       99,
				Username: "author99",
				Nickname: "Nick 99",
			},
		},
		{
			VideoID:     v2ID,
			AuthorID:    100,
			Title:       "Test Video 2",
			PlayURL:     "/media/videos/2.mp4",
			PublishedAt: time.Now().UTC().Truncate(time.Millisecond),
			Author: domainfeed.FeedAuthor{
				ID:       100,
				Username: "author100",
			},
		},
	}

	if err := cardRepo.SetCards(ctx, cards, 1*time.Minute); err != nil {
		t.Fatalf("failed to set cards: %v", err)
	}

	fetchedCards, err := cardRepo.GetCards(ctx, []int64{v1ID, v2ID, 999999})
	if err != nil {
		t.Fatalf("failed to get cards: %v", err)
	}
	if len(fetchedCards) != 2 {
		t.Fatalf("expected 2 cards returned, got %d", len(fetchedCards))
	}
	if fetchedCards[v1ID].Title != "Test Video 1" || fetchedCards[v1ID].Author.Username != "author99" {
		t.Fatalf("mismatched card 1: %+v", fetchedCards[v1ID])
	}
	if fetchedCards[v2ID].Title != "Test Video 2" {
		t.Fatalf("mismatched card 2: %+v", fetchedCards[v2ID])
	}

	// 3. Stat Cache
	statRepo := persistencefeed.NewStatCacheRepository(rdb)
	defer func() {
		_ = rdb.Del(ctx, fmt.Sprintf("feed:stat:%d", v1ID), fmt.Sprintf("feed:stat:%d", v2ID)).Err()
	}()

	stats := []*domainfeed.FeedStat{
		{VideoID: v1ID, LikeCount: 10, CommentCount: 2, ViewCount: 100},
		{VideoID: v2ID, LikeCount: 0, CommentCount: 0, ViewCount: 5},
	}
	if err := statRepo.SetStats(ctx, stats, 1*time.Minute); err != nil {
		t.Fatalf("failed to set stats: %v", err)
	}

	fetchedStats, err := statRepo.GetStats(ctx, []int64{v1ID, v2ID, 888888})
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if len(fetchedStats) != 2 {
		t.Fatalf("expected 2 stats returned, got %d", len(fetchedStats))
	}
	if fetchedStats[v1ID].ViewCount != 100 || fetchedStats[v2ID].ViewCount != 5 {
		t.Fatalf("mismatched stats: %+v", fetchedStats)
	}

	if err := statRepo.InvalidateStat(ctx, v1ID); err != nil {
		t.Fatalf("failed to invalidate stat: %v", err)
	}
	statsAfterInv, _ := statRepo.GetStats(ctx, []int64{v1ID})
	if len(statsAfterInv) != 0 {
		t.Fatalf("expected invalidated stat to be removed, got: %+v", statsAfterInv)
	}

	// 4. Exposure Repository
	expoRepo := persistencefeed.NewExposureRepository(rdb)
	viewerID := testUID
	expoKey := fmt.Sprintf("reco:exposed:%d", viewerID)
	defer func() { _ = rdb.Del(ctx, expoKey).Err() }()

	isExpo, err := expoRepo.IsExposed(ctx, viewerID, v1ID)
	if err != nil || isExpo {
		t.Fatalf("expected not exposed before recording, got %v, err: %v", isExpo, err)
	}

	if err := expoRepo.RecordExposures(ctx, viewerID, []int64{v1ID, v2ID}); err != nil {
		t.Fatalf("failed to record exposures: %v", err)
	}

	isExpo1, err := expoRepo.IsExposed(ctx, viewerID, v1ID)
	if err != nil || !isExpo1 {
		t.Fatalf("expected v1ID exposed, got %v, err: %v", isExpo1, err)
	}
	isExpo2, err := expoRepo.IsExposed(ctx, viewerID, v2ID)
	if err != nil || !isExpo2 {
		t.Fatalf("expected v2ID exposed, got %v, err: %v", isExpo2, err)
	}
	isExpoOther, err := expoRepo.IsExposed(ctx, viewerID, 777777)
	if err != nil || isExpoOther {
		t.Fatalf("expected other not exposed, got %v", isExpoOther)
	}
}
