package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	infraoutbox "github.com/Tidehyc/gorangefeed/internal/infra/outbox"
	persistenceoutbox "github.com/Tidehyc/gorangefeed/internal/infra/persistence/outbox"
	"github.com/google/uuid"
)

type failPublisher struct {
	err error
}

func (f *failPublisher) Publish(ctx context.Context, event *domainoutbox.Event) error {
	return f.err
}

// TestCheckpointM4_WorkerLeaseReclaim_And_DeadLetter validates Checkpoint CP-M4:
// 1. Worker crash simulation: lease expires after 30s and is automatically reclaimed back to 'pending'.
// 2. Dead-letter queue simulation: 10 consecutive delivery failures transitions status to 'dead'.
func TestCheckpointM4_WorkerLeaseReclaim_And_DeadLetter(t *testing.T) {
	ctx, cleanup := setupE2ETestExtended(t)
	defer cleanup()

	ctx.DB.Exec("DELETE FROM outbox")
	now := time.Now()

	t.Run("WorkerCrash_And_LeaseReclaim", func(t *testing.T) {
		eventID := uuid.New().String()
		initialEvent := &persistenceoutbox.OutboxModel{
			EventID:     eventID,
			EventType:   "video.published",
			Payload:     []byte(`{"video_id":9001,"author_id":8001}`),
			Status:      string(domainoutbox.StatusPending),
			Attempts:    0,
			NextRetryAt: now.Add(-1 * time.Minute),
			CreatedAt:   now,
		}
		if err := ctx.DB.Create(initialEvent).Error; err != nil {
			t.Fatalf("failed to insert initial outbox event: %v", err)
		}

		// Step 1: Worker Alpha claims the lease for 30s
		workerAlphaToken := uuid.New().String()
		claimed, err := ctx.OutboxRepo.ClaimLease(context.Background(), workerAlphaToken, 30*time.Second, 10)
		if err != nil {
			t.Fatalf("worker alpha failed to claim lease: %v", err)
		}

		var claimedThis bool
		for _, e := range claimed {
			if e.EventID == eventID {
				claimedThis = true
				if e.Status != domainoutbox.StatusSending {
					t.Fatalf("expected status 'sending', got '%s'", e.Status)
				}
				if e.LeaseToken == nil || *e.LeaseToken != workerAlphaToken {
					t.Fatalf("expected lease token '%s', got '%v'", workerAlphaToken, e.LeaseToken)
				}
			}
		}
		if !claimedThis {
			t.Fatalf("worker alpha did not claim the target event")
		}

		// Step 2: Worker Beta tries to claim, but cannot claim this active event
		workerBetaToken := uuid.New().String()
		claimedBeta, err := ctx.OutboxRepo.ClaimLease(context.Background(), workerBetaToken, 30*time.Second, 10)
		if err != nil {
			t.Fatalf("worker beta claim lease errored: %v", err)
		}
		for _, e := range claimedBeta {
			if e.EventID == eventID {
				t.Fatalf("worker beta should NOT claim event %s under active lease", eventID)
			}
		}

		// Step 3: Simulate Worker Alpha crash by winding lease_until back past expiry
		if err := ctx.DB.Model(&persistenceoutbox.OutboxModel{}).
			Where("event_id = ?", eventID).
			Update("lease_until", now.Add(-10*time.Second)).Error; err != nil {
			t.Fatalf("failed to simulate lease expiry in DB: %v", err)
		}

		// Step 4: Run lease reclamation
		scheduler, err := infraoutbox.NewScheduler(ctx.OutboxRepo, &failPublisher{err: errors.New("dummy")})
		if err != nil {
			t.Fatalf("failed to create scheduler: %v", err)
		}

		reclaimedCount, err := scheduler.ReclaimExpired(context.Background())
		if err != nil {
			t.Fatalf("failed to reclaim expired leases: %v", err)
		}
		if reclaimedCount == 0 {
			t.Fatalf("expected at least 1 reclaimed lease, got 0")
		}

		// Verify event in DB is now 'pending' with nil lease_token
		var updated persistenceoutbox.OutboxModel
		if err := ctx.DB.Where("event_id = ?", eventID).First(&updated).Error; err != nil {
			t.Fatalf("failed to fetch reclaimed outbox model: %v", err)
		}
		if updated.Status != string(domainoutbox.StatusPending) {
			t.Fatalf("expected status 'pending' after reclaim, got '%s'", updated.Status)
		}
		if updated.LeaseToken != nil {
			t.Fatalf("expected nil lease_token after reclaim, got '%s'", *updated.LeaseToken)
		}

		// Step 5: Worker Beta can now successfully claim the reclaimed event
		claimedBeta2, err := ctx.OutboxRepo.ClaimLease(context.Background(), workerBetaToken, 30*time.Second, 10)
		if err != nil {
			t.Fatalf("worker beta second claim failed: %v", err)
		}
		var reclaimedByBeta bool
		for _, e := range claimedBeta2 {
			if e.EventID == eventID {
				reclaimedByBeta = true
				if e.LeaseToken == nil || *e.LeaseToken != workerBetaToken {
					t.Fatalf("expected event claimed by beta '%s', got '%v'", workerBetaToken, e.LeaseToken)
				}
			}
		}
		if !reclaimedByBeta {
			t.Fatalf("expected event %s to be reclaimed by worker beta", eventID)
		}
	})

	t.Run("ContinuousFailures_TransitionToDead", func(t *testing.T) {
		ctx.DB.Exec("DELETE FROM outbox")
		eventID := uuid.New().String()
		initialEvent := &persistenceoutbox.OutboxModel{
			EventID:     eventID,
			EventType:   "interaction.liked",
			Payload:     []byte(`{"video_id":9002,"user_id":8002}`),
			Status:      string(domainoutbox.StatusPending),
			Attempts:    0,
			NextRetryAt: now.Add(-1 * time.Minute),
			CreatedAt:   now,
		}
		if err := ctx.DB.Create(initialEvent).Error; err != nil {
			t.Fatalf("failed to insert outbox event: %v", err)
		}

		mockPub := &failPublisher{err: errors.New("simulated broker unreachable")}
		scheduler, err := infraoutbox.NewScheduler(
			ctx.OutboxRepo,
			mockPub,
			infraoutbox.WithMaxAttempts(10),
			infraoutbox.WithBatchSize(10),
			infraoutbox.WithLeaseDuration(10*time.Second),
		)
		if err != nil {
			t.Fatalf("failed to initialize scheduler: %v", err)
		}

		// Simulate 10 failure cycles
		for cycle := 1; cycle <= 10; cycle++ {
			// Fast-forward next_retry_at to past so it can be claimed immediately
			if err := ctx.DB.Model(&persistenceoutbox.OutboxModel{}).
				Where("event_id = ?", eventID).
				Updates(map[string]any{
					"next_retry_at": time.Now().Add(-1 * time.Second),
					"status":        string(domainoutbox.StatusPending),
				}).Error; err != nil {
				t.Fatalf("failed to fast forward retry: %v", err)
			}

			dispatched, err := scheduler.PollAndDispatch(context.Background())
			if err != nil {
				t.Fatalf("cycle %d poll and dispatch failed: %v", cycle, err)
			}
			if dispatched == 0 {
				t.Fatalf("cycle %d expected to dispatch event, but got 0", cycle)
			}
		}

		// Final check: after 10 failures, status must be 'dead' and attempts == 10
		var finalModel persistenceoutbox.OutboxModel
		if err := ctx.DB.Where("event_id = ?", eventID).First(&finalModel).Error; err != nil {
			t.Fatalf("failed to find final outbox record: %v", err)
		}

		if finalModel.Status != string(domainoutbox.StatusDead) {
			t.Fatalf("expected status 'dead', got '%s'", finalModel.Status)
		}
		if finalModel.Attempts != 10 {
			t.Fatalf("expected attempts 10, got %d", finalModel.Attempts)
		}
		if finalModel.LastError == nil || *finalModel.LastError == "" {
			t.Fatalf("expected last_error to be recorded, but got nil or empty")
		}
	})
}

// TestCheckpointM4_ConcurrentLikes_Consistency validates Checkpoint CP-M4:
// Concurrently fire 300 like / unlike operations across different users against a single video.
// Asserts that after all transactions complete:
// 1. SELECT COUNT(*) FROM likes == video_stats.like_count (absolute zero-drift consistency);
// 2. like_count >= 0 (no negative underflow on rogue unlikes);
// 3. Exactly the net positive likes remain in DB and stats.
func TestCheckpointM4_ConcurrentLikes_Consistency(t *testing.T) {
	ctx, cleanup := setupE2ETestExtended(t)
	defer cleanup()

	// 1. Prepare target video with 0 likes
	videoID := int64(88801)
	authorID := int64(999901)
	now := time.Now()

	ctx.DB.Exec("DELETE FROM likes WHERE video_id = ?", videoID)
	ctx.DB.Exec("DELETE FROM video_stats WHERE video_id = ?", videoID)
	ctx.DB.Exec("DELETE FROM videos WHERE id = ?", videoID)

	ctx.DB.Exec("INSERT INTO videos (id, author_id, title, play_url, cover_url, status, published_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)",
		videoID, authorID, "Concurrent Likes Test Video", "/media/videos/test.mp4", "/media/covers/test.jpg", now, now, now)
	ctx.DB.Exec("INSERT INTO video_stats (video_id, like_count, comment_count, view_count, created_at, updated_at) VALUES (?, 0, 0, 0, ?, ?)",
		videoID, now, now)

	// Total users configured:
	// 180 users will like only. (IDs: 8100001 ~ 8100180) -> net +180
	// 120 users will like then unlike. (IDs: 8100181 ~ 8100300) -> net 0
	// 50 users will only unlike without liking first. (IDs: 8100301 ~ 8100350) -> net 0 (test underflow guard)
	// Expected final like_count = 180, and DB likes count = 180.

	var wg sync.WaitGroup
	startCh := make(chan struct{})

	// Helper to send HTTP requests with JWT
	sendRequest := func(method, path string, userID int64) int {
		token, _, err := ctx.JWTMgr.GenerateAccessToken(userID, uuid.New().String())
		if err != nil {
			t.Errorf("failed to generate access token for user %d: %v", userID, err)
			return 0
		}
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		ctx.Engine.ServeHTTP(w, req)
		return w.Code
	}

	// Group 1: 180 users like only
	for i := 1; i <= 180; i++ {
		userID := int64(8100000 + i)
		wg.Add(1)
		go func(uid int64) {
			defer wg.Done()
			<-startCh
			code := sendRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/like", videoID), uid)
			if code != http.StatusOK {
				t.Errorf("user %d like failed with status %d", uid, code)
			}
		}(userID)
	}

	// Group 2: 120 users like then unlike
	for i := 181; i <= 300; i++ {
		userID := int64(8100000 + i)
		wg.Add(1)
		go func(uid int64) {
			defer wg.Done()
			<-startCh
			codeLike := sendRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/like", videoID), uid)
			if codeLike != http.StatusOK {
				t.Errorf("user %d like failed with status %d", uid, codeLike)
			}
			codeUnlike := sendRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d/like", videoID), uid)
			if codeUnlike != http.StatusOK {
				t.Errorf("user %d unlike failed with status %d", uid, codeUnlike)
			}
		}(userID)
	}

	// Group 3: 50 rogue users unlike directly without prior liking
	for i := 301; i <= 350; i++ {
		userID := int64(8100000 + i)
		wg.Add(1)
		go func(uid int64) {
			defer wg.Done()
			<-startCh
			code := sendRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d/like", videoID), uid)
			if code != http.StatusOK {
				t.Errorf("user %d rogue unlike failed with status %d", uid, code)
			}
		}(userID)
	}

	// Trigger all concurrent goroutines simultaneously
	close(startCh)
	wg.Wait()

	// 2. Query DB and assert absolute consistency
	var dbLikesCount int64
	if err := ctx.DB.Table("likes").Where("video_id = ?", videoID).Count(&dbLikesCount).Error; err != nil {
		t.Fatalf("failed to count likes from DB: %v", err)
	}

	var statsLikeCount int64
	if err := ctx.DB.Table("video_stats").Where("video_id = ?", videoID).Pluck("like_count", &statsLikeCount).Error; err != nil {
		t.Fatalf("failed to query video_stats like_count: %v", err)
	}

	if dbLikesCount != 180 {
		t.Fatalf("expected 180 persistent like records in DB, got %d", dbLikesCount)
	}
	if statsLikeCount != 180 {
		t.Fatalf("expected 180 like_count in video_stats, got %d", statsLikeCount)
	}
	if dbLikesCount != statsLikeCount {
		t.Fatalf("CONCURRENCY DRIFT DETECTED: dbLikesCount (%d) != statsLikeCount (%d)", dbLikesCount, statsLikeCount)
	}
}

// TestCheckpointM4_ConcurrentComment_Idempotency validates Checkpoint CP-M4:
// 5 concurrent submissions with the same idempotency_key by the same user within 50ms.
// Asserts:
// 1. All 5 requests succeed (HTTP 200/201);
// 2. All 5 return identical comment ID and payload;
// 3. Exactly 1 comment record inserted into MySQL;
// 4. video_stats.comment_count is incremented by exactly 1.
func TestCheckpointM4_ConcurrentComment_Idempotency(t *testing.T) {
	ctx, cleanup := setupE2ETestExtended(t)
	defer cleanup()

	videoID := int64(88802)
	authorID := int64(999902)
	userID := int64(8200001)
	now := time.Now()

	ctx.DB.Exec("DELETE FROM comments WHERE video_id = ?", videoID)
	ctx.DB.Exec("DELETE FROM video_stats WHERE video_id = ?", videoID)
	ctx.DB.Exec("DELETE FROM videos WHERE id = ?", videoID)

	ctx.DB.Exec("INSERT INTO videos (id, author_id, title, play_url, cover_url, status, published_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)",
		videoID, authorID, "Concurrent Comment Idempotency Test Video", "/media/videos/test.mp4", "/media/covers/test.jpg", now, now, now)
	ctx.DB.Exec("INSERT INTO video_stats (video_id, like_count, comment_count, view_count, created_at, updated_at) VALUES (?, 0, 0, 0, ?, ?)",
		videoID, now, now)

	idempotencyKey := "cp_m4_idemp_" + uuid.New().String()
	token, _, err := ctx.JWTMgr.GenerateAccessToken(userID, uuid.New().String())
	if err != nil {
		t.Fatalf("failed to generate access token: %v", err)
	}

	const concurrency = 5
	var wg sync.WaitGroup
	startCh := make(chan struct{})

	type result struct {
		statusCode int
		commentID  int64
		content    string
	}

	results := make([]result, concurrency)

	for i := 0; i < concurrency; i++ {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startCh

			reqBody := map[string]interface{}{
				"content":         "Idempotent comment under high concurrency!",
				"idempotency_key": idempotencyKey,
			}
			bodyBytes, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/comments", videoID), bytes.NewReader(bodyBytes))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			ctx.Engine.ServeHTTP(w, req)

			var resp struct {
				Code int `json:"code"`
				Data struct {
					Comment struct {
						Comment struct {
							ID      int64  `json:"id"`
							Content string `json:"content"`
						} `json:"comment"`
					} `json:"comment"`
				} `json:"data"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &resp)

			results[idx] = result{
				statusCode: w.Code,
				commentID:  resp.Data.Comment.Comment.ID,
				content:    resp.Data.Comment.Comment.Content,
			}
		}()
	}

	// Trigger all 5 submissions concurrently
	close(startCh)
	wg.Wait()

	// 1. Assert HTTP level responses
	var expectedCommentID int64
	for i, res := range results {
		if res.statusCode != http.StatusOK && res.statusCode != http.StatusCreated {
			t.Fatalf("request %d failed with status %d", i, res.statusCode)
		}
		if res.commentID <= 0 {
			t.Fatalf("request %d returned non-positive comment ID %d", i, res.commentID)
		}
		if expectedCommentID == 0 {
			expectedCommentID = res.commentID
		} else if res.commentID != expectedCommentID {
			t.Fatalf("request %d returned comment ID %d, expected %d", i, res.commentID, expectedCommentID)
		}
	}

	// 2. Assert DB level records: exactly 1 comment in comments table
	var commentCountInDB int64
	if err := ctx.DB.Table("comments").
		Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).
		Count(&commentCountInDB).Error; err != nil {
		t.Fatalf("failed to count comments in DB: %v", err)
	}
	if commentCountInDB != 1 {
		t.Fatalf("expected exactly 1 comment record in DB, got %d", commentCountInDB)
	}

	// 3. Assert DB level stats: video_stats.comment_count incremented by exactly 1
	var statsCommentCount int64
	if err := ctx.DB.Table("video_stats").
		Where("video_id = ?", videoID).
		Pluck("comment_count", &statsCommentCount).Error; err != nil {
		t.Fatalf("failed to query video_stats comment_count: %v", err)
	}
	if statsCommentCount != 1 {
		t.Fatalf("expected video_stats.comment_count == 1, got %d", statsCommentCount)
	}
}
