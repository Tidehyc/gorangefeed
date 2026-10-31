package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpinteraction "github.com/Tidehyc/gorangefeed/internal/http/interaction"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	persistencefeed "github.com/Tidehyc/gorangefeed/internal/infra/persistence/feed"
	persistencerelation "github.com/Tidehyc/gorangefeed/internal/infra/persistence/relation"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
)

type commentEnvelope struct {
	Data struct {
		Comment *domaininteraction.CommentDetail `json:"comment"`
	} `json:"data"`
}

type commentListEnvelope struct {
	Data struct {
		Items      []*domaininteraction.CommentDetail `json:"items"`
		NextCursor string                             `json:"next_cursor"`
		HasMore    bool                               `json:"has_more"`
	} `json:"data"`
}

type relationEnvelope struct {
	Data *domainrelation.Friendship `json:"data"`
}

type myLikedVideosEnvelope struct {
	Data struct {
		Items      []*domainvideo.VideoDetail `json:"items"`
		NextCursor int64                      `json:"next_cursor"`
		HasMore    bool                       `json:"has_more"`
	} `json:"data"`
}

type likeRankEnvelope struct {
	Data struct {
		Items      []*domainfeed.FeedItem `json:"items"`
		NextCursor string                 `json:"next_cursor"`
		HasMore    bool                   `json:"has_more"`
	} `json:"data"`
}

func TestM4_InteractionLifecycle_EndToEnd(t *testing.T) {
	ctx, cleanup := setupE2ETestExtended(t)
	if cleanup != nil {
		defer cleanup()
	}

	engine := ctx.Engine
	db := ctx.DB
	rdb := ctx.RDB

	suffix := time.Now().Format("0405_000")
	userA := fmt.Sprintf("m4_userA_%s", suffix)
	userB := fmt.Sprintf("m4_userB_%s", suffix)
	authorNormal := fmt.Sprintf("m4_authorP_%s", suffix)
	authorBigV := fmt.Sprintf("m4_authorV_%s", suffix)
	password := "SecurePass123"

	// =========================================================================
	// Phase 1: User Registration & Authentication
	// =========================================================================
	uidA, tokenA := registerAndLogin(t, engine, userA, password)
	uidB, tokenB := registerAndLogin(t, engine, userB, password)
	uidP, tokenP := registerAndLogin(t, engine, authorNormal, password)
	uidV, tokenV := registerAndLogin(t, engine, authorBigV, password)

	if uidA <= 0 || uidB <= 0 || uidP <= 0 || uidV <= 0 {
		t.Fatalf("failed to register users: uidA=%d, uidB=%d, uidP=%d, uidV=%d", uidA, uidB, uidP, uidV)
	}

	// =========================================================================
	// Phase 2: Video Publication
	// Normal Author P publishes V1 & V2
	// Big V Author V publishes V3 & V4
	// =========================================================================
	v1 := publishTestVideo(t, engine, tokenP, "Normal Video 1", "/media/videos/p1.mp4")
	v2 := publishTestVideo(t, engine, tokenP, "Normal Video 2", "/media/videos/p2.mp4")
	v3 := publishTestVideo(t, engine, tokenV, "BigV Video 1", "/media/videos/v1.mp4")
	v4 := publishTestVideo(t, engine, tokenV, "BigV Video 2", "/media/videos/v2.mp4")

	v1ID := v1.Video.ID
	v2ID := v2.Video.ID
	v3ID := v3.Video.ID
	v4ID := v4.Video.ID

	// =========================================================================
	// Phase 3: Video Like Concurrency, Idempotency & Count Atomicity (E2E-M4-01, E2E-M4-02)
	// =========================================================================
	// 1. User A likes Video 1 (First time) -> 200 OK
	{
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/like", v1ID), nil)
		req.Header.Set("Authorization", "Bearer "+tokenA)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("like video failed: code=%d, body=%s", w.Code, w.Body.String())
		}

		// 2. User A repeats like on Video 1 twice (Idempotency) -> both return 200 OK
		for i := 0; i < 2; i++ {
			reqRepeat, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/like", v1ID), nil)
			reqRepeat.Header.Set("Authorization", "Bearer "+tokenA)
			wRepeat := httptest.NewRecorder()
			engine.ServeHTTP(wRepeat, reqRepeat)
			if wRepeat.Code != http.StatusOK {
				t.Fatalf("repeat like video failed: code=%d, body=%s", wRepeat.Code, wRepeat.Body.String())
			}
		}

		// Verify DB consistency: exactly 1 like record, like_count == 1
		var likeCountInDB int64
		db.Table("likes").Where("user_id = ? AND video_id = ?", uidA, v1ID).Count(&likeCountInDB)
		if likeCountInDB != 1 {
			t.Fatalf("expected 1 like record in DB, got %d", likeCountInDB)
		}

		var statLikeCount int64
		db.Table("video_stats").Where("video_id = ?", v1ID).Select("like_count").Scan(&statLikeCount)
		if statLikeCount != 1 {
			t.Fatalf("expected video_stats.like_count == 1, got %d", statLikeCount)
		}

		// User B also likes Video 1 -> like_count == 2
		reqB, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/like", v1ID), nil)
		reqB.Header.Set("Authorization", "Bearer "+tokenB)
		wB := httptest.NewRecorder()
		engine.ServeHTTP(wB, reqB)
		if wB.Code != http.StatusOK {
			t.Fatalf("user B like video failed: code=%d", wB.Code)
		}

		db.Table("video_stats").Where("video_id = ?", v1ID).Select("like_count").Scan(&statLikeCount)
		if statLikeCount != 2 {
			t.Fatalf("expected video_stats.like_count == 2 after User B like, got %d", statLikeCount)
		}

		// User B unlikes Video 1 -> like_count == 1
		reqUnB, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d/like", v1ID), nil)
		reqUnB.Header.Set("Authorization", "Bearer "+tokenB)
		wUnB := httptest.NewRecorder()
		engine.ServeHTTP(wUnB, reqUnB)
		if wUnB.Code != http.StatusOK {
			t.Fatalf("user B unlike video failed: code=%d", wUnB.Code)
		}

		db.Table("video_stats").Where("video_id = ?", v1ID).Select("like_count").Scan(&statLikeCount)
		if statLikeCount != 1 {
			t.Fatalf("expected video_stats.like_count == 1 after User B unlike, got %d", statLikeCount)
		}

		// User B unlikes again (Idempotent, non-negative) -> 200 OK, like_count remains 1
		wUnB2 := httptest.NewRecorder()
		engine.ServeHTTP(wUnB2, reqUnB)
		if wUnB2.Code != http.StatusOK {
			t.Fatalf("user B repeat unlike failed: code=%d", wUnB2.Code)
		}

		db.Table("video_stats").Where("video_id = ?", v1ID).Select("like_count").Scan(&statLikeCount)
		if statLikeCount != 1 {
			t.Fatalf("expected video_stats.like_count == 1, got %d", statLikeCount)
		}

		// User A unlikes Video 1 -> like_count == 0
		reqUnA, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d/like", v1ID), nil)
		reqUnA.Header.Set("Authorization", "Bearer "+tokenA)
		wUnA := httptest.NewRecorder()
		engine.ServeHTTP(wUnA, reqUnA)
		if wUnA.Code != http.StatusOK {
			t.Fatalf("user A unlike video failed: code=%d", wUnA.Code)
		}

		db.Table("video_stats").Where("video_id = ?", v1ID).Select("like_count").Scan(&statLikeCount)
		if statLikeCount != 0 {
			t.Fatalf("expected video_stats.like_count == 0, got %d", statLikeCount)
		}

		// User A unlikes again -> like_count must NEVER become negative
		wUnA2 := httptest.NewRecorder()
		engine.ServeHTTP(wUnA2, reqUnA)
		if wUnA2.Code != http.StatusOK {
			t.Fatalf("user A repeat unlike failed: code=%d", wUnA2.Code)
		}

		db.Table("video_stats").Where("video_id = ?", v1ID).Select("like_count").Scan(&statLikeCount)
		if statLikeCount < 0 {
			t.Fatalf("video_stats.like_count became negative: %d", statLikeCount)
		}

		// Re-like Video 1 so User A has liked V1
		engine.ServeHTTP(httptest.NewRecorder(), req)
	}

	// User A also likes Video 2 and Video 3
	{
		reqLike2, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/like", v2ID), nil)
		reqLike2.Header.Set("Authorization", "Bearer "+tokenA)
		engine.ServeHTTP(httptest.NewRecorder(), reqLike2)

		reqLike3, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/like", v3ID), nil)
		reqLike3.Header.Set("Authorization", "Bearer "+tokenA)
		engine.ServeHTTP(httptest.NewRecorder(), reqLike3)
	}

	// =========================================================================
	// Phase 4: My Liked Videos Stream & Monotonic ID Cursor (E2E-M4-03)
	// User A liked V1, V2, V3 (in that order). Reverse order is V3, V2, V1.
	// =========================================================================
	{
		// Page 1: limit = 2
		reqMyLikes1, _ := http.NewRequest(http.MethodGet, "/api/v1/users/me/likes?cursor=0&limit=2", nil)
		reqMyLikes1.Header.Set("Authorization", "Bearer "+tokenA)
		wMyLikes1 := httptest.NewRecorder()
		engine.ServeHTTP(wMyLikes1, reqMyLikes1)
		if wMyLikes1.Code != http.StatusOK {
			t.Fatalf("get my likes page 1 failed: code=%d, body=%s", wMyLikes1.Code, wMyLikes1.Body.String())
		}

		var myLikesResp1 myLikedVideosEnvelope
		parseJSONResponse(t, wMyLikes1, &myLikesResp1)
		if len(myLikesResp1.Data.Items) != 2 {
			t.Fatalf("expected 2 liked videos on page 1, got %d", len(myLikesResp1.Data.Items))
		}
		if myLikesResp1.Data.Items[0].Video.ID != v3ID || myLikesResp1.Data.Items[1].Video.ID != v2ID {
			t.Errorf("expected [V3, V2], got [%d, %d]", myLikesResp1.Data.Items[0].Video.ID, myLikesResp1.Data.Items[1].Video.ID)
		}
		if !myLikesResp1.Data.HasMore || myLikesResp1.Data.NextCursor <= 0 {
			t.Fatalf("expected has_more=true and positive next_cursor")
		}

		// Page 2: limit = 2 with next_cursor
		page2URL := fmt.Sprintf("/api/v1/users/me/likes?cursor=%d&limit=2", myLikesResp1.Data.NextCursor)
		reqMyLikes2, _ := http.NewRequest(http.MethodGet, page2URL, nil)
		reqMyLikes2.Header.Set("Authorization", "Bearer "+tokenA)
		wMyLikes2 := httptest.NewRecorder()
		engine.ServeHTTP(wMyLikes2, reqMyLikes2)
		if wMyLikes2.Code != http.StatusOK {
			t.Fatalf("get my likes page 2 failed: code=%d, body=%s", wMyLikes2.Code, wMyLikes2.Body.String())
		}

		var myLikesResp2 myLikedVideosEnvelope
		parseJSONResponse(t, wMyLikes2, &myLikesResp2)
		if len(myLikesResp2.Data.Items) != 1 {
			t.Fatalf("expected 1 liked video on page 2, got %d", len(myLikesResp2.Data.Items))
		}
		if myLikesResp2.Data.Items[0].Video.ID != v1ID {
			t.Errorf("expected [V1], got [%d]", myLikesResp2.Data.Items[0].Video.ID)
		}
		if myLikesResp2.Data.HasMore {
			t.Errorf("expected has_more=false on last page")
		}
	}

	// =========================================================================
	// Phase 5: Comment Publication Idempotency & Replies (E2E-M4-04)
	// =========================================================================
	var c1ID, c2ID int64
	{
		// 1. Root comment with Header Idempotency-Key
		cReq := httpinteraction.CreateCommentRequest{
			Content:  "Root comment by User A",
			ParentID: 0,
		}
		reqC1 := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/comments", v1ID), cReq, "192.168.1.1")
		reqC1.Header.Set("Authorization", "Bearer "+tokenA)
		reqC1.Header.Set("Idempotency-Key", fmt.Sprintf("idem_root_%s", suffix))
		wC1 := httptest.NewRecorder()
		engine.ServeHTTP(wC1, reqC1)
		if wC1.Code != http.StatusCreated {
			t.Fatalf("create root comment failed: code=%d, body=%s", wC1.Code, wC1.Body.String())
		}

		var c1Resp commentEnvelope
		parseJSONResponse(t, wC1, &c1Resp)
		if c1Resp.Data.Comment == nil || c1Resp.Data.Comment.Comment == nil {
			t.Fatalf("expected comment data in response")
		}
		c1ID = c1Resp.Data.Comment.Comment.ID

		// Verify comment_count in DB
		var commentCount int64
		db.Table("video_stats").Where("video_id = ?", v1ID).Select("comment_count").Scan(&commentCount)
		if commentCount != 1 {
			t.Fatalf("expected comment_count == 1, got %d", commentCount)
		}

		// 2. Concurrent/Replayed Comment with same idempotency key -> Idempotent replay
		wC1Repeat := httptest.NewRecorder()
		reqC1Repeat := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/comments", v1ID), cReq, "192.168.1.1")
		reqC1Repeat.Header.Set("Authorization", "Bearer "+tokenA)
		reqC1Repeat.Header.Set("Idempotency-Key", fmt.Sprintf("idem_root_%s", suffix))
		engine.ServeHTTP(wC1Repeat, reqC1Repeat)
		if wC1Repeat.Code != http.StatusOK && wC1Repeat.Code != http.StatusCreated {
			t.Fatalf("repeat comment failed: code=%d", wC1Repeat.Code)
		}

		var c1RepeatResp commentEnvelope
		parseJSONResponse(t, wC1Repeat, &c1RepeatResp)
		if c1RepeatResp.Data.Comment.Comment.ID != c1ID {
			t.Fatalf("expected replayed comment ID %d, got %d", c1ID, c1RepeatResp.Data.Comment.Comment.ID)
		}
		db.Table("video_stats").Where("video_id = ?", v1ID).Select("comment_count").Scan(&commentCount)
		if commentCount != 1 {
			t.Fatalf("comment_count incremented on idempotent retry: %d", commentCount)
		}

		// 3. User B replies to Comment 1
		c2Req := httpinteraction.CreateCommentRequest{
			Content:        "Reply by User B",
			ParentID:       c1ID,
			IdempotencyKey: fmt.Sprintf("idem_reply_%s", suffix),
		}
		reqC2 := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/comments", v1ID), c2Req, "192.168.1.2")
		reqC2.Header.Set("Authorization", "Bearer "+tokenB)
		wC2 := httptest.NewRecorder()
		engine.ServeHTTP(wC2, reqC2)
		if wC2.Code != http.StatusCreated {
			t.Fatalf("create reply comment failed: code=%d, body=%s", wC2.Code, wC2.Body.String())
		}

		var c2Resp commentEnvelope
		parseJSONResponse(t, wC2, &c2Resp)
		c2ID = c2Resp.Data.Comment.Comment.ID

		db.Table("video_stats").Where("video_id = ?", v1ID).Select("comment_count").Scan(&commentCount)
		if commentCount != 2 {
			t.Fatalf("expected comment_count == 2 after reply, got %d", commentCount)
		}

		// 4. User A likes Comment 2
		reqLikeC2, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/comments/%d/like", c2ID), nil)
		reqLikeC2.Header.Set("Authorization", "Bearer "+tokenA)
		wLikeC2 := httptest.NewRecorder()
		engine.ServeHTTP(wLikeC2, reqLikeC2)
		if wLikeC2.Code != http.StatusOK {
			t.Fatalf("like comment failed: code=%d", wLikeC2.Code)
		}

		// 5. Query comments with sort=hot: C2 (1 like) must precede C1 (0 likes)
		reqHot, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d/comments?sort=hot", v1ID), nil)
		wHot := httptest.NewRecorder()
		engine.ServeHTTP(wHot, reqHot)
		if wHot.Code != http.StatusOK {
			t.Fatalf("get hot comments failed: code=%d", wHot.Code)
		}

		var hotResp commentListEnvelope
		parseJSONResponse(t, wHot, &hotResp)
		if len(hotResp.Data.Items) < 2 {
			t.Fatalf("expected at least 2 comments, got %d", len(hotResp.Data.Items))
		}
		if hotResp.Data.Items[0].Comment.ID != c2ID {
			t.Errorf("expected C2 (with 1 like) to be first in hot comments, got ID=%d", hotResp.Data.Items[0].Comment.ID)
		}
	}

	// =========================================================================
	// Phase 6: User Relations, Friendship & Self-Follow Prevention (E2E-M4-05)
	// =========================================================================
	{
		// 1. Self-follow prevention
		reqSelf, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/follow", uidA), nil)
		reqSelf.Header.Set("Authorization", "Bearer "+tokenA)
		wSelf := httptest.NewRecorder()
		engine.ServeHTTP(wSelf, reqSelf)
		if wSelf.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for self-follow, got %d", wSelf.Code)
		}

		// 2. User A follows User B
		reqFollowB, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/follow", uidB), nil)
		reqFollowB.Header.Set("Authorization", "Bearer "+tokenA)
		wFollowB := httptest.NewRecorder()
		engine.ServeHTTP(wFollowB, reqFollowB)
		if wFollowB.Code != http.StatusOK {
			t.Fatalf("follow user B failed: code=%d", wFollowB.Code)
		}

		// Check relation: A follows B, but B does not follow A -> is_friend=false
		reqRelA, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/users/%d/relation", uidB), nil)
		reqRelA.Header.Set("Authorization", "Bearer "+tokenA)
		wRelA := httptest.NewRecorder()
		engine.ServeHTTP(wRelA, reqRelA)
		if wRelA.Code != http.StatusOK {
			t.Fatalf("get relation failed: code=%d", wRelA.Code)
		}

		var relRespA relationEnvelope
		parseJSONResponse(t, wRelA, &relRespA)
		if !relRespA.Data.IsFollowing || relRespA.Data.IsFollower || relRespA.Data.IsFriend {
			t.Fatalf("expected following=true, follower=false, friend=false; got %+v", relRespA.Data)
		}

		// 3. User B follows User A -> Mutual friendship!
		reqFollowA, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/follow", uidA), nil)
		reqFollowA.Header.Set("Authorization", "Bearer "+tokenB)
		wFollowA := httptest.NewRecorder()
		engine.ServeHTTP(wFollowA, reqFollowA)
		if wFollowA.Code != http.StatusOK {
			t.Fatalf("user B follow user A failed: code=%d", wFollowA.Code)
		}

		// Check mutual friendship: is_friend=true
		wRelMutual := httptest.NewRecorder()
		engine.ServeHTTP(wRelMutual, reqRelA)
		var relRespMutual relationEnvelope
		parseJSONResponse(t, wRelMutual, &relRespMutual)
		if !relRespMutual.Data.IsFriend {
			t.Fatalf("expected mutual friend is_friend=true, got %+v", relRespMutual.Data)
		}

		// User A also follows Normal Author P and Big V Author V
		reqFollowP, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/follow", uidP), nil)
		reqFollowP.Header.Set("Authorization", "Bearer "+tokenA)
		engine.ServeHTTP(httptest.NewRecorder(), reqFollowP)

		reqFollowV, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/follow", uidV), nil)
		reqFollowV.Header.Set("Authorization", "Bearer "+tokenA)
		engine.ServeHTTP(httptest.NewRecorder(), reqFollowV)
	}

	// =========================================================================
	// Phase 7: Following Feed Hybrid Push-Pull & Multi-Way Merge (E2E-M4-06)
	// User A follows Author P (normal, push) and Author V (big V, pull).
	// =========================================================================
	{
		// Set Author V as Big V (follower_count = 6000 >= 5000)
		db.Exec("UPDATE relation_stats SET follower_count = 6000 WHERE user_id = ?", uidV)

		// Create FanoutService to simulate event consumption
		relRepo := persistencerelation.NewRepository(db)
		fStore := persistencefeed.NewFollowingStore(rdb, db)
		dedup := rabbitmq.NewDeduplicator(db)
		fanout, err := servicefeed.NewFanoutService(relRepo, fStore, dedup)
		if err != nil {
			t.Fatalf("failed to create fanout service: %v", err)
		}

		// Dispatch video.published for Author P (Normal -> pushes to User A's inbox)
		pPayload, _ := json.Marshal(servicefeed.VideoPublishedPayload{VideoID: v2ID, AuthorID: uidP})
		if err := fanout.HandleVideoPublished(context.Background(), fmt.Sprintf("evt-p-%d", v2ID), pPayload); err != nil {
			t.Fatalf("fanout for normal author failed: %v", err)
		}

		// Dispatch video.published for Author V (Big V -> pushes to Author V's outbox)
		vPayload, _ := json.Marshal(servicefeed.VideoPublishedPayload{VideoID: v4ID, AuthorID: uidV})
		if err := fanout.HandleVideoPublished(context.Background(), fmt.Sprintf("evt-v-%d", v4ID), vPayload); err != nil {
			t.Fatalf("fanout for big V author failed: %v", err)
		}

		// User A requests Following Feed
		reqFollowFeed, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/following?limit=10", nil)
		reqFollowFeed.Header.Set("Authorization", "Bearer "+tokenA)
		wFollowFeed := httptest.NewRecorder()
		engine.ServeHTTP(wFollowFeed, reqFollowFeed)
		if wFollowFeed.Code != http.StatusOK {
			t.Fatalf("get following feed failed: code=%d, body=%s", wFollowFeed.Code, wFollowFeed.Body.String())
		}

		var fFeedResp feedResponseEnvelope
		parseJSONResponse(t, wFollowFeed, &fFeedResp)
		if len(fFeedResp.Data.Items) == 0 {
			t.Fatalf("expected following feed items, got 0")
		}

		// Verify ordering: multi-way merge ensures monotonically decreasing video ID
		for i := 0; i < len(fFeedResp.Data.Items)-1; i++ {
			if fFeedResp.Data.Items[i].ID <= fFeedResp.Data.Items[i+1].ID {
				t.Errorf("following feed not sorted in ID DESC order: %d <= %d", fFeedResp.Data.Items[i].ID, fFeedResp.Data.Items[i+1].ID)
			}
		}

		// Verify dynamic hydration: all following feed items have IsFollowed == true
		for _, item := range fFeedResp.Data.Items {
			if !item.IsFollowed {
				t.Errorf("expected IsFollowed=true in following feed for item %d", item.ID)
			}
		}
	}

	// =========================================================================
	// Phase 8: Public L1 Page Skeleton Sharing & Dynamic Hydration (E2E-M4-08)
	// =========================================================================
	{
		// 1. Anonymous visitor requests global feed
		reqAnon, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/global?limit=5", nil)
		wAnon := httptest.NewRecorder()
		engine.ServeHTTP(wAnon, reqAnon)
		if wAnon.Code != http.StatusOK {
			t.Fatalf("anonymous global feed failed: code=%d", wAnon.Code)
		}

		var anonResp feedResponseEnvelope
		parseJSONResponse(t, wAnon, &anonResp)
		for _, item := range anonResp.Data.Items {
			if item.IsLiked || item.IsFollowed {
				t.Errorf("anonymous visitor should have IsLiked=false, IsFollowed=false; got %+v", item)
			}
		}

		// Verify Redis L1 key was created with public skeleton
		l1Key := "feed:page:global:public:0:5:v1"
		exists, err := rdb.Exists(context.Background(), l1Key).Result()
		if err != nil || exists == 0 {
			t.Fatalf("expected public L1 cache key %s to exist in Redis, exists=%d, err=%v", l1Key, exists, err)
		}

		// 2. User A requests global feed -> hits exact same public L1 key, but dynamically hydrated!
		reqAuth, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/global?limit=5", nil)
		reqAuth.Header.Set("Authorization", "Bearer "+tokenA)
		wAuth := httptest.NewRecorder()
		engine.ServeHTTP(wAuth, reqAuth)
		if wAuth.Code != http.StatusOK {
			t.Fatalf("auth global feed failed: code=%d", wAuth.Code)
		}

		var authResp feedResponseEnvelope
		parseJSONResponse(t, wAuth, &authResp)
		// User A liked V1, V2, V3 -> their IsLiked should be true!
		for _, item := range authResp.Data.Items {
			if item.ID == v1ID || item.ID == v2ID || item.ID == v3ID {
				if !item.IsLiked {
					t.Errorf("expected item %d to have IsLiked=true for User A", item.ID)
				}
			}
			// User A follows Author P and Author V -> items by them should have IsFollowed=true
			if item.AuthorID == uidP || item.AuthorID == uidV {
				if !item.IsFollowed {
					t.Errorf("expected item %d (author=%d) to have IsFollowed=true for User A", item.ID, item.AuthorID)
				}
			}
		}
	}

	// =========================================================================
	// Phase 9: Like Rank Feed Composite Cursor Pagination
	// =========================================================================
	{
		reqLikeRank, _ := http.NewRequest(http.MethodGet, "/api/v1/feed/like-rank?limit=2", nil)
		wLikeRank := httptest.NewRecorder()
		engine.ServeHTTP(wLikeRank, reqLikeRank)
		if wLikeRank.Code != http.StatusOK {
			t.Fatalf("like rank feed failed: code=%d, body=%s", wLikeRank.Code, wLikeRank.Body.String())
		}

		var rankResp likeRankEnvelope
		parseJSONResponse(t, wLikeRank, &rankResp)
		if len(rankResp.Data.Items) == 0 {
			t.Fatalf("expected like rank items, got 0")
		}
		if rankResp.Data.HasMore && rankResp.Data.NextCursor == "" {
			t.Fatalf("expected non-empty composite next_cursor when has_more=true")
		}

		// Query second page using composite cursor
		if rankResp.Data.HasMore {
			page2URL := fmt.Sprintf("/api/v1/feed/like-rank?cursor=%s&limit=2", rankResp.Data.NextCursor)
			reqPage2, _ := http.NewRequest(http.MethodGet, page2URL, nil)
			wPage2 := httptest.NewRecorder()
			engine.ServeHTTP(wPage2, reqPage2)
			if wPage2.Code != http.StatusOK {
				t.Fatalf("like rank page 2 failed: code=%d", wPage2.Code)
			}
		}
	}

	_ = c1ID
	_ = c2ID
}
