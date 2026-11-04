package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	httpasset "github.com/Tidehyc/gorangefeed/internal/http/asset"
	persistenceoutbox "github.com/Tidehyc/gorangefeed/internal/infra/persistence/outbox"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	"github.com/redis/go-redis/v9"
)

type favoriteActionEnvelope struct {
	Code    int                            `json:"code"`
	Message string                         `json:"message"`
	Data    httpasset.FavoriteVideoResponse `json:"data"`
}

type favoriteFoldersEnvelope struct {
	Code    int                           `json:"code"`
	Message string                        `json:"message"`
	Data    httpasset.ListFoldersResponse `json:"data"`
}

type createFolderEnvelope struct {
	Code    int                         `json:"code"`
	Message string                      `json:"message"`
	Data    *domainasset.FavoriteFolder `json:"data"`
}

type folderItemsEnvelope struct {
	Code    int                               `json:"code"`
	Message string                            `json:"message"`
	Data    *httpasset.ListFolderItemsResponse `json:"data"`
}

type watchHistoryEnvelope struct {
	Code    int                      `json:"code"`
	Message string                   `json:"message"`
	Data    *domainasset.WatchHistory `json:"data"`
}

type listHistoryEnvelope struct {
	Code    int                           `json:"code"`
	Message string                        `json:"message"`
	Data    *httpasset.ListHistoryResponse `json:"data"`
}

func TestM8_AssetLifecycle_EndToEnd(t *testing.T) {
	ctxExt, cleanup := setupE2ETestExtended(t)
	if cleanup != nil {
		defer cleanup()
	}

	engine := ctxExt.Engine
	db := ctxExt.DB
	rdb := ctxExt.RDB
	hotConsumer := ctxExt.HotConsumer

	suffix := time.Now().Format("0405_000")
	userAName := fmt.Sprintf("asset_user_a_%s", suffix)
	userBName := fmt.Sprintf("asset_user_b_%s", suffix)
	testPass := "SecurePass123"

	// Register test users
	userAID, userAToken := registerAndLogin(t, engine, userAName, testPass)
	userBID, userBToken := registerAndLogin(t, engine, userBName, testPass)
	if userAID <= 0 || userBID <= 0 {
		t.Fatalf("failed to register users: userA=%d, userB=%d", userAID, userBID)
	}

	// Creator (userB) publishes two test videos
	v1 := publishTestVideo(t, engine, userBToken, "M8 Asset Video 1", "/media/videos/m8_v1.mp4")
	v2 := publishTestVideo(t, engine, userBToken, "M8 Asset Video 2", "/media/videos/m8_v2.mp4")
	if v1 == nil || v1.Video == nil || v2 == nil || v2.Video == nil {
		t.Fatalf("failed to publish test videos")
	}

	// =========================================================================
	// E2E-M8-01: 默认收藏夹自动初始化与自定义私有/公开收藏夹分类及权限隔离
	// =========================================================================
	var privateFolderID int64
	var publicFolderID int64

	t.Run("E2E-M8-01_Default_Folder_Auto_Init_And_Folder_Permissions", func(t *testing.T) {
		// 1.1 First time query of userA folders: should automatically initialize default folder
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodGet, "/api/v1/user/favorites", nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("list folders failed: code=%d, body=%s", w.Code, w.Body.String())
		}

		var listResp favoriteFoldersEnvelope
		parseJSONResponse(t, w, &listResp)
		if len(listResp.Data.Folders) != 1 {
			t.Fatalf("expected 1 default folder initialized, got %d", len(listResp.Data.Folders))
		}
		defaultFolder := listResp.Data.Folders[0]
		if !defaultFolder.IsDefault {
			t.Errorf("expected is_default=true for initial folder")
		}
		if defaultFolder.Name != "默认收藏夹" {
			t.Errorf("expected default folder name '默认收藏夹', got %q", defaultFolder.Name)
		}
		if defaultFolder.ItemCount != 0 {
			t.Errorf("expected initial item_count 0, got %d", defaultFolder.ItemCount)
		}

		// 1.2 UserA creates a custom PRIVATE folder
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodPost, "/api/v1/user/favorites", httpasset.CreateFolderRequest{
			Name:        "私密珍藏",
			Description: "仅自己可见的私密收藏",
			IsPublic:    new(bool), // false -> private
		}, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("create private folder failed: code=%d, body=%s", w.Code, w.Body.String())
		}
		var privResp createFolderEnvelope
		parseJSONResponse(t, w, &privResp)
		if privResp.Data == nil || privResp.Data.ID <= 0 {
			t.Fatalf("invalid created private folder response: %+v", privResp)
		}
		if privResp.Data.IsPublic {
			t.Errorf("expected is_public=false for custom private folder")
		}
		privateFolderID = privResp.Data.ID

		// 1.3 UserA adds video v1 to this custom private folder
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/favorite?folder_id=%d", v1.Video.ID, privateFolderID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("favorite video into private folder failed: code=%d, body=%s", w.Code, w.Body.String())
		}

		// 1.4 Security & Access Control: UserB (third-party) attempts to access UserA's private folder
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/user/favorites/%d/items", privateFolderID), nil, "192.168.1.102")
		req.Header.Set("Authorization", "Bearer "+userBToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden when third-party accesses private folder, got code=%d, body=%s", w.Code, w.Body.String())
		}

		// Anonymous (unauthenticated) attempt should also be 403 Forbidden
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/user/favorites/%d/items", privateFolderID), nil, "192.168.1.103")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for anonymous viewer accessing private folder, got %d", w.Code)
		}

		// UserA (owner) successfully accesses the private folder items
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/user/favorites/%d/items", privateFolderID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("owner failed to access private folder items: code=%d, body=%s", w.Code, w.Body.String())
		}
		var ownerItemsResp folderItemsEnvelope
		parseJSONResponse(t, w, &ownerItemsResp)
		if len(ownerItemsResp.Data.Items) != 1 || ownerItemsResp.Data.Items[0].VideoID != v1.Video.ID {
			t.Fatalf("expected 1 item with videoID=%d in private folder, got %+v", v1.Video.ID, ownerItemsResp.Data)
		}

		// 1.5 UserA creates a custom PUBLIC folder and adds video v2
		isPub := true
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodPost, "/api/v1/user/favorites", httpasset.CreateFolderRequest{
			Name:        "公开分享精选",
			Description: "全员可见的公开收藏",
			IsPublic:    &isPub,
		}, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("create public folder failed: code=%d, body=%s", w.Code, w.Body.String())
		}
		var pubResp createFolderEnvelope
		parseJSONResponse(t, w, &pubResp)
		publicFolderID = pubResp.Data.ID

		// Add video v2 to public folder
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/favorite?folder_id=%d", v2.Video.ID, publicFolderID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("favorite video into public folder failed: code=%d", w.Code)
		}

		// UserB (third-party) can view public folder items without restriction
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/user/favorites/%d/items", publicFolderID), nil, "192.168.1.102")
		req.Header.Set("Authorization", "Bearer "+userBToken)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("third-party failed to view public folder items: code=%d, body=%s", w.Code, w.Body.String())
		}
		var thirdPartyItemsResp folderItemsEnvelope
		parseJSONResponse(t, w, &thirdPartyItemsResp)
		if len(thirdPartyItemsResp.Data.Items) != 1 || thirdPartyItemsResp.Data.Items[0].VideoID != v2.Video.ID {
			t.Fatalf("expected 1 item with videoID=%d in public folder, got %+v", v2.Video.ID, thirdPartyItemsResp.Data)
		}
	})

	// =========================================================================
	// E2E-M8-02: 视频收藏与取消收藏原子流转及计数与幂等性保障
	// =========================================================================
	t.Run("E2E-M8-02_Favorite_Lifecycle_Atomic_Counts_And_Idempotency", func(t *testing.T) {
		// 2.1 UserA favorites video v1 into default folder (no folder_id specified)
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/favorite", v1.Video.ID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("favorite into default folder failed: code=%d, body=%s", w.Code, w.Body.String())
		}
		var favResp favoriteActionEnvelope
		parseJSONResponse(t, w, &favResp)
		if !favResp.Data.IsNew {
			t.Errorf("expected is_new=true for first favorite")
		}

		// Verify default folder item_count incremented to 1
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, "/api/v1/user/favorites", nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		var listResp favoriteFoldersEnvelope
		parseJSONResponse(t, w, &listResp)

		var defFolder *domainasset.FavoriteFolder
		for _, f := range listResp.Data.Folders {
			if f.IsDefault {
				defFolder = f
				break
			}
		}
		if defFolder == nil || defFolder.ItemCount != 1 {
			t.Fatalf("expected default folder item_count=1, got %+v", defFolder)
		}

		// Verify video_stats.favorite_count incremented
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d", v1.Video.ID), nil, "192.168.1.101")
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("get video detail failed: code=%d", w.Code)
		}
		var vidResp videoDetailEnvelope
		parseJSONResponse(t, w, &vidResp)
		// v1 was favorited in private folder earlier and now default folder -> favorite_count should be 2
		if vidResp.Data.Stats.FavoriteCount != 2 {
			t.Fatalf("expected video favorite_count=2, got %d", vidResp.Data.Stats.FavoriteCount)
		}

		// 2.2 Idempotency: Favorite same video into default folder again
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/favorite", v1.Video.ID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("repeat favorite failed: code=%d", w.Code)
		}
		parseJSONResponse(t, w, &favResp)
		if favResp.Data.IsNew {
			t.Errorf("expected is_new=false for duplicate favorite")
		}

		// Ensure default folder item_count remains 1
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, "/api/v1/user/favorites", nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		parseJSONResponse(t, w, &listResp)
		for _, f := range listResp.Data.Folders {
			if f.IsDefault && f.ItemCount != 1 {
				t.Fatalf("expected default folder item_count still 1, got %d", f.ItemCount)
			}
		}

		// 2.3 Unfavorite video v1 from default folder
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d/favorite", v1.Video.ID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("unfavorite failed: code=%d, body=%s", w.Code, w.Body.String())
		}

		// Check default folder item_count decremented to 0
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, "/api/v1/user/favorites", nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		parseJSONResponse(t, w, &listResp)
		for _, f := range listResp.Data.Folders {
			if f.IsDefault && f.ItemCount != 0 {
				t.Fatalf("expected default folder item_count 0, got %d", f.ItemCount)
			}
		}

		// Check video_stats.favorite_count decremented from 2 to 1
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d", v1.Video.ID), nil, "192.168.1.101")
		engine.ServeHTTP(w, req)
		parseJSONResponse(t, w, &vidResp)
		if vidResp.Data.Stats.FavoriteCount != 1 {
			t.Fatalf("expected video favorite_count decremented to 1, got %d", vidResp.Data.Stats.FavoriteCount)
		}

		// 2.4 Negative boundary guard: Repeat unfavorite should be safe and not decrement below 0
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d/favorite", v1.Video.ID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("repeat unfavorite failed: code=%d", w.Code)
		}

		// Verify default folder item_count remains 0
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, "/api/v1/user/favorites", nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		parseJSONResponse(t, w, &listResp)
		for _, f := range listResp.Data.Folders {
			if f.IsDefault && f.ItemCount != 0 {
				t.Fatalf("expected default folder item_count remains 0, got %d", f.ItemCount)
			}
		}
	})

	// =========================================================================
	// E2E-M8-03: 收藏与取消收藏事件 Outbox 投递与热榜 3 倍加权消费
	// =========================================================================
	t.Run("E2E-M8-03_Outbox_Event_Delivery_And_Hot_Score_Consumer_Weighting", func(t *testing.T) {
		now := time.Now().UTC()
		minuteBucketKey := fmt.Sprintf("feed:hot:minute:%s", now.Truncate(time.Minute).Format("200601021504"))

		// 3.1 UserA favorites video v2 into default folder
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/favorite", v2.Video.ID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("favorite video v2 failed: code=%d, body=%s", w.Code, w.Body.String())
		}

		// Query Outbox table for the created favorite event
		var outboxEvent persistenceoutbox.OutboxModel
		err := db.Table("outbox").
			Where("event_type = ?", domainoutbox.EventTypeInteractionFavorited).
			Order("id DESC").
			First(&outboxEvent).Error
		if err != nil {
			t.Fatalf("failed to find outbox event for favorited: %v", err)
		}

		// Initial minute bucket score for v2
		initialScore, _ := rdb.ZScore(context.Background(), minuteBucketKey, fmt.Sprintf("%d", v2.Video.ID)).Result()

		// 3.2 HotScoreConsumer handles the favorited event
		err = hotConsumer.HandleEngagementEvent(
			context.Background(),
			outboxEvent.EventID,
			rabbitmq.RoutingKeyInteractionFavorited,
			outboxEvent.Payload,
		)
		if err != nil {
			t.Fatalf("HotScoreConsumer handle favorited event failed: %v", err)
		}

		// Assert hot score increased by +9.0 (ScoreFavoriteDelta = 9.0, exactly 3x LikeDelta 3.0)
		scoreAfterFav, err := rdb.ZScore(context.Background(), minuteBucketKey, fmt.Sprintf("%d", v2.Video.ID)).Result()
		if err != nil {
			t.Fatalf("failed to get minute score after favorite: %v", err)
		}
		expectedFavScore := initialScore + domainfeed.ScoreFavoriteDelta
		if scoreAfterFav != expectedFavScore {
			t.Fatalf("expected minute hot score %f after favorite (+9.0), got %f", expectedFavScore, scoreAfterFav)
		}

		// Idempotent consumption check: second handle of same event should not change score
		err = hotConsumer.HandleEngagementEvent(
			context.Background(),
			outboxEvent.EventID,
			rabbitmq.RoutingKeyInteractionFavorited,
			outboxEvent.Payload,
		)
		if err != nil {
			t.Fatalf("HotScoreConsumer repeat handle failed: %v", err)
		}
		scoreRepeat, _ := rdb.ZScore(context.Background(), minuteBucketKey, fmt.Sprintf("%d", v2.Video.ID)).Result()
		if scoreRepeat != scoreAfterFav {
			t.Fatalf("hot score should be unchanged on repeat consume, expected %f, got %f", scoreAfterFav, scoreRepeat)
		}

		// 3.3 UserA unfavorites video v2
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d/favorite", v2.Video.ID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("unfavorite v2 failed: code=%d", w.Code)
		}

		// Query Outbox table for the created unfavorite event
		var unfavEvent persistenceoutbox.OutboxModel
		err = db.Table("outbox").
			Where("event_type = ?", domainoutbox.EventTypeInteractionUnfavorited).
			Order("id DESC").
			First(&unfavEvent).Error
		if err != nil {
			t.Fatalf("failed to find outbox event for unfavorited: %v", err)
		}

		// HotScoreConsumer handles unfavorite event
		err = hotConsumer.HandleEngagementEvent(
			context.Background(),
			unfavEvent.EventID,
			rabbitmq.RoutingKeyInteractionUnfavorited,
			unfavEvent.Payload,
		)
		if err != nil {
			t.Fatalf("HotScoreConsumer handle unfavorited event failed: %v", err)
		}

		// Assert hot score decremented by -9.0 (ScoreUnfavoriteDelta = -9.0)
		scoreAfterUnfav, err := rdb.ZScore(context.Background(), minuteBucketKey, fmt.Sprintf("%d", v2.Video.ID)).Result()
		if err != nil {
			t.Fatalf("failed to get minute score after unfavorite: %v", err)
		}
		if scoreAfterUnfav != initialScore {
			t.Fatalf("expected minute hot score to return to %f after unfavorite (-9.0), got %f", initialScore, scoreAfterUnfav)
		}
	})

	// =========================================================================
	// E2E-M8-04: 播放历史滑动窗口保留策略（100条）与原子淘汰
	// =========================================================================
	t.Run("E2E-M8-04_Playback_History_Sliding_Window_And_Auto_Trimming", func(t *testing.T) {
		// Report 105 distinct video watch histories sequentially (videoID 2001 ~ 2105)
		startVideoID := int64(2001)
		totalRecords := 105

		for i := 0; i < totalRecords; i++ {
			vid := startVideoID + int64(i)
			w := httptest.NewRecorder()
			req := newJSONRequest(http.MethodPost, "/api/v1/user/history", httpasset.RecordProgressRequest{
				VideoID:        vid,
				LastPositionMs: (i + 1) * 1000,
				DurationMs:     120000,
			}, "192.168.1.101")
			req.Header.Set("Authorization", "Bearer "+userAToken)
			engine.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("record history for video %d failed: code=%d, body=%s", vid, w.Code, w.Body.String())
			}
		}

		zsetKey := fmt.Sprintf("asset:history:zset:%d", userAID)
		hashKey := fmt.Sprintf("asset:history:detail:%d", userAID)

		// Assert Redis ZSet cardinality is strictly capped at MaxHistoryCapacity (100)
		zcard, err := rdb.ZCard(context.Background(), zsetKey).Result()
		if err != nil {
			t.Fatalf("failed to query zcard: %v", err)
		}
		if zcard != 100 {
			t.Fatalf("expected ZSet cardinality 100 after 105 reports, got %d", zcard)
		}

		// Assert Redis Hash size is also strictly trimmed to 100
		hlen, err := rdb.HLen(context.Background(), hashKey).Result()
		if err != nil {
			t.Fatalf("failed to query hlen: %v", err)
		}
		if hlen != 100 {
			t.Fatalf("expected Hash length 100, got %d", hlen)
		}

		// Assert oldest 5 items (2001 ~ 2005) have been completely evicted
		for i := 0; i < 5; i++ {
			evictedID := fmt.Sprintf("%d", startVideoID+int64(i))
			_, err := rdb.ZScore(context.Background(), zsetKey, evictedID).Result()
			if err != redis.Nil {
				t.Fatalf("expected video %s to be evicted from ZSet, but got err: %v", evictedID, err)
			}
			hexists, _ := rdb.HExists(context.Background(), hashKey, evictedID).Result()
			if hexists {
				t.Fatalf("expected video %s to be evicted from Hash details", evictedID)
			}
		}

		// Assert latest video 2105 exists and is at the top of history
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodGet, "/api/v1/user/history?limit=10", nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("list history failed: code=%d, body=%s", w.Code, w.Body.String())
		}
		var historyResp listHistoryEnvelope
		parseJSONResponse(t, w, &historyResp)
		if len(historyResp.Data.Items) != 10 {
			t.Fatalf("expected 10 history items, got %d", len(historyResp.Data.Items))
		}
		if historyResp.Data.Items[0].VideoID != 2105 {
			t.Fatalf("expected newest video 2105 at top, got %d", historyResp.Data.Items[0].VideoID)
		}

		// Clear watch history
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodDelete, "/api/v1/user/history", nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("clear history failed: code=%d", w.Code)
		}

		// Verify ZSet & Hash cleared
		zcardAfter, _ := rdb.ZCard(context.Background(), zsetKey).Result()
		hlenAfter, _ := rdb.HLen(context.Background(), hashKey).Result()
		if zcardAfter != 0 || hlenAfter != 0 {
			t.Fatalf("expected history storage cleared, got zcard=%d, hlen=%d", zcardAfter, hlenAfter)
		}
	})

	// =========================================================================
	// E2E-M8-05: 跨设备播放断点续播上报与精准恢复
	// =========================================================================
	t.Run("E2E-M8-05_Cross_Device_Breakpoint_Progress_Restore", func(t *testing.T) {
		// Device 1: user reports progress 23500ms on video v2
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/history", v2.Video.ID), httpasset.RecordProgressRequest{
			LastPositionMs: 23500,
			DurationMs:     60000,
		}, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("device 1 report progress failed: code=%d, body=%s", w.Code, w.Body.String())
		}

		// Device 2: user switches device and fetches resume progress for video v2
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d/history", v2.Video.ID), nil, "10.0.0.88")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("device 2 fetch resume progress failed: code=%d, body=%s", w.Code, w.Body.String())
		}
		var progResp watchHistoryEnvelope
		parseJSONResponse(t, w, &progResp)
		if progResp.Data.LastPositionMs != 23500 {
			t.Fatalf("expected resumed position 23500ms, got %d", progResp.Data.LastPositionMs)
		}
		if progResp.Data.DurationMs != 60000 {
			t.Fatalf("expected duration 60000ms, got %d", progResp.Data.DurationMs)
		}

		// Device 2 continues playing and updates progress to 45200ms
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/history", v2.Video.ID), httpasset.RecordProgressRequest{
			LastPositionMs: 45200,
			DurationMs:     60000,
		}, "10.0.0.88")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("device 2 update progress failed: code=%d", w.Code)
		}

		// Query again: assert position updated to 45200ms
		w = httptest.NewRecorder()
		req = newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d/history", v2.Video.ID), nil, "192.168.1.101")
		req.Header.Set("Authorization", "Bearer "+userAToken)
		engine.ServeHTTP(w, req)
		parseJSONResponse(t, w, &progResp)
		if progResp.Data.LastPositionMs != 45200 {
			t.Fatalf("expected updated position 45200ms, got %d", progResp.Data.LastPositionMs)
		}
	})
}
