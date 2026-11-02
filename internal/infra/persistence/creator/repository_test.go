package creator_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistencecreator "github.com/Tidehyc/gorangefeed/internal/infra/persistence/creator"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, domaincreator.DraftRepository, domaincreator.ScheduledTaskRepository) {
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
		t.Skipf("skipping creator persistence test: config load failed (%v)", err)
		return nil, nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping creator persistence test: MySQL connection failed (%v)", err)
		return nil, nil, nil
	}

	draftRepo := persistencecreator.NewDraftRepository(db)
	taskRepo := persistencecreator.NewScheduledTaskRepository(db)
	return db, draftRepo, taskRepo
}

func TestDraftRepository_CRUDAndPermissions(t *testing.T) {
	_, draftRepo, _ := setupTestDB(t)
	ctx := context.Background()
	authorID := time.Now().UnixNano()

	draft := &domaincreator.Draft{
		AuthorID:    authorID,
		Title:       "Initial Draft Title",
		Description: "Initial Description",
		PlayURL:     "http://example.com/draft.mp4",
		CoverURL:    "http://example.com/draft.jpg",
		DurationMs:  12000,
		Status:      domaincreator.DraftStatusDraft,
	}

	// 1. Create
	if err := draftRepo.Create(ctx, draft); err != nil {
		t.Fatalf("create draft failed: %v", err)
	}
	if draft.ID <= 0 {
		t.Fatalf("expected positive draft ID, got %d", draft.ID)
	}

	// 2. GetByID
	fetched, err := draftRepo.GetByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("get draft by id failed: %v", err)
	}
	if fetched.Title != draft.Title || fetched.AuthorID != authorID {
		t.Fatalf("mismatched draft details: %+v", fetched)
	}

	// 3. Update with wrong author (should fail with ErrDraftForbidden)
	wrongAuthorDraft := *draft
	wrongAuthorDraft.AuthorID = authorID + 1
	wrongAuthorDraft.Title = "Malicious update"
	if err := draftRepo.Update(ctx, &wrongAuthorDraft); err != domaincreator.ErrDraftForbidden {
		t.Fatalf("expected ErrDraftForbidden, got %v", err)
	}

	// 4. Update with correct author
	draft.Title = "Updated Draft Title"
	draft.Description = "Updated Description"
	if err := draftRepo.Update(ctx, draft); err != nil {
		t.Fatalf("update draft failed: %v", err)
	}

	fetchedUpdated, err := draftRepo.GetByID(ctx, draft.ID)
	if err != nil || fetchedUpdated.Title != "Updated Draft Title" {
		t.Fatalf("expected updated title, got %v, err: %v", fetchedUpdated, err)
	}

	// 5. Delete with wrong author (should fail)
	if err := draftRepo.Delete(ctx, draft.ID, authorID+1); err != domaincreator.ErrDraftForbidden {
		t.Fatalf("expected ErrDraftForbidden, got %v", err)
	}

	// 6. Delete with correct author
	if err := draftRepo.Delete(ctx, draft.ID, authorID); err != nil {
		t.Fatalf("delete draft failed: %v", err)
	}

	// 7. Get after delete should return ErrDraftNotFound
	_, err = draftRepo.GetByID(ctx, draft.ID)
	if err != domaincreator.ErrDraftNotFound {
		t.Fatalf("expected ErrDraftNotFound after delete, got %v", err)
	}
}

func TestDraftRepository_ListCursorPagination(t *testing.T) {
	_, draftRepo, _ := setupTestDB(t)
	ctx := context.Background()
	authorID := time.Now().UnixNano()

	// Insert 5 drafts with slight delay to ensure distinct updated_at
	for i := 1; i <= 5; i++ {
		d := &domaincreator.Draft{
			AuthorID:    authorID,
			Title:       fmt.Sprintf("Draft %d", i),
			Description: "desc",
			PlayURL:     "http://example.com/play.mp4",
			Status:      domaincreator.DraftStatusDraft,
		}
		if err := draftRepo.Create(ctx, d); err != nil {
			t.Fatalf("create draft %d failed: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Page 1: limit 2
	p1, cursor1, hasMore1, err := draftRepo.ListByAuthor(ctx, authorID, nil, 2)
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(p1) != 2 || !hasMore1 || cursor1 == nil {
		t.Fatalf("expected 2 items and hasMore=true, got %d, hasMore=%v", len(p1), hasMore1)
	}

	// Page 2: limit 2 using cursor1
	p2, cursor2, hasMore2, err := draftRepo.ListByAuthor(ctx, authorID, cursor1, 2)
	if err != nil {
		t.Fatalf("page 2 failed: %v", err)
	}
	if len(p2) != 2 || !hasMore2 || cursor2 == nil {
		t.Fatalf("expected 2 items and hasMore=true, got %d, hasMore=%v", len(p2), hasMore2)
	}

	// Ensure no duplicates between Page 1 and Page 2
	seen := make(map[int64]bool)
	for _, item := range p1 {
		seen[item.ID] = true
	}
	for _, item := range p2 {
		if seen[item.ID] {
			t.Fatalf("duplicate draft ID %d found across pages", item.ID)
		}
	}

	// Page 3: limit 2 using cursor2 (should have 1 item left)
	p3, _, hasMore3, err := draftRepo.ListByAuthor(ctx, authorID, cursor2, 2)
	if err != nil {
		t.Fatalf("page 3 failed: %v", err)
	}
	if len(p3) != 1 || hasMore3 {
		t.Fatalf("expected 1 item and hasMore=false, got %d, hasMore=%v", len(p3), hasMore3)
	}
}

func TestDraftRepository_PublishDraft(t *testing.T) {
	_, draftRepo, _ := setupTestDB(t)
	ctx := context.Background()
	authorID := time.Now().UnixNano()

	draft := &domaincreator.Draft{
		AuthorID:    authorID,
		Title:       "Publishable Draft",
		Description: "Ready for launch",
		PlayURL:     "http://example.com/play.mp4",
		Status:      domaincreator.DraftStatusDraft,
	}
	if err := draftRepo.Create(ctx, draft); err != nil {
		t.Fatalf("create draft failed: %v", err)
	}

	// Publish draft
	video, err := draftRepo.PublishDraft(ctx, draft.ID, authorID, func(ctx context.Context, d *domaincreator.Draft) (*domainvideo.Video, error) {
		return &domainvideo.Video{
			ID:          999,
			AuthorID:    d.AuthorID,
			Title:       d.Title,
			Status:      domainvideo.VideoStatusPublished,
			PublishedAt: time.Now(),
		}, nil
	})
	if err != nil {
		t.Fatalf("publish draft failed: %v", err)
	}
	if video.ID != 999 {
		t.Fatalf("expected video id 999, got %d", video.ID)
	}

	// Check draft state transitioned to published
	fetched, err := draftRepo.GetByID(ctx, draft.ID)
	if err != nil {
		t.Fatalf("get draft failed: %v", err)
	}
	if fetched.Status != domaincreator.DraftStatusPublished {
		t.Fatalf("expected draft status published, got %v", fetched.Status)
	}

	// Second publish must be rejected
	_, err = draftRepo.PublishDraft(ctx, draft.ID, authorID, func(ctx context.Context, d *domaincreator.Draft) (*domainvideo.Video, error) {
		return nil, nil
	})
	if err != domaincreator.ErrDraftAlreadyPublished {
		t.Fatalf("expected ErrDraftAlreadyPublished, got %v", err)
	}
}

func TestScheduledTaskRepository_LifecycleAndReversion(t *testing.T) {
	_, draftRepo, taskRepo := setupTestDB(t)
	ctx := context.Background()
	authorID := time.Now().UnixNano()

	// 1. Create a draft to link to scheduled task
	draft := &domaincreator.Draft{
		AuthorID:    authorID,
		Title:       "Linked Draft",
		Description: "Scheduled",
		PlayURL:     "http://example.com/play.mp4",
		Status:      domaincreator.DraftStatusScheduled,
	}
	if err := draftRepo.Create(ctx, draft); err != nil {
		t.Fatalf("create draft failed: %v", err)
	}

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	scheduledAt := time.Now().UTC().Add(10 * time.Minute)

	task := &domaincreator.ScheduledTask{
		TaskID:      taskID,
		AuthorID:    authorID,
		DraftID:     &draft.ID,
		Title:       draft.Title,
		PlayURL:     draft.PlayURL,
		ScheduledAt: scheduledAt,
		Status:      domaincreator.ScheduledStatusScheduled,
	}

	// 2. Create task
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create scheduled task failed: %v", err)
	}

	// 3. GetByTaskID
	fetchedTask, err := taskRepo.GetByTaskID(ctx, taskID)
	if err != nil || fetchedTask.TaskID != taskID {
		t.Fatalf("get task failed: %v", err)
	}

	// 4. ListByAuthor
	taskList, err := taskRepo.ListByAuthor(ctx, authorID, 10)
	if err != nil || len(taskList) == 0 {
		t.Fatalf("list tasks failed: %v, count: %d", err, len(taskList))
	}

	// 5. Cancel task with correct author -> should revert linked draft to DraftStatusDraft
	if err := taskRepo.Cancel(ctx, taskID, authorID); err != nil {
		t.Fatalf("cancel task failed: %v", err)
	}

	canceledTask, err := taskRepo.GetByTaskID(ctx, taskID)
	if err != nil || canceledTask.Status != domaincreator.ScheduledStatusCanceled {
		t.Fatalf("expected canceled task status, got %v, err: %v", canceledTask, err)
	}

	revertedDraft, err := draftRepo.GetByID(ctx, draft.ID)
	if err != nil || revertedDraft.Status != domaincreator.DraftStatusDraft {
		t.Fatalf("expected linked draft to revert to draft status, got %v, err: %v", revertedDraft, err)
	}

	// 6. Cancel again should fail with ErrTaskNotScheduled
	if err := taskRepo.Cancel(ctx, taskID, authorID); err != domaincreator.ErrTaskNotScheduled {
		t.Fatalf("expected ErrTaskNotScheduled on repeat cancel, got %v", err)
	}
}

func TestScheduledTaskRepository_AcquireDueTask_CAS(t *testing.T) {
	_, _, taskRepo := setupTestDB(t)
	ctx := context.Background()
	authorID := time.Now().UnixNano()
	taskID := fmt.Sprintf("cas-%d", time.Now().UnixNano())

	task := &domaincreator.ScheduledTask{
		TaskID:      taskID,
		AuthorID:    authorID,
		Title:       "CAS Task",
		PlayURL:     "http://example.com/play.mp4",
		ScheduledAt: time.Now().UTC().Add(-1 * time.Minute), // due in the past
		Status:      domaincreator.ScheduledStatusScheduled,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	// Test ListDueTasks
	dueTasks, err := taskRepo.ListDueTasks(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("list due tasks failed: %v", err)
	}
	found := false
	for _, dt := range dueTasks {
		if dt.TaskID == taskID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected task %s in due tasks", taskID)
	}

	// Concurrency test: 2 workers race to acquire the same due task
	var wg sync.WaitGroup
	var successCount int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			acquired, err := taskRepo.AcquireDueTask(ctx, taskID)
			if err == nil && acquired {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Fatalf("expected exactly 1 worker to acquire task via CAS, got %d", successCount)
	}

	// Mark published
	if err := taskRepo.MarkPublished(ctx, taskID, 12345); err != nil {
		t.Fatalf("mark published failed: %v", err)
	}

	publishedTask, err := taskRepo.GetByTaskID(ctx, taskID)
	if err != nil || publishedTask.Status != domaincreator.ScheduledStatusPublished || *publishedTask.VideoID != 12345 {
		t.Fatalf("expected published task with videoID 12345, got %+v", publishedTask)
	}

	// Test MarkFailed on another task
	failTaskID := fmt.Sprintf("fail-%d", time.Now().UnixNano())
	failTask := &domaincreator.ScheduledTask{
		TaskID:      failTaskID,
		AuthorID:    authorID,
		Title:       "Fail Task",
		PlayURL:     "http://example.com/play.mp4",
		ScheduledAt: time.Now().UTC(),
		Status:      domaincreator.ScheduledStatusExecuting,
	}
	_ = taskRepo.Create(ctx, failTask)
	if err := taskRepo.MarkFailed(ctx, failTaskID, "ffmpeg transcoding timed out"); err != nil {
		t.Fatalf("mark failed error: %v", err)
	}
	failedFetched, _ := taskRepo.GetByTaskID(ctx, failTaskID)
	if failedFetched.Status != domaincreator.ScheduledStatusFailed || *failedFetched.ErrorMsg != "ffmpeg transcoding timed out" {
		t.Fatalf("expected failed task with error msg, got %+v", failedFetched)
	}
}
