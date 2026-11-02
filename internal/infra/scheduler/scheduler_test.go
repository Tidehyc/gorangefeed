package scheduler_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	infraredis "github.com/Tidehyc/gorangefeed/internal/infra/redis"
	infrascheduler "github.com/Tidehyc/gorangefeed/internal/infra/scheduler"
	"github.com/redis/go-redis/v9"
)

type mockScheduledTaskRepo struct {
	mu        sync.Mutex
	tasks     map[string]*domaincreator.ScheduledTask
	dueCalls  int
	acquireOK bool
}

func newMockTaskRepo() *mockScheduledTaskRepo {
	return &mockScheduledTaskRepo{
		tasks:     make(map[string]*domaincreator.ScheduledTask),
		acquireOK: true,
	}
}

func (m *mockScheduledTaskRepo) Create(ctx context.Context, task *domaincreator.ScheduledTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.TaskID] = task
	return nil
}

func (m *mockScheduledTaskRepo) GetByTaskID(ctx context.Context, taskID string) (*domaincreator.ScheduledTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return nil, domaincreator.ErrTaskNotFound
	}
	return t, nil
}

func (m *mockScheduledTaskRepo) ListByAuthor(ctx context.Context, authorID int64, limit int) ([]*domaincreator.ScheduledTask, error) {
	return nil, nil
}

func (m *mockScheduledTaskRepo) Cancel(ctx context.Context, taskID string, authorID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return domaincreator.ErrTaskNotFound
	}
	t.Status = domaincreator.ScheduledStatusCanceled
	return nil
}

func (m *mockScheduledTaskRepo) AcquireDueTask(ctx context.Context, taskID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok || t.Status != domaincreator.ScheduledStatusScheduled {
		return false, nil
	}
	t.Status = domaincreator.ScheduledStatusExecuting
	return true, nil
}

func (m *mockScheduledTaskRepo) MarkPublished(ctx context.Context, taskID string, videoID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return domaincreator.ErrTaskNotFound
	}
	t.Status = domaincreator.ScheduledStatusPublished
	t.VideoID = &videoID
	return nil
}

func (m *mockScheduledTaskRepo) MarkFailed(ctx context.Context, taskID string, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return domaincreator.ErrTaskNotFound
	}
	t.Status = domaincreator.ScheduledStatusFailed
	t.ErrorMsg = &errMsg
	return nil
}

func (m *mockScheduledTaskRepo) ListDueTasks(ctx context.Context, dueTime time.Time, limit int) ([]*domaincreator.ScheduledTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var due []*domaincreator.ScheduledTask
	for _, t := range m.tasks {
		if t.Status == domaincreator.ScheduledStatusScheduled && !t.ScheduledAt.After(dueTime) {
			due = append(due, t)
		}
	}
	return due, nil
}

type mockVideoRepoForScheduler struct {
	mu           sync.Mutex
	createdCount int
	createdVideo *domainvideo.Video
}

func (m *mockVideoRepoForScheduler) CreateWithStats(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) error {
	return m.CreateWithStatsAndOutbox(ctx, video, stats, nil)
}

func (m *mockVideoRepoForScheduler) CreateWithStatsAndOutbox(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats, outboxEvent *domainoutbox.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createdCount++
	video.ID = int64(1000 + m.createdCount)
	m.createdVideo = video
	return nil
}

func (m *mockVideoRepoForScheduler) FindByID(ctx context.Context, id int64) (*domainvideo.Video, error) {
	return nil, nil
}
func (m *mockVideoRepoForScheduler) FindDetailByID(ctx context.Context, id int64) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	return nil, nil, nil
}
func (m *mockVideoRepoForScheduler) FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	return nil, nil, nil
}
func (m *mockVideoRepoForScheduler) ListByAuthorCursor(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.Video, []*domainvideo.VideoStats, int64, bool, error) {
	return nil, nil, 0, false, nil
}
func (m *mockVideoRepoForScheduler) UpdateStatus(ctx context.Context, id int64, authorID int64, status domainvideo.VideoStatus) error {
	return nil
}
func (m *mockVideoRepoForScheduler) UpdatePublishSeq(ctx context.Context, id int64, publishSeq int64) error {
	return nil
}

type mockTimelineRepoForScheduler struct {
	mu           sync.Mutex
	appendedID   int64
	appendedSeq  int64
}

func (m *mockTimelineRepoForScheduler) GetFloor(ctx context.Context) (int64, error) { return 0, nil }
func (m *mockTimelineRepoForScheduler) SetFloor(ctx context.Context, floor int64) error { return nil }
func (m *mockTimelineRepoForScheduler) Append(ctx context.Context, videoID int64) error {
	return m.AppendWithSeq(ctx, videoID, videoID)
}
func (m *mockTimelineRepoForScheduler) AppendWithSeq(ctx context.Context, videoID int64, seq int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendedID = videoID
	m.appendedSeq = seq
	return nil
}
func (m *mockTimelineRepoForScheduler) Trim(ctx context.Context, floor int64) error { return nil }
func (m *mockTimelineRepoForScheduler) ListByCursor(ctx context.Context, cursor int64, limit int) ([]int64, error) {
	return nil, nil
}
func (m *mockTimelineRepoForScheduler) Rebuild(ctx context.Context, videoIDs []int64, floor int64) error {
	return nil
}

func setupRedisForSchedulerTest(t *testing.T) (*redis.Client, func()) {
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
		t.Skipf("skipping scheduler test: config load failed (%v)", err)
		return nil, nil
	}

	rdb, err := infraredis.New(&cfg.Redis)
	if err != nil {
		t.Skipf("skipping scheduler test: redis connection failed (%v)", err)
		return nil, nil
	}

	cleanup := func() {
		_ = rdb.Close()
	}
	return rdb, cleanup
}

func TestDelayedScheduler_EnqueueAndExecute(t *testing.T) {
	rdb, cleanup := setupRedisForSchedulerTest(t)
	if rdb == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	testPrefix := fmt.Sprintf("test:sched:%d:", time.Now().UnixNano())
	queueKey := testPrefix + "queue"
	lockPrefix := testPrefix + "lock:"
	seqKey := testPrefix + "seq"
	defer rdb.Del(ctx, queueKey, seqKey)

	taskRepo := newMockTaskRepo()
	videoRepo := &mockVideoRepoForScheduler{}
	timelineRepo := &mockTimelineRepoForScheduler{}
	seqGen := infraredis.NewCustomSequenceGenerator(rdb, seqKey)

	sched := infrascheduler.NewDelayedScheduler(
		rdb,
		taskRepo,
		videoRepo,
		nil,
		seqGen,
		timelineRepo,
		nil,
		&infrascheduler.Config{
			QueueKey:      queueKey,
			LockKeyPrefix: lockPrefix,
			PollInterval:  50 * time.Millisecond,
		},
	)

	// Create a scheduled task that is due right now
	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	task := &domaincreator.ScheduledTask{
		TaskID:      taskID,
		AuthorID:    100,
		Title:       "Scheduled Video Title",
		Description: "Scheduled Desc",
		PlayURL:     "http://example.com/play.mp4",
		CoverURL:    "http://example.com/cover.jpg",
		ScheduledAt: time.Now().UTC().Add(-1 * time.Second), // already due
		Status:      domaincreator.ScheduledStatusScheduled,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create mock task failed: %v", err)
	}

	// 1. Enqueue
	if err := sched.Enqueue(ctx, taskID, task.ScheduledAt); err != nil {
		t.Fatalf("enqueue task failed: %v", err)
	}

	// 2. Process due tasks
	count, err := sched.ProcessDueTasks(ctx)
	if err != nil {
		t.Fatalf("process due tasks failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 task executed, got %d", count)
	}

	// 3. Verify task state
	updatedTask, _ := taskRepo.GetByTaskID(ctx, taskID)
	if updatedTask.Status != domaincreator.ScheduledStatusPublished || updatedTask.VideoID == nil {
		t.Fatalf("expected task published with video ID, got %+v", updatedTask)
	}

	// 4. Verify video created and publishSeq assigned
	if videoRepo.createdCount != 1 || videoRepo.createdVideo == nil {
		t.Fatalf("expected 1 video created, got %d", videoRepo.createdCount)
	}
	if videoRepo.createdVideo.PublishSeq != 1 {
		t.Fatalf("expected publishSeq=1, got %d", videoRepo.createdVideo.PublishSeq)
	}

	// 5. Verify timeline appended with sequence
	if timelineRepo.appendedID != *updatedTask.VideoID || timelineRepo.appendedSeq != 1 {
		t.Fatalf("expected timeline appended with id=%d, seq=1; got id=%d, seq=%d",
			*updatedTask.VideoID, timelineRepo.appendedID, timelineRepo.appendedSeq)
	}
}

func TestDelayedScheduler_ConcurrencyMutualExclusion(t *testing.T) {
	rdb, cleanup := setupRedisForSchedulerTest(t)
	if rdb == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()
	testPrefix := fmt.Sprintf("test:concur_sched:%d:", time.Now().UnixNano())
	queueKey := testPrefix + "queue"
	lockPrefix := testPrefix + "lock:"
	seqKey := testPrefix + "seq"
	defer rdb.Del(ctx, queueKey, seqKey)

	taskRepo := newMockTaskRepo()
	videoRepo := &mockVideoRepoForScheduler{}
	timelineRepo := &mockTimelineRepoForScheduler{}
	seqGen := infraredis.NewCustomSequenceGenerator(rdb, seqKey)

	// Create 2 schedulers simulating 2 worker processes
	sched1 := infrascheduler.NewDelayedScheduler(
		rdb, taskRepo, videoRepo, nil, seqGen, timelineRepo, nil,
		&infrascheduler.Config{QueueKey: queueKey, LockKeyPrefix: lockPrefix},
	)
	sched2 := infrascheduler.NewDelayedScheduler(
		rdb, taskRepo, videoRepo, nil, seqGen, timelineRepo, nil,
		&infrascheduler.Config{QueueKey: queueKey, LockKeyPrefix: lockPrefix},
	)

	taskID := fmt.Sprintf("concur-task-%d", time.Now().UnixNano())
	task := &domaincreator.ScheduledTask{
		TaskID:      taskID,
		AuthorID:    200,
		Title:       "Concurrent Task",
		PlayURL:     "http://example.com/play.mp4",
		ScheduledAt: time.Now().UTC().Add(-2 * time.Second),
		Status:      domaincreator.ScheduledStatusScheduled,
	}
	_ = taskRepo.Create(ctx, task)
	_ = sched1.Enqueue(ctx, taskID, task.ScheduledAt)

	// Both schedulers race to process due tasks
	var wg sync.WaitGroup
	var executedCount int32

	runScheduler := func(s *infrascheduler.DelayedScheduler) {
		defer wg.Done()
		c, _ := s.ProcessDueTasks(ctx)
		atomic.AddInt32(&executedCount, int32(c))
	}

	wg.Add(2)
	go runScheduler(sched1)
	go runScheduler(sched2)
	wg.Wait()

	if executedCount != 1 {
		t.Fatalf("expected exactly 1 task execution across concurrent workers, got %d", executedCount)
	}
	if videoRepo.createdCount != 1 {
		t.Fatalf("expected exactly 1 video created, got %d", videoRepo.createdCount)
	}
}
