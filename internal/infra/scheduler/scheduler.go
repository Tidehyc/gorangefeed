package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	DefaultDelayQueueKey   = "schedule:delay_queue:tasks"
	DefaultLockKeyPrefix   = "lock:schedule:task:"
	DefaultPollInterval    = 1 * time.Second
	DefaultLockExpiry      = 30 * time.Second
)

// Config configures the delayed scheduler behavior.
type Config struct {
	QueueKey      string
	LockKeyPrefix string
	PollInterval  time.Duration
}

// DelayedScheduler handles the execution of scheduled publish tasks using Redis ZSets
// and distributed locking for mutual exclusion across worker replicas.
type DelayedScheduler struct {
	rdb          redis.Cmdable
	queueKey     string
	lockPrefix   string
	pollInterval time.Duration
	workerID     string

	taskRepo     domaincreator.ScheduledTaskRepository
	videoRepo    domainvideo.Repository
	draftRepo    domaincreator.DraftRepository
	seqGen       domaincreator.SequenceGenerator
	timelineRepo domainfeed.TimelineRepository
	pageCache    domainfeed.PageCacheRepository

	onPublishedHook func(ctx context.Context, videoID int64, publishSeq int64)

	stopCh chan struct{}
	wg     sync.WaitGroup
	mu     sync.Mutex
	running bool
}

// NewDelayedScheduler creates a new DelayedScheduler instance.
func NewDelayedScheduler(
	rdb redis.Cmdable,
	taskRepo domaincreator.ScheduledTaskRepository,
	videoRepo domainvideo.Repository,
	draftRepo domaincreator.DraftRepository,
	seqGen domaincreator.SequenceGenerator,
	timelineRepo domainfeed.TimelineRepository,
	pageCache domainfeed.PageCacheRepository,
	cfg *Config,
) *DelayedScheduler {
	queueKey := DefaultDelayQueueKey
	lockPrefix := DefaultLockKeyPrefix
	pollInterval := DefaultPollInterval

	if cfg != nil {
		if cfg.QueueKey != "" {
			queueKey = cfg.QueueKey
		}
		if cfg.LockKeyPrefix != "" {
			lockPrefix = cfg.LockKeyPrefix
		}
		if cfg.PollInterval > 0 {
			pollInterval = cfg.PollInterval
		}
	}

	return &DelayedScheduler{
		rdb:          rdb,
		queueKey:     queueKey,
		lockPrefix:   lockPrefix,
		pollInterval: pollInterval,
		workerID:     uuid.NewString(),
		taskRepo:     taskRepo,
		videoRepo:    videoRepo,
		draftRepo:    draftRepo,
		seqGen:       seqGen,
		timelineRepo: timelineRepo,
		pageCache:    pageCache,
		stopCh:       make(chan struct{}),
	}
}

// SetOnPublishedHook allows wiring an optional hook invoked when a scheduled video is published.
func (s *DelayedScheduler) SetOnPublishedHook(hook func(ctx context.Context, videoID int64, publishSeq int64)) {
	s.onPublishedHook = hook
}

// Enqueue adds a task ID to the Redis delay sorted set scored by its target Unix timestamp.
func (s *DelayedScheduler) Enqueue(ctx context.Context, taskID string, scheduledAt time.Time) error {
	if taskID == "" {
		return errors.New("taskID cannot be empty")
	}
	err := s.rdb.ZAdd(ctx, s.queueKey, redis.Z{
		Score:  float64(scheduledAt.Unix()),
		Member: taskID,
	}).Err()
	if err != nil {
		return fmt.Errorf("redis enqueue scheduled task error (%s): %w", taskID, err)
	}
	return nil
}

// Remove removes a task ID from the Redis delay sorted set (e.g. upon cancellation).
func (s *DelayedScheduler) Remove(ctx context.Context, taskID string) error {
	if taskID == "" {
		return errors.New("taskID cannot be empty")
	}
	err := s.rdb.ZRem(ctx, s.queueKey, taskID).Err()
	if err != nil {
		return fmt.Errorf("redis remove scheduled task error (%s): %w", taskID, err)
	}
	return nil
}

// ProcessDueTasks scans for due tasks, acquires locks, executes publication atomically, and returns count.
func (s *DelayedScheduler) ProcessDueTasks(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	nowScore := strconv.FormatInt(now.Unix(), 10)

	// 1. Scan due tasks from Redis ZSet
	taskIDs, err := s.rdb.ZRangeByScore(ctx, s.queueKey, &redis.ZRangeBy{
		Min:    "-inf",
		Max:    nowScore,
		Offset: 0,
		Count:  20,
	}).Result()
	if err != nil {
		return 0, fmt.Errorf("scan redis due tasks error: %w", err)
	}

	// 2. MySQL fallback scan: ensure no tasks are missed if Redis was restarted
	if s.taskRepo != nil {
		dbDue, dbErr := s.taskRepo.ListDueTasks(ctx, now, 20)
		if dbErr == nil && len(dbDue) > 0 {
			seen := make(map[string]bool, len(taskIDs))
			for _, id := range taskIDs {
				seen[id] = true
			}
			for _, t := range dbDue {
				if !seen[t.TaskID] {
					taskIDs = append(taskIDs, t.TaskID)
				}
			}
		}
	}

	if len(taskIDs) == 0 {
		return 0, nil
	}

	executedCount := 0
	for _, taskID := range taskIDs {
		executed, procErr := s.processSingleTask(ctx, taskID)
		if procErr != nil {
			// Log and continue to next task without breaking the whole loop
			continue
		}
		if executed {
			executedCount++
		}
	}

	return executedCount, nil
}

func (s *DelayedScheduler) processSingleTask(ctx context.Context, taskID string) (bool, error) {
	lockKey := s.lockPrefix + taskID

	// 1. Acquire Redis distributed lock
	locked, err := s.rdb.SetNX(ctx, lockKey, s.workerID, DefaultLockExpiry).Result()
	if err != nil || !locked {
		return false, nil // Another worker is actively handling this task
	}
	defer s.releaseLock(ctx, lockKey)

	// 2. Atomic CAS state check in MySQL (status: scheduled -> executing)
	acquired, err := s.taskRepo.AcquireDueTask(ctx, taskID)
	if err != nil {
		return false, fmt.Errorf("acquire task CAS error (%s): %w", taskID, err)
	}
	if !acquired {
		// Task was already executed, canceled, or acquired by another process
		_ = s.Remove(ctx, taskID)
		return false, nil
	}

	// Task successfully acquired by this worker! Remove from Redis delay queue
	_ = s.Remove(ctx, taskID)

	// 3. Load task metadata
	task, err := s.taskRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		_ = s.taskRepo.MarkFailed(ctx, taskID, fmt.Sprintf("fetch task details error: %v", err))
		return false, err
	}

	// 4. Generate monotonic publish sequence
	var publishSeq int64
	if s.seqGen != nil {
		seq, seqErr := s.seqGen.NextPublishSeq(ctx)
		if seqErr != nil {
			_ = s.taskRepo.MarkFailed(ctx, taskID, fmt.Sprintf("generate sequence error: %v", seqErr))
			return false, seqErr
		}
		publishSeq = seq
	}

	// 5. Insert video row and create outbox event
	video := &domainvideo.Video{
		AuthorID:    task.AuthorID,
		Title:       task.Title,
		Description: task.Description,
		CoverURL:    task.CoverURL,
		PlayURL:     task.PlayURL,
		DurationMs:  task.DurationMs,
		Status:      domainvideo.VideoStatusPublished,
		PublishSeq:  publishSeq,
		PublishedAt: time.Now().UTC(),
	}
	stats := &domainvideo.VideoStats{
		LikeCount:    0,
		CommentCount: 0,
		ViewCount:    0,
	}

	outboxEvent, _ := domainoutbox.NewEvent(domainoutbox.EventTypeVideoPublished, map[string]any{
		"author_id": task.AuthorID,
	})

	if err := s.videoRepo.CreateWithStatsAndOutbox(ctx, video, stats, outboxEvent); err != nil {
		_ = s.taskRepo.MarkFailed(ctx, taskID, fmt.Sprintf("create video error: %v", err))
		return false, err
	}

	// 6. Update scheduled task as published
	if err := s.taskRepo.MarkPublished(ctx, taskID, video.ID); err != nil {
		return false, err
	}

	// 7. Update linked draft status if present
	if task.DraftID != nil && *task.DraftID > 0 && s.draftRepo != nil {
		if draft, draftErr := s.draftRepo.GetByID(ctx, *task.DraftID); draftErr == nil && draft != nil {
			draft.MarkPublished()
			_ = s.draftRepo.Update(ctx, draft)
		}
	}

	// 8. Append to Redis global feed timeline with publishSeq
	if s.timelineRepo != nil {
		_ = s.timelineRepo.AppendWithSeq(ctx, video.ID, publishSeq)
	}
	if s.pageCache != nil {
		_ = s.pageCache.DeletePage(ctx, "feed:page:global:public:0:10:v1")
		_ = s.pageCache.DeletePage(ctx, "feed:page:global:public:0:3:v1")
		_ = s.pageCache.DeletePage(ctx, "feed:page:global:public:0:2:v1")
	}

	if s.onPublishedHook != nil {
		s.onPublishedHook(ctx, video.ID, publishSeq)
	}

	return true, nil
}

func (s *DelayedScheduler) releaseLock(ctx context.Context, lockKey string) {
	// Simple lock release (or Lua check-and-del)
	val, err := s.rdb.Get(ctx, lockKey).Result()
	if err == nil && val == s.workerID {
		_ = s.rdb.Del(ctx, lockKey).Err()
	}
}

// Start launches the background scheduler loop.
func (s *DelayedScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case <-ticker.C:
				_, _ = s.ProcessDueTasks(ctx)
			}
		}
	}()
}

// Stop gracefully shuts down the background scheduler loop.
func (s *DelayedScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	s.wg.Wait()
}
