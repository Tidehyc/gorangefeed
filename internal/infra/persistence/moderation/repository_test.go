package moderation_test

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistencemoderation "github.com/Tidehyc/gorangefeed/internal/infra/persistence/moderation"
	"gorm.io/gorm"
)

var testVideoIDCounter int64 = 8800000

func getUniqueVideoID() int64 {
	return atomic.AddInt64(&testVideoIDCounter, 1) + time.Now().UnixNano()%10000
}

func setupTestDB(t *testing.T) (*gorm.DB, domainmoderation.ReviewRepository) {
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
		t.Skipf("skipping moderation persistence test: config load failed (%v)", err)
		return nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping moderation persistence test: MySQL connection failed (%v)", err)
		return nil, nil
	}

	repo := persistencemoderation.NewRepository(db)
	return db, repo
}

func TestModerationRepository_TaskCRUD(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()
	videoID := getUniqueVideoID()

	task, err := domainmoderation.NewReviewTask(videoID, domainmoderation.ReviewPolicyPostReview)
	if err != nil {
		t.Fatalf("unexpected error creating task: %v", err)
	}

	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create review task: %v", err)
	}
	if task.ID <= 0 {
		t.Fatalf("expected positive task.ID, got %d", task.ID)
	}

	// Fetch by VideoID
	fetched, err := repo.GetTaskByVideoID(ctx, videoID)
	if err != nil {
		t.Fatalf("failed to get task by video id: %v", err)
	}
	if fetched.ID != task.ID || fetched.Status != domainmoderation.ReviewStatusPendingMachine {
		t.Fatalf("unexpected fetched task: %+v", fetched)
	}

	// Update task machine inspection
	_ = fetched.ApplyMachineResult(75, []string{"prohibited_word"})
	if err := repo.UpdateTask(ctx, fetched); err != nil {
		t.Fatalf("failed to update task: %v", err)
	}

	// Fetch by ID
	byID, err := repo.GetTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("failed to get task by id: %v", err)
	}
	if byID.Status != domainmoderation.ReviewStatusRejected || byID.RiskScore != 75 {
		t.Fatalf("unexpected updated task: %+v", byID)
	}
	if len(byID.MatchedWords) != 1 || byID.MatchedWords[0] != "prohibited_word" {
		t.Fatalf("unexpected matched words: %v", byID.MatchedWords)
	}
}

func TestModerationRepository_ListTasksPagination(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()

	var taskIDs []int64
	for i := 0; i < 5; i++ {
		vid := getUniqueVideoID()
		task, _ := domainmoderation.NewReviewTask(vid, domainmoderation.ReviewPolicyPreReview)
		_ = repo.CreateTask(ctx, task)
		taskIDs = append(taskIDs, task.ID)
	}

	tasks, nextCursor, hasMore, err := repo.ListTasks(ctx, domainmoderation.ReviewStatusPendingMachine, 0, 3)
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if !hasMore {
		t.Fatalf("expected hasMore to be true")
	}

	// Next page
	tasks2, _, _, err := repo.ListTasks(ctx, domainmoderation.ReviewStatusPendingMachine, nextCursor, 3)
	if err != nil {
		t.Fatalf("failed to list next page: %v", err)
	}
	if len(tasks2) == 0 {
		t.Fatalf("expected items on next page")
	}
}

func TestModerationRepository_AuditLogs(t *testing.T) {
	_, repo := setupTestDB(t)
	ctx := context.Background()
	videoID := getUniqueVideoID()

	log := &domainmoderation.ModerationLog{
		TargetType: domainmoderation.TargetTypeVideo,
		TargetID:   videoID,
		Action:     domainmoderation.ActionHumanApprove,
		OperatorID: 999,
		Reason:     "audit passed manual inspection",
		CreatedAt:  time.Now().UTC(),
	}

	if err := repo.CreateLog(ctx, log); err != nil {
		t.Fatalf("failed to create log: %v", err)
	}
	if log.ID <= 0 {
		t.Fatalf("expected positive log ID, got %d", log.ID)
	}

	logs, err := repo.ListLogs(ctx, domainmoderation.TargetTypeVideo, videoID, 10)
	if err != nil {
		t.Fatalf("failed to list logs: %v", err)
	}
	if len(logs) != 1 || logs[0].OperatorID != 999 {
		t.Fatalf("unexpected logs: %+v", logs)
	}
}
