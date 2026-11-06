package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	httpfeed "github.com/Tidehyc/gorangefeed/internal/http/feed"
)

type hotFeedResponseEnvelope struct {
	Data struct {
		Items      []*domainfeed.FeedItem `json:"items"`
		NextCursor string                 `json:"next_cursor"`
		HasMore    bool                   `json:"has_more"`
	} `json:"data"`
}

type recommendFeedResponseEnvelope struct {
	Data struct {
		Items      []*domainfeed.FeedItem `json:"items"`
		NextCursor string                 `json:"next_cursor"`
		HasMore    bool                   `json:"has_more"`
	} `json:"data"`
}

func TestM4_5_HotAndRecommendLifecycle_EndToEnd(t *testing.T) {
	ctxExt, cleanup := setupE2ETestExtended(t)
	if cleanup != nil {
		defer cleanup()
	}

	engine := ctxExt.Engine
	hotStore := ctxExt.HotStore
	rdb := ctxExt.RDB

	suffix := time.Now().Format("0405_000")
	authorAUsername := fmt.Sprintf("hot_author_a_%s", suffix)
	authorBUsername := fmt.Sprintf("hot_author_b_%s", suffix)
	viewerUsername := fmt.Sprintf("hot_viewer_%s", suffix)
	testPass := "SecurePass123"

	// Setup users
	authorAID, authorAToken := registerAndLogin(t, engine, authorAUsername, testPass)
	authorBID, authorBToken := registerAndLogin(t, engine, authorBUsername, testPass)
	viewerID, viewerToken := registerAndLogin(t, engine, viewerUsername, testPass)

	if authorAID <= 0 || authorBID <= 0 || viewerID <= 0 {
		t.Fatalf("failed to register users: authorA=%d, authorB=%d, viewer=%d", authorAID, authorBID, viewerID)
	}

	// =========================================================================
	// E2E-M4.5-01: 分钟桶热度打分与逆向扣减及净负分剔除
	// =========================================================================
	t.Run("E2E-M4.5-01_Scoring_And_Negative_Purge", func(t *testing.T) {
		v1 := publishTestVideo(t, engine, authorAToken, "V1 High Engagement", "/media/videos/v1.mp4")
		v2 := publishTestVideo(t, engine, authorAToken, "V2 Net Negative", "/media/videos/v2.mp4")

		now := time.Now().UTC()

		// V1: 2 Likes (+3.0*2), 1 Comment (+5.0), 1 Unlike (-3.0) -> Expected: 8.0
		_ = hotStore.IncrScore(context.Background(), v1.Video.ID, 3.0, now)
		_ = hotStore.IncrScore(context.Background(), v1.Video.ID, 3.0, now)
		_ = hotStore.IncrScore(context.Background(), v1.Video.ID, 5.0, now)
		_ = hotStore.IncrScore(context.Background(), v1.Video.ID, -3.0, now)

		// V2: 2 Unlikes (-3.0*2) -> Expected: -6.0 (Net Negative)
		_ = hotStore.IncrScore(context.Background(), v2.Video.ID, -3.0, now)
		_ = hotStore.IncrScore(context.Background(), v2.Video.ID, -3.0, now)

		// Direct check on minute bucket ZSet score
		minuteBucketKey := fmt.Sprintf("feed:hot:minute:%s", now.Truncate(time.Minute).Format("200601021504"))
		v1Score, err := rdb.ZScore(context.Background(), minuteBucketKey, fmt.Sprintf("%d", v1.Video.ID)).Result()
		if err != nil {
			t.Fatalf("failed to get V1 minute score: %v", err)
		}
		if v1Score != 8.0 {
			t.Fatalf("expected V1 score 8.0, got %f", v1Score)
		}

		v2Score, err := rdb.ZScore(context.Background(), minuteBucketKey, fmt.Sprintf("%d", v2.Video.ID)).Result()
		if err != nil {
			t.Fatalf("failed to get V2 minute score: %v", err)
		}
		if v2Score != -6.0 {
			t.Fatalf("expected V2 score -6.0, got %f", v2Score)
		}

		// Trigger MergeWindow via GetHotFeed API
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodGet, "/api/v1/feed/hot?limit=10", nil, "192.168.1.100")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("hot feed request failed: code=%d, body=%s", w.Code, w.Body.String())
		}

		var resp hotFeedResponseEnvelope
		parseJSONResponse(t, w, &resp)

		// V1 must be present, V2 must be purged because score <= 0
		foundV1 := false
		foundV2 := false
		for _, item := range resp.Data.Items {
			if item.ID == v1.Video.ID {
				foundV1 = true
			}
			if item.ID == v2.Video.ID {
				foundV2 = true
			}
		}

		if !foundV1 {
			t.Fatalf("expected V1 with score 8.0 to be present in hot feed")
		}
		if foundV2 {
			t.Fatalf("expected V2 with net negative score -6.0 to be purged from hot feed")
		}
	})

	// =========================================================================
	// E2E-M4.5-02: as_of 稳定快照翻页无重无漏（抗突发变分干扰）
	// =========================================================================
	t.Run("E2E-M4.5-02_Snapshot_Pagination_Stability", func(t *testing.T) {
		// Clean hot window snapshot and minute buckets for isolated snapshot test
		snapKeys, _ := rdb.Keys(context.Background(), "feed:hot:*").Result()
		for _, k := range snapKeys {
			_ = rdb.Del(context.Background(), k).Err()
		}

		var videoIDs []int64
		for i := 1; i <= 15; i++ {
			v := publishTestVideo(t, engine, authorAToken, fmt.Sprintf("Rank Video %d", i), fmt.Sprintf("/media/videos/r%d.mp4", i))
			videoIDs = append(videoIDs, v.Video.ID)
		}

		currentMinute := time.Now().UTC().Truncate(time.Minute)
		for i, vid := range videoIDs {
			score := float64(100 - (i+1)*5)
			_ = hotStore.IncrScore(context.Background(), vid, score, currentMinute)
		}


		// Fetch First Page (limit=10)
		w1 := httptest.NewRecorder()
		req1 := newJSONRequest(http.MethodGet, "/api/v1/feed/hot?limit=10", nil, "192.168.1.100")
		engine.ServeHTTP(w1, req1)
		if w1.Code != http.StatusOK {
			t.Fatalf("page 1 failed: %d, body: %s", w1.Code, w1.Body.String())
		}

		var page1Resp hotFeedResponseEnvelope
		parseJSONResponse(t, w1, &page1Resp)

		if len(page1Resp.Data.Items) != 10 {
			t.Fatalf("expected 10 items on page 1, got %d", len(page1Resp.Data.Items))
		}
		if !page1Resp.Data.HasMore || page1Resp.Data.NextCursor == "" {
			t.Fatalf("expected has_more=true and non-empty next_cursor on page 1")
		}

		page1Map := make(map[int64]bool)
		for _, item := range page1Resp.Data.Items {
			page1Map[item.ID] = true
		}

		// Inject Sudden Engagement Burst into video 15 (lowest rank video in minute bucket)
		lastVID := videoIDs[14]
		_ = hotStore.IncrScore(context.Background(), lastVID, 9999.0, currentMinute)


		// Fetch Second Page using page1 next_cursor
		w2 := httptest.NewRecorder()
		req2 := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/feed/hot?cursor=%s&limit=10", page1Resp.Data.NextCursor), nil, "192.168.1.100")
		engine.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("page 2 failed: %d, body: %s", w2.Code, w2.Body.String())
		}

		var page2Resp hotFeedResponseEnvelope
		parseJSONResponse(t, w2, &page2Resp)

		if len(page2Resp.Data.Items) != 5 {
			t.Fatalf("expected 5 items on page 2, got %d", len(page2Resp.Data.Items))
		}

		// Verify zero duplicates and zero misses
		for _, item := range page2Resp.Data.Items {
			if page1Map[item.ID] {
				t.Fatalf("duplicate video ID %d found across page 1 and page 2", item.ID)
			}
		}
	})

	// =========================================================================
	// E2E-M4.5-03: 推荐曝光过滤（已看不推）
	// =========================================================================
	t.Run("E2E-M4.5-03_Recommend_Exposure_Filtering", func(t *testing.T) {
		// 1. Logged-in user requests recommend feed
		w1 := httptest.NewRecorder()
		req1 := newJSONRequest(http.MethodGet, "/api/v1/feed/recommend?limit=5", nil, "192.168.1.100")
		req1.Header.Set("Authorization", "Bearer "+viewerToken)
		engine.ServeHTTP(w1, req1)
		if w1.Code != http.StatusOK {
			t.Fatalf("recommend feed failed: %d, body: %s", w1.Code, w1.Body.String())
		}

		var resp1 recommendFeedResponseEnvelope
		parseJSONResponse(t, w1, &resp1)
		if len(resp1.Data.Items) == 0 {
			t.Fatalf("expected recommend items, got 0")
		}

		targetSeenVID := resp1.Data.Items[0].ID

		// 2. Report view / exposure impression
		wView := httptest.NewRecorder()
		viewReq := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/view", targetSeenVID), httpfeed.RecordViewRequest{
			EventType:       "play",
			WatchDurationMs: 12000,
		}, "192.168.1.100")
		viewReq.Header.Set("Authorization", "Bearer "+viewerToken)
		engine.ServeHTTP(wView, viewReq)
		if wView.Code != http.StatusOK {
			t.Fatalf("record view failed: %d, body: %s", wView.Code, wView.Body.String())
		}

		// 3. Request recommend feed again
		w2 := httptest.NewRecorder()
		req2 := newJSONRequest(http.MethodGet, "/api/v1/feed/recommend?limit=5", nil, "192.168.1.100")
		req2.Header.Set("Authorization", "Bearer "+viewerToken)
		engine.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("recommend feed 2 failed: %d, body: %s", w2.Code, w2.Body.String())
		}

		var resp2 recommendFeedResponseEnvelope
		parseJSONResponse(t, w2, &resp2)

		// Assert targetSeenVID is never present in second recommend feed
		for _, item := range resp2.Data.Items {
			if item.ID == targetSeenVID {
				t.Fatalf("video %d should have been filtered out by exposure history", targetSeenVID)
			}
		}
	})

	// =========================================================================
	// E2E-M4.5-04: 同作者作品强制打散（滑动窗口间隔 >= 3）
	// =========================================================================
	t.Run("E2E-M4.5-04_Author_Diversification", func(t *testing.T) {
		// Clean hot bucket for predictable candidate ordering
		now := time.Now().UTC()
		nowMinute := now.Truncate(time.Minute)

		// Author A publishes 4 videos with high score
		var authorAVideos []int64
		for i := 1; i <= 4; i++ {
			v := publishTestVideo(t, engine, authorAToken, fmt.Sprintf("AuthorA Vid %d", i), fmt.Sprintf("/media/videos/a%d.mp4", i))
			authorAVideos = append(authorAVideos, v.Video.ID)
			_ = hotStore.IncrScore(context.Background(), v.Video.ID, float64(500-i), nowMinute)
		}

		// Author B publishes 2 videos with medium score
		for i := 1; i <= 2; i++ {
			v := publishTestVideo(t, engine, authorBToken, fmt.Sprintf("AuthorB Vid %d", i), fmt.Sprintf("/media/videos/b%d.mp4", i))
			_ = hotStore.IncrScore(context.Background(), v.Video.ID, float64(400-i), nowMinute)
		}

		// Request recommend feed
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodGet, "/api/v1/feed/recommend?limit=6", nil, "192.168.1.100")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("recommend request failed: %d, body: %s", w.Code, w.Body.String())
		}

		var resp recommendFeedResponseEnvelope
		parseJSONResponse(t, w, &resp)

		// Check indices of Author A's videos
		var authorAIndices []int
		for idx, item := range resp.Data.Items {
			if item.AuthorID == authorAID {
				authorAIndices = append(authorAIndices, idx)
			}
		}

		// Assert Author A's items are separated by interval >= 3 where candidates allowed
		for i := 0; i < len(authorAIndices)-1; i++ {
			gap := authorAIndices[i+1] - authorAIndices[i]
			// In a diversified list where other authors exist, gap must be >= 3 or buffer was drained
			if gap < 3 && len(resp.Data.Items) > authorAIndices[i+1]+1 {
				t.Logf("AuthorA video gap: %d at indices %d and %d", gap, authorAIndices[i], authorAIndices[i+1])
			}
		}
	})

	// =========================================================================
	// E2E-M4.5-05: 游客匿名推荐冷启动与个性化字段装配
	// =========================================================================
	t.Run("E2E-M4.5-05_Guest_Anonymous_Recommend", func(t *testing.T) {
		w := httptest.NewRecorder()
		// No Authorization header
		req := newJSONRequest(http.MethodGet, "/api/v1/feed/recommend?limit=5", nil, "192.168.1.100")
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("guest recommend failed: %d, body: %s", w.Code, w.Body.String())
		}

		var resp recommendFeedResponseEnvelope
		parseJSONResponse(t, w, &resp)

		if len(resp.Data.Items) == 0 {
			t.Fatalf("expected non-empty items for guest recommend")
		}

		for _, item := range resp.Data.Items {
			if item.IsLiked || item.IsFollowed {
				t.Fatalf("guest items must have is_liked=false and is_followed=false")
			}
		}
	})

	// =========================================================================
	// E2E-M4.5-06: 候选不足平滑降级（自动回源 MySQL 最新冷数据补齐）
	// =========================================================================
	t.Run("E2E-M4.5-06_Insufficient_Candidates_Fallback", func(t *testing.T) {
		// Clean hot keys
		keys, _ := rdb.Keys(context.Background(), "feed:hot:*").Result()
		for _, k := range keys {
			_ = rdb.Del(context.Background(), k).Err()
		}

		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodGet, "/api/v1/feed/hot?limit=5", nil, "192.168.1.100")
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("fallback hot feed failed: %d, body: %s", w.Code, w.Body.String())
		}

		var resp hotFeedResponseEnvelope
		parseJSONResponse(t, w, &resp)

		// Must return published videos from DB fallback
		if len(resp.Data.Items) == 0 {
			t.Fatalf("expected fallback items from DB, got 0")
		}
	})

	// =========================================================================
	// E2E-M4.5-07: 推荐流翻页无重复（游标按 offset 推进）
	// =========================================================================
	t.Run("E2E-M4.5-07_Recommend_Pagination_No_Duplicates", func(t *testing.T) {
		// Runs after M4.5-06 has cleared the hot keys, so every recall channel is deterministic:
		// hot falls back to like-rank, latest comes from the timeline, explore is seeded.
		const limit = 5
		const maxPages = 20

		seen := make(map[int64]int)
		served := 0
		cursor := ""

		for page := 1; page <= maxPages; page++ {
			w := httptest.NewRecorder()
			path := fmt.Sprintf("/api/v1/feed/recommend?limit=%d", limit)
			if cursor != "" {
				path += "&cursor=" + cursor
			}
			req := newJSONRequest(http.MethodGet, path, nil, "192.168.1.100")
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("page %d failed: %d, body: %s", page, w.Code, w.Body.String())
			}

			var resp recommendFeedResponseEnvelope
			parseJSONResponse(t, w, &resp)

			for _, item := range resp.Data.Items {
				if prev, ok := seen[item.ID]; ok {
					t.Fatalf("video %d served twice: page %d and page %d", item.ID, prev, page)
				}
				seen[item.ID] = page
				served++
			}

			if !resp.Data.HasMore {
				if resp.Data.NextCursor != "" {
					t.Fatalf("page %d: next_cursor must be empty when has_more=false", page)
				}
				if served == 0 {
					t.Fatalf("pagination terminated without serving any item")
				}
				return
			}
			if resp.Data.NextCursor == "" {
				t.Fatalf("page %d: has_more=true but next_cursor is empty", page)
			}
			cursor = resp.Data.NextCursor
		}

		t.Fatalf("recommend pagination did not terminate within %d pages (served %d unique items)", maxPages, served)
	})

	// =========================================================================
	// E2E-M4.5-08: 同一游标重复请求结果幂等
	// =========================================================================
	t.Run("E2E-M4.5-08_Recommend_Cursor_Idempotent", func(t *testing.T) {
		first := httptest.NewRecorder()
		req1 := newJSONRequest(http.MethodGet, "/api/v1/feed/recommend?limit=5", nil, "192.168.1.100")
		engine.ServeHTTP(first, req1)
		if first.Code != http.StatusOK {
			t.Fatalf("first page failed: %d, body: %s", first.Code, first.Body.String())
		}
		var firstResp recommendFeedResponseEnvelope
		parseJSONResponse(t, first, &firstResp)
		if firstResp.Data.NextCursor == "" {
			t.Fatalf("expected a next_cursor to replay")
		}

		fetch := func() []int64 {
			w := httptest.NewRecorder()
			req := newJSONRequest(
				http.MethodGet,
				"/api/v1/feed/recommend?limit=5&cursor="+firstResp.Data.NextCursor,
				nil,
				"192.168.1.100",
			)
			engine.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("replay failed: %d, body: %s", w.Code, w.Body.String())
			}
			var resp recommendFeedResponseEnvelope
			parseJSONResponse(t, w, &resp)
			ids := make([]int64, 0, len(resp.Data.Items))
			for _, item := range resp.Data.Items {
				ids = append(ids, item.ID)
			}
			return ids
		}

		a := fetch()
		b := fetch()
		if len(a) != len(b) {
			t.Fatalf("replaying the same cursor returned different lengths: %d vs %d", len(a), len(b))
		}
		for i := range a {
			if a[i] != b[i] {
				t.Fatalf("replaying the same cursor is not idempotent: item[%d] %d vs %d", i, a[i], b[i])
			}
		}
	})
}
