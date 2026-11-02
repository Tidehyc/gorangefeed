package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpcreator "github.com/Tidehyc/gorangefeed/internal/http/creator"
	infrascheduler "github.com/Tidehyc/gorangefeed/internal/infra/scheduler"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type creatorDraftEnvelope struct {
	Code    int                        `json:"code"`
	Message string                     `json:"message"`
	Data    *httpcreator.DraftResponse `json:"data"`
}

type creatorDraftListEnvelope struct {
	Code    int                             `json:"code"`
	Message string                          `json:"message"`
	Data    *httpcreator.ListDraftsResponse `json:"data"`
}

type creatorScheduledTaskEnvelope struct {
	Code    int                                `json:"code"`
	Message string                             `json:"message"`
	Data    *httpcreator.ScheduledTaskResponse `json:"data"`
}

type creatorScheduleResponseEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID      string `json:"task_id"`
		ScheduledAt string `json:"scheduled_at"`
		Status      string `json:"status"`
	} `json:"data"`
}

type creatorScheduledTaskListEnvelope struct {
	Code    int                                     `json:"code"`
	Message string                                  `json:"message"`
	Data    *httpcreator.ListScheduledTasksResponse `json:"data"`
}

type creatorPublishResponseEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		VideoID    int64  `json:"video_id"`
		PublishSeq int64  `json:"publish_seq"`
		Title      string `json:"title"`
		Status     string `json:"status"`
	} `json:"data"`
}

func saveTestDraft(t *testing.T, engine *gin.Engine, token, title, playURL string) *httpcreator.DraftResponse {
	t.Helper()
	w := httptest.NewRecorder()
	req := newJSONRequest(http.MethodPost, "/api/v1/creator/drafts", httpcreator.SaveDraftRequestPayload{
		Title:       title,
		Description: "Draft description for " + title,
		PlayURL:     playURL,
		CoverURL:    "https://example.com/cover.jpg",
		DurationMs:  12000,
	}, "192.168.10.50")
	req.Header.Set("Authorization", "Bearer "+token)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to save draft %q: code=%d, body=%s", title, w.Code, w.Body.String())
	}

	var resp creatorDraftEnvelope
	parseJSONResponse(t, w, &resp)
	if resp.Data == nil {
		t.Fatalf("expected draft data, got nil")
	}
	return resp.Data
}

func TestM7_CreatorLifecycle_EndToEnd(t *testing.T) {
	ctx, cleanup := setupE2ETestExtended(t)
	if cleanup != nil {
		defer cleanup()
	}

	suffix := time.Now().Format("0405_000")
	userAName := fmt.Sprintf("creator_user_a_%s", suffix)
	userBName := fmt.Sprintf("creator_user_b_%s", suffix)

	userAID, tokenA := registerAndLogin(t, ctx.Engine, userAName, "Passwd123!")
	userBID, tokenB := registerAndLogin(t, ctx.Engine, userBName, "Passwd123!")
	_ = userBID

	var draft1ID int64

	// E2E-M7-01: Draft CRUD and Cursor Pagination
	t.Run("E2E-M7-01_Draft_CRUD_And_Cursor_Pagination", func(t *testing.T) {
		// 1. Create draft 1
		d1 := saveTestDraft(t, ctx.Engine, tokenA, "Draft One", "/media/videos/draft1.mp4")
		if d1.ID <= 0 {
			t.Fatalf("expected positive draft ID, got %d", d1.ID)
		}
		if d1.AuthorID != userAID {
			t.Fatalf("expected author ID %d, got %d", userAID, d1.AuthorID)
		}
		if d1.Status != "draft" {
			t.Fatalf("expected status 'draft', got %s", d1.Status)
		}
		draft1ID = d1.ID

		// 2. Get draft 1 as author
		wGet := httptest.NewRecorder()
		reqGet := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/creator/drafts/%d", draft1ID), nil, "192.168.10.50")
		reqGet.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wGet, reqGet)
		if wGet.Code != http.StatusOK {
			t.Fatalf("expected 200 for GetDraft, got %d: %s", wGet.Code, wGet.Body.String())
		}
		var getResp creatorDraftEnvelope
		parseJSONResponse(t, wGet, &getResp)
		if getResp.Data == nil || getResp.Data.Title != "Draft One" {
			t.Fatalf("expected title 'Draft One', got %+v", getResp.Data)
		}

		// 3. User B attempts to get User A's draft (data isolation check)
		wGetB := httptest.NewRecorder()
		reqGetB := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/creator/drafts/%d", draft1ID), nil, "192.168.10.51")
		reqGetB.Header.Set("Authorization", "Bearer "+tokenB)
		ctx.Engine.ServeHTTP(wGetB, reqGetB)
		if wGetB.Code != http.StatusForbidden {
			t.Fatalf("expected 403 data isolation for other user, got %d: %s", wGetB.Code, wGetB.Body.String())
		}

		// 4. Update draft 1
		wPut := httptest.NewRecorder()
		reqPut := newJSONRequest(http.MethodPut, fmt.Sprintf("/api/v1/creator/drafts/%d", draft1ID), httpcreator.UpdateDraftRequestPayload{
			Title:       "Draft One Updated",
			Description: "Updated description",
			PlayURL:     "/media/videos/draft1_v2.mp4",
			DurationMs:  15000,
		}, "192.168.10.50")
		reqPut.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wPut, reqPut)
		if wPut.Code != http.StatusOK {
			t.Fatalf("expected 200 for UpdateDraft, got %d: %s", wPut.Code, wPut.Body.String())
		}

		// Verify update in get
		wGetAfterPut := httptest.NewRecorder()
		reqGetAfterPut := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/creator/drafts/%d", draft1ID), nil, "192.168.10.50")
		reqGetAfterPut.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wGetAfterPut, reqGetAfterPut)
		var getAfterPutResp creatorDraftEnvelope
		parseJSONResponse(t, wGetAfterPut, &getAfterPutResp)
		if getAfterPutResp.Data.Title != "Draft One Updated" || getAfterPutResp.Data.PlayURL != "/media/videos/draft1_v2.mp4" {
			t.Fatalf("draft not updated as expected: %+v", getAfterPutResp.Data)
		}

		// 5. Create Draft 2 and Draft 3 for pagination test
		d2 := saveTestDraft(t, ctx.Engine, tokenA, "Draft Two", "/media/videos/draft2.mp4")
		d3 := saveTestDraft(t, ctx.Engine, tokenA, "Draft Three", "/media/videos/draft3.mp4")
		_ = d2
		_ = d3

		// 6. List drafts with limit=2 (first page)
		wList1 := httptest.NewRecorder()
		reqList1 := newJSONRequest(http.MethodGet, "/api/v1/creator/drafts?limit=2", nil, "192.168.10.50")
		reqList1.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wList1, reqList1)
		if wList1.Code != http.StatusOK {
			t.Fatalf("expected 200 for ListDrafts, got %d: %s", wList1.Code, wList1.Body.String())
		}
		var list1Resp creatorDraftListEnvelope
		parseJSONResponse(t, wList1, &list1Resp)
		if len(list1Resp.Data.Items) != 2 {
			t.Fatalf("expected 2 items on page 1, got %d", len(list1Resp.Data.Items))
		}
		if !list1Resp.Data.HasMore || list1Resp.Data.NextCursor == "" {
			t.Fatalf("expected has_more=true and valid next_cursor, got has_more=%v, cursor=%s", list1Resp.Data.HasMore, list1Resp.Data.NextCursor)
		}

		// Page 2
		wList2 := httptest.NewRecorder()
		reqList2 := newJSONRequest(http.MethodGet, "/api/v1/creator/drafts?limit=2&cursor="+list1Resp.Data.NextCursor, nil, "192.168.10.50")
		reqList2.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wList2, reqList2)
		if wList2.Code != http.StatusOK {
			t.Fatalf("expected 200 for ListDrafts page 2, got %d: %s", wList2.Code, wList2.Body.String())
		}
		var list2Resp creatorDraftListEnvelope
		parseJSONResponse(t, wList2, &list2Resp)
		if len(list2Resp.Data.Items) < 1 {
			t.Fatalf("expected at least 1 item on page 2, got %d", len(list2Resp.Data.Items))
		}

		// 7. Delete Draft 1
		wDel := httptest.NewRecorder()
		reqDel := newJSONRequest(http.MethodDelete, fmt.Sprintf("/api/v1/creator/drafts/%d", draft1ID), nil, "192.168.10.50")
		reqDel.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wDel, reqDel)
		if wDel.Code != http.StatusOK {
			t.Fatalf("expected 200 for DeleteDraft, got %d: %s", wDel.Code, wDel.Body.String())
		}

		// Confirm 404 after delete
		wGetDel := httptest.NewRecorder()
		reqGetDel := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/creator/drafts/%d", draft1ID), nil, "192.168.10.50")
		reqGetDel.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wGetDel, reqGetDel)
		if wGetDel.Code != http.StatusNotFound {
			t.Fatalf("expected 404 after delete, got %d", wGetDel.Code)
		}
	})

	// E2E-M7-02: Draft OneClick Publish and Sequence
	t.Run("E2E-M7-02_Draft_OneClick_Publish_And_Sequence", func(t *testing.T) {
		dPub := saveTestDraft(t, ctx.Engine, tokenA, "Publishable Draft", "/media/videos/publishable.mp4")

		// 1. One-click publish
		wPub := httptest.NewRecorder()
		reqPub := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/creator/drafts/%d/publish", dPub.ID), nil, "192.168.10.50")
		reqPub.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wPub, reqPub)
		if wPub.Code != http.StatusOK {
			t.Fatalf("expected 200 for PublishDraft, got %d: %s", wPub.Code, wPub.Body.String())
		}

		var pubResp creatorPublishResponseEnvelope
		parseJSONResponse(t, wPub, &pubResp)
		if pubResp.Data.VideoID <= 0 {
			t.Fatalf("expected positive video_id, got %d", pubResp.Data.VideoID)
		}
		if pubResp.Data.PublishSeq <= 0 {
			t.Fatalf("expected positive publish_seq, got %d", pubResp.Data.PublishSeq)
		}
		if pubResp.Data.Status != "published" {
			t.Fatalf("expected status 'published', got %s", pubResp.Data.Status)
		}

		// 2. Verify video in DB
		v, err := ctx.VideoRepo.FindByID(context.Background(), pubResp.Data.VideoID)
		if err != nil || v == nil {
			t.Fatalf("failed to query published video in DB: %v", err)
		}
		if v.Status != domainvideo.VideoStatusPublished {
			t.Fatalf("expected video status published, got %d", v.Status)
		}
		if v.PublishSeq != pubResp.Data.PublishSeq {
			t.Fatalf("expected video publish_seq %d, got %d", pubResp.Data.PublishSeq, v.PublishSeq)
		}

		// 3. Verify draft status in DB
		dCheck, err := ctx.DraftRepo.GetByID(context.Background(), dPub.ID)
		if err != nil || dCheck == nil {
			t.Fatalf("failed to query draft: %v", err)
		}
		if dCheck.Status != domaincreator.DraftStatusPublished {
			t.Fatalf("expected draft status published (3), got %d", dCheck.Status)
		}

		// 4. Verify global timeline ZSet in Redis (score == publish_seq)
		score, err := ctx.RDB.ZScore(context.Background(), "timeline:global", fmt.Sprintf("%d", pubResp.Data.VideoID)).Result()
		if err != nil {
			t.Fatalf("expected video on timeline:global: %v", err)
		}
		if int64(score) != pubResp.Data.PublishSeq {
			t.Fatalf("expected timeline score to match publish_seq (%d), got %f", pubResp.Data.PublishSeq, score)
		}

		// 5. Repeat publish must be rejected with 400
		wRep := httptest.NewRecorder()
		reqRep := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/creator/drafts/%d/publish", dPub.ID), nil, "192.168.10.50")
		reqRep.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wRep, reqRep)
		if wRep.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for repeat publish on already published draft, got %d: %s", wRep.Code, wRep.Body.String())
		}
	})

	// E2E-M7-03: Scheduled Time Validation Rejection
	t.Run("E2E-M7-03_Scheduled_Time_Validation_Rejection", func(t *testing.T) {
		// Case A: Too soon (< 5 minutes in future)
		wA := httptest.NewRecorder()
		reqA := newJSONRequest(http.MethodPost, "/api/v1/creator/publish-scheduled", httpcreator.SchedulePublishRequestPayload{
			Title:       "Too Soon Video",
			PlayURL:     "/media/videos/too_soon.mp4",
			ScheduledAt: time.Now().Add(2 * time.Minute).Format(time.RFC3339),
		}, "192.168.10.50")
		reqA.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wA, reqA)
		if wA.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for scheduled_at < 5m in future, got %d: %s", wA.Code, wA.Body.String())
		}

		// Case B: In the past
		wB := httptest.NewRecorder()
		reqB := newJSONRequest(http.MethodPost, "/api/v1/creator/publish-scheduled", httpcreator.SchedulePublishRequestPayload{
			Title:       "Past Video",
			PlayURL:     "/media/videos/past.mp4",
			ScheduledAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		}, "192.168.10.50")
		reqB.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wB, reqB)
		if wB.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for scheduled_at in past, got %d: %s", wB.Code, wB.Body.String())
		}

		// Case C: Too far (> 30 days in future)
		wC := httptest.NewRecorder()
		reqC := newJSONRequest(http.MethodPost, "/api/v1/creator/publish-scheduled", httpcreator.SchedulePublishRequestPayload{
			Title:       "Far Future Video",
			PlayURL:     "/media/videos/far_future.mp4",
			ScheduledAt: time.Now().Add(35 * 24 * time.Hour).Format(time.RFC3339),
		}, "192.168.10.50")
		reqC.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wC, reqC)
		if wC.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for scheduled_at > 30d in future, got %d: %s", wC.Code, wC.Body.String())
		}

		// Case D: Invalid RFC3339 string
		wD := httptest.NewRecorder()
		reqD := newJSONRequest(http.MethodPost, "/api/v1/creator/publish-scheduled", httpcreator.SchedulePublishRequestPayload{
			Title:       "Malformed Date Video",
			PlayURL:     "/media/videos/malformed.mp4",
			ScheduledAt: "2026-99-99 88:88:88",
		}, "192.168.10.50")
		reqD.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wD, reqD)
		if wD.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for malformed scheduled_at, got %d: %s", wD.Code, wD.Body.String())
		}
	})

	// E2E-M7-04: Scheduled Publish Due Execution
	t.Run("E2E-M7-04_Scheduled_Publish_Due_Execution", func(t *testing.T) {
		dSched := saveTestDraft(t, ctx.Engine, tokenA, "Scheduled Draft", "/media/videos/scheduled.mp4")

		// 1. Create scheduled task for 10 minutes in future
		wSched := httptest.NewRecorder()
		reqSched := newJSONRequest(http.MethodPost, "/api/v1/creator/publish-scheduled", httpcreator.SchedulePublishRequestPayload{
			DraftID:     &dSched.ID,
			Title:       "Scheduled Video Title",
			PlayURL:     "/media/videos/scheduled.mp4",
			ScheduledAt: time.Now().Add(10 * time.Minute).Format(time.RFC3339),
		}, "192.168.10.50")
		reqSched.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wSched, reqSched)
		if wSched.Code != http.StatusCreated {
			t.Fatalf("expected 201 for SchedulePublish, got %d: %s", wSched.Code, wSched.Body.String())
		}

		var schedResp creatorScheduleResponseEnvelope
		parseJSONResponse(t, wSched, &schedResp)
		taskID := schedResp.Data.TaskID
		if taskID == "" {
			t.Fatalf("expected non-empty task_id")
		}

		// Verify draft status in DB is now scheduled (2)
		dCheck, err := ctx.DraftRepo.GetByID(context.Background(), dSched.ID)
		if err != nil || dCheck.Status != domaincreator.DraftStatusScheduled {
			t.Fatalf("expected draft status scheduled (2), got %+v, err=%v", dCheck, err)
		}

		// 2. Simulate time passing: update DB scheduled_at and Redis delay score to past
		pastTime := time.Now().Add(-1 * time.Minute)
		if err := ctx.DB.Exec("UPDATE scheduled_tasks SET scheduled_at = ? WHERE task_id = ?", pastTime, taskID).Error; err != nil {
			t.Fatalf("failed to update scheduled_at in DB: %v", err)
		}
		if err := ctx.RDB.ZAdd(context.Background(), "schedule:delay_queue:tasks", redis.Z{
			Score:  float64(pastTime.Unix()),
			Member: taskID,
		}).Err(); err != nil {
			t.Fatalf("failed to update score in Redis: %v", err)
		}

		// 3. Process due tasks via Scheduler
		processed, err := ctx.Scheduler.ProcessDueTasks(context.Background())
		if err != nil {
			t.Fatalf("failed to process due tasks: %v", err)
		}
		if processed < 1 {
			t.Fatalf("expected at least 1 task processed, got %d", processed)
		}

		// 4. Verify task in DB is published
		taskInDB, err := ctx.TaskRepo.GetByTaskID(context.Background(), taskID)
		if err != nil || taskInDB == nil {
			t.Fatalf("failed to fetch task from DB: %v", err)
		}
		if taskInDB.Status != domaincreator.ScheduledStatusPublished {
			t.Fatalf("expected task status published (2), got %d", taskInDB.Status)
		}
		if taskInDB.VideoID == nil || *taskInDB.VideoID <= 0 {
			t.Fatalf("expected task to have valid video_id, got %+v", taskInDB.VideoID)
		}

		// 5. Verify draft in DB is published
		dCheckAfter, err := ctx.DraftRepo.GetByID(context.Background(), dSched.ID)
		if err != nil || dCheckAfter.Status != domaincreator.DraftStatusPublished {
			t.Fatalf("expected draft status published (3), got %+v, err=%v", dCheckAfter, err)
		}

		// 6. Verify video on timeline
		score, err := ctx.RDB.ZScore(context.Background(), "timeline:global", fmt.Sprintf("%d", *taskInDB.VideoID)).Result()
		if err != nil || score <= 0 {
			t.Fatalf("expected scheduled video on timeline with positive score: score=%f, err=%v", score, err)
		}
	})

	// E2E-M7-05: Cancel Scheduled Task and Draft Reversion
	t.Run("E2E-M7-05_Cancel_Scheduled_Task_And_Draft_Reversion", func(t *testing.T) {
		dCancel := saveTestDraft(t, ctx.Engine, tokenA, "Cancelable Draft", "/media/videos/cancelable.mp4")

		// 1. Schedule publish
		wSched := httptest.NewRecorder()
		reqSched := newJSONRequest(http.MethodPost, "/api/v1/creator/publish-scheduled", httpcreator.SchedulePublishRequestPayload{
			DraftID:     &dCancel.ID,
			Title:       "Cancelable Video Title",
			PlayURL:     "/media/videos/cancelable.mp4",
			ScheduledAt: time.Now().Add(15 * time.Minute).Format(time.RFC3339),
		}, "192.168.10.50")
		reqSched.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wSched, reqSched)
		if wSched.Code != http.StatusCreated {
			t.Fatalf("expected 201 for SchedulePublish, got %d: %s", wSched.Code, wSched.Body.String())
		}
		var schedResp creatorScheduleResponseEnvelope
		parseJSONResponse(t, wSched, &schedResp)
		taskID := schedResp.Data.TaskID

		// Confirm draft is scheduled in DB
		dCheck, err := ctx.DraftRepo.GetByID(context.Background(), dCancel.ID)
		if err != nil || dCheck.Status != domaincreator.DraftStatusScheduled {
			t.Fatalf("expected draft status scheduled (2), got %+v", dCheck)
		}

		// Confirm task is in delay queue
		_, err = ctx.RDB.ZScore(context.Background(), "schedule:delay_queue:tasks", taskID).Result()
		if err != nil {
			t.Fatalf("expected task in Redis delay queue: %v", err)
		}

		// 2. Cancel scheduled task
		wCancel := httptest.NewRecorder()
		reqCancel := newJSONRequest(http.MethodDelete, fmt.Sprintf("/api/v1/creator/scheduled-tasks/%s", taskID), nil, "192.168.10.50")
		reqCancel.Header.Set("Authorization", "Bearer "+tokenA)
		ctx.Engine.ServeHTTP(wCancel, reqCancel)
		if wCancel.Code != http.StatusOK {
			t.Fatalf("expected 200 for CancelScheduledTask, got %d: %s", wCancel.Code, wCancel.Body.String())
		}

		// 3. Verify task is canceled in DB
		taskInDB, err := ctx.TaskRepo.GetByTaskID(context.Background(), taskID)
		if err != nil || taskInDB.Status != domaincreator.ScheduledStatusCanceled {
			t.Fatalf("expected task status canceled (3), got %+v, err=%v", taskInDB, err)
		}

		// 4. Verify draft is reverted back to draft (1) in DB
		dCheckReverted, err := ctx.DraftRepo.GetByID(context.Background(), dCancel.ID)
		if err != nil || dCheckReverted.Status != domaincreator.DraftStatusDraft {
			t.Fatalf("expected draft status reverted to draft (1), got %+v, err=%v", dCheckReverted, err)
		}

		// 5. Verify task is removed from Redis delay queue
		_, err = ctx.RDB.ZScore(context.Background(), "schedule:delay_queue:tasks", taskID).Result()
		if err != redis.Nil {
			t.Fatalf("expected task to be removed from delay queue, got err=%v", err)
		}
	})

	// E2E-M7-06: 50 Concurrent Publishes Monotonic Sequence Generation
	t.Run("E2E-M7-06_Concurrent_Publish_Monotonic_Sequence_Generation", func(t *testing.T) {
		const concurrency = 50
		var wg sync.WaitGroup
		seqChan := make(chan int64, concurrency)
		errChan := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				seq, err := ctx.SeqGen.NextPublishSeq(context.Background())
				if err != nil {
					errChan <- err
					return
				}
				seqChan <- seq
			}()
		}

		wg.Wait()
		close(seqChan)
		close(errChan)

		for err := range errChan {
			t.Fatalf("concurrent sequence generation failed: %v", err)
		}

		seqs := make([]int64, 0, concurrency)
		for s := range seqChan {
			seqs = append(seqs, s)
		}
		if len(seqs) != concurrency {
			t.Fatalf("expected %d sequences, got %d", concurrency, len(seqs))
		}

		sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })

		for i := 0; i < len(seqs); i++ {
			if seqs[i] <= 0 {
				t.Fatalf("sequence must be positive, got %d at index %d", seqs[i], i)
			}
			if i > 0 && seqs[i] != seqs[i-1]+1 {
				t.Fatalf("sequence gap or collision: seq[%d]=%d, seq[%d]=%d", i-1, seqs[i-1], i, seqs[i])
			}
		}
	})

	// E2E-M7-07: Sequence Gap Elimination on Timeline
	t.Run("E2E-M7-07_Sequence_Gap_Elimination_On_Timeline", func(t *testing.T) {
		// Clean up timeline for deterministic gap isolation
		_ = ctx.RDB.Del(context.Background(), "timeline:global").Err()

		// Allocate 5 monotonic sequences
		seqs := make([]int64, 5)
		for i := 0; i < 5; i++ {
			s, err := ctx.SeqGen.NextPublishSeq(context.Background())
			if err != nil {
				t.Fatalf("failed to allocate sequence: %v", err)
			}
			seqs[i] = s
		}

		// Create 5 fake video IDs corresponding to seqs[0]..seqs[4]
		videoIDs := []int64{70001, 70002, 70003, 70004, 70005}

		// Append to timeline in scrambled / out-of-order sequence:
		// Order: index 4, index 1, index 3, index 0, index 2
		scrambledOrder := []int{4, 1, 3, 0, 2}
		for _, idx := range scrambledOrder {
			err := ctx.TimelineRepo.AppendWithSeq(context.Background(), videoIDs[idx], seqs[idx])
			if err != nil {
				t.Fatalf("failed to append to timeline with seq: %v", err)
			}
		}

		// Page 1: limit 2, cursor 0
		page1, err := ctx.TimelineRepo.ListByCursor(context.Background(), 0, 2)
		if err != nil {
			t.Fatalf("page 1 failed: %v", err)
		}
		if len(page1) != 2 || page1[0] != videoIDs[4] || page1[1] != videoIDs[3] {
			t.Fatalf("expected page 1 [%d, %d], got %v", videoIDs[4], videoIDs[3], page1)
		}

		// Page 2: limit 2, cursor seqs[3]
		page2, err := ctx.TimelineRepo.ListByCursor(context.Background(), seqs[3], 2)
		if err != nil {
			t.Fatalf("page 2 failed: %v", err)
		}
		if len(page2) != 2 || page2[0] != videoIDs[2] || page2[1] != videoIDs[1] {
			t.Fatalf("expected page 2 [%d, %d], got %v", videoIDs[2], videoIDs[1], page2)
		}

		// Page 3: limit 2, cursor seqs[1]
		page3, err := ctx.TimelineRepo.ListByCursor(context.Background(), seqs[1], 2)
		if err != nil {
			t.Fatalf("page 3 failed: %v", err)
		}
		if len(page3) != 1 || page3[0] != videoIDs[0] {
			t.Fatalf("expected page 3 [%d], got %v", videoIDs[0], page3)
		}
	})

	// E2E-M7-08: Scheduler Worker Mutual Exclusion
	t.Run("E2E-M7-08_Scheduler_Worker_Mutual_Exclusion", func(t *testing.T) {
		taskTitle := fmt.Sprintf("Mutual Exclusion Task %s", suffix)
		dueTime := time.Now().Add(-5 * time.Second)
		taskID := uuid.New().String()

		// 1. Create a due scheduled task directly
		schedTask := &domaincreator.ScheduledTask{
			TaskID:      taskID,
			AuthorID:    userAID,
			Title:       taskTitle,
			Description: "Mutual exclusion desc",
			PlayURL:     "/media/videos/race.mp4",
			CoverURL:    "https://example.com/cover.jpg",
			DurationMs:  10000,
			ScheduledAt: dueTime,
			Status:      domaincreator.ScheduledStatusScheduled,
		}
		if err := ctx.TaskRepo.Create(context.Background(), schedTask); err != nil {
			t.Fatalf("failed to insert scheduled task in DB: %v", err)
		}
		if err := ctx.Scheduler.Enqueue(context.Background(), schedTask.TaskID, dueTime); err != nil {
			t.Fatalf("failed to enqueue task: %v", err)
		}

		// 2. Setup 2 competing schedulers
		scheduler1 := ctx.Scheduler
		scheduler2 := infrascheduler.NewDelayedScheduler(
			ctx.RDB,
			ctx.TaskRepo,
			ctx.VideoRepo,
			ctx.DraftRepo,
			ctx.SeqGen,
			ctx.TimelineRepo,
			ctx.PageCache,
			nil,
		)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = scheduler1.ProcessDueTasks(context.Background())
		}()
		go func() {
			defer wg.Done()
			_, _ = scheduler2.ProcessDueTasks(context.Background())
		}()
		wg.Wait()

		// 3. Verify exactly 1 video exists with this title in DB
		var videoCount int64
		if err := ctx.DB.Table("videos").Where("title = ?", taskTitle).Count(&videoCount).Error; err != nil {
			t.Fatalf("failed to count videos in DB: %v", err)
		}
		if videoCount != 1 {
			t.Fatalf("expected EXACTLY 1 video created due to mutual exclusion, got %d", videoCount)
		}

		// 4. Verify task in DB is published
		taskInDB, err := ctx.TaskRepo.GetByTaskID(context.Background(), schedTask.TaskID)
		if err != nil || taskInDB.Status != domaincreator.ScheduledStatusPublished {
			t.Fatalf("expected task status published (2), got %+v, err=%v", taskInDB, err)
		}
	})
}
