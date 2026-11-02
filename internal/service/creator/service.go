package creator

import (
	"context"
	"fmt"
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	infrascheduler "github.com/Tidehyc/gorangefeed/internal/infra/scheduler"
	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
	"github.com/google/uuid"
)

type SaveDraftRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	PlayURL     string `json:"play_url"`
	CoverURL    string `json:"cover_url"`
	DurationMs  int    `json:"duration_ms"`
}

type UpdateDraftRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	PlayURL     string `json:"play_url"`
	CoverURL    string `json:"cover_url"`
	DurationMs  int    `json:"duration_ms"`
}

type SchedulePublishRequest struct {
	DraftID     *int64    `json:"draft_id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PlayURL     string    `json:"play_url"`
	CoverURL    string    `json:"cover_url"`
	DurationMs  int       `json:"duration_ms"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

type Service interface {
	SaveDraft(ctx context.Context, authorID int64, req SaveDraftRequest) (*domaincreator.Draft, error)
	GetDraft(ctx context.Context, draftID int64, authorID int64) (*domaincreator.Draft, error)
	UpdateDraft(ctx context.Context, draftID int64, authorID int64, req UpdateDraftRequest) (*domaincreator.Draft, error)
	DeleteDraft(ctx context.Context, draftID int64, authorID int64) error
	ListDrafts(ctx context.Context, authorID int64, cursor string, limit int) ([]*domaincreator.Draft, string, bool, error)
	PublishDraft(ctx context.Context, draftID int64, authorID int64) (*domainvideo.Video, error)

	SchedulePublish(ctx context.Context, authorID int64, req SchedulePublishRequest) (*domaincreator.ScheduledTask, error)
	ListScheduledTasks(ctx context.Context, authorID int64, limit int) ([]*domaincreator.ScheduledTask, error)
	CancelScheduledTask(ctx context.Context, taskID string, authorID int64) error
}

type service struct {
	draftRepo domaincreator.DraftRepository
	taskRepo  domaincreator.ScheduledTaskRepository
	videoRepo domainvideo.Repository
	seqGen    domaincreator.SequenceGenerator
	scheduler *infrascheduler.DelayedScheduler
	feedSvc   servicefeed.Service
}

// NewService constructs a new Creator Center Service.
func NewService(
	draftRepo domaincreator.DraftRepository,
	taskRepo domaincreator.ScheduledTaskRepository,
	videoRepo domainvideo.Repository,
	seqGen domaincreator.SequenceGenerator,
	scheduler *infrascheduler.DelayedScheduler,
	feedSvc servicefeed.Service,
) Service {
	return &service{
		draftRepo: draftRepo,
		taskRepo:  taskRepo,
		videoRepo: videoRepo,
		seqGen:    seqGen,
		scheduler: scheduler,
		feedSvc:   feedSvc,
	}
}

func (s *service) SaveDraft(ctx context.Context, authorID int64, req SaveDraftRequest) (*domaincreator.Draft, error) {
	if authorID <= 0 {
		return nil, domaincreator.ErrInvalidAuthorID
	}

	draft := &domaincreator.Draft{
		AuthorID:    authorID,
		Title:       req.Title,
		Description: req.Description,
		PlayURL:     req.PlayURL,
		CoverURL:    req.CoverURL,
		DurationMs:  req.DurationMs,
		Status:      domaincreator.DraftStatusDraft,
	}

	if err := draft.ValidateForSave(); err != nil {
		return nil, err
	}

	if err := s.draftRepo.Create(ctx, draft); err != nil {
		return nil, fmt.Errorf("save draft error: %w", err)
	}

	return draft, nil
}

func (s *service) GetDraft(ctx context.Context, draftID int64, authorID int64) (*domaincreator.Draft, error) {
	if draftID <= 0 {
		return nil, domaincreator.ErrDraftNotFound
	}
	if authorID <= 0 {
		return nil, domaincreator.ErrInvalidAuthorID
	}

	draft, err := s.draftRepo.GetByID(ctx, draftID)
	if err != nil {
		return nil, err
	}
	if draft.AuthorID != authorID {
		return nil, domaincreator.ErrDraftForbidden
	}

	return draft, nil
}

func (s *service) UpdateDraft(ctx context.Context, draftID int64, authorID int64, req UpdateDraftRequest) (*domaincreator.Draft, error) {
	if draftID <= 0 {
		return nil, domaincreator.ErrDraftNotFound
	}
	if authorID <= 0 {
		return nil, domaincreator.ErrInvalidAuthorID
	}

	existing, err := s.draftRepo.GetByID(ctx, draftID)
	if err != nil {
		return nil, err
	}
	if existing.AuthorID != authorID {
		return nil, domaincreator.ErrDraftForbidden
	}
	if existing.Status == domaincreator.DraftStatusPublished {
		return nil, domaincreator.ErrDraftAlreadyPublished
	}
	if existing.Status == domaincreator.DraftStatusScheduled {
		return nil, domaincreator.ErrDraftAlreadyScheduled
	}

	existing.Title = req.Title
	existing.Description = req.Description
	existing.PlayURL = req.PlayURL
	existing.CoverURL = req.CoverURL
	existing.DurationMs = req.DurationMs

	if err := existing.ValidateForSave(); err != nil {
		return nil, err
	}

	if err := s.draftRepo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update draft error: %w", err)
	}

	return existing, nil
}

func (s *service) DeleteDraft(ctx context.Context, draftID int64, authorID int64) error {
	if draftID <= 0 {
		return domaincreator.ErrDraftNotFound
	}
	if authorID <= 0 {
		return domaincreator.ErrInvalidAuthorID
	}

	return s.draftRepo.Delete(ctx, draftID, authorID)
}

func (s *service) ListDrafts(ctx context.Context, authorID int64, cursorStr string, limit int) ([]*domaincreator.Draft, string, bool, error) {
	if authorID <= 0 {
		return nil, "", false, domaincreator.ErrInvalidAuthorID
	}

	cursorObj, err := domaincreator.DecodeDraftCursor(cursorStr)
	if err != nil {
		return nil, "", false, err
	}

	drafts, nextCursor, hasMore, err := s.draftRepo.ListByAuthor(ctx, authorID, cursorObj, limit)
	if err != nil {
		return nil, "", false, err
	}

	return drafts, nextCursor.Encode(), hasMore, nil
}

func (s *service) PublishDraft(ctx context.Context, draftID int64, authorID int64) (*domainvideo.Video, error) {
	if draftID <= 0 {
		return nil, domaincreator.ErrDraftNotFound
	}
	if authorID <= 0 {
		return nil, domaincreator.ErrInvalidAuthorID
	}

	video, err := s.draftRepo.PublishDraft(ctx, draftID, authorID, func(pubCtx context.Context, draft *domaincreator.Draft) (*domainvideo.Video, error) {
		// 1. Generate monotonic sequence
		var seq int64
		if s.seqGen != nil {
			var seqErr error
			seq, seqErr = s.seqGen.NextPublishSeq(pubCtx)
			if seqErr != nil {
				return nil, fmt.Errorf("generate publish sequence error: %w", seqErr)
			}
		}

		// 2. Prepare video and stats
		v := &domainvideo.Video{
			AuthorID:    draft.AuthorID,
			Title:       draft.Title,
			Description: draft.Description,
			CoverURL:    draft.CoverURL,
			PlayURL:     draft.PlayURL,
			DurationMs:  draft.DurationMs,
			Status:      domainvideo.VideoStatusPublished,
			PublishSeq:  seq,
			PublishedAt: time.Now().UTC(),
		}
		stats := &domainvideo.VideoStats{
			LikeCount:    0,
			CommentCount: 0,
			ViewCount:    0,
		}

		outboxEvent, _ := domainoutbox.NewEvent(domainoutbox.EventTypeVideoPublished, map[string]any{
			"author_id": draft.AuthorID,
		})

		// 3. Atomically insert video and outbox event
		if err := s.videoRepo.CreateWithStatsAndOutbox(pubCtx, v, stats, outboxEvent); err != nil {
			return nil, fmt.Errorf("create video error: %w", err)
		}

		return v, nil
	})

	if err != nil {
		return nil, err
	}

	// 4. Append to Redis global feed timeline with sequence
	if s.feedSvc != nil && video != nil {
		if err := s.feedSvc.AppendTimelineWithSeq(ctx, video.ID, video.PublishSeq); err != nil {
			// Timeline append failure logged but does not abort publication
		}
	}

	return video, nil
}

func (s *service) SchedulePublish(ctx context.Context, authorID int64, req SchedulePublishRequest) (*domaincreator.ScheduledTask, error) {
	if authorID <= 0 {
		return nil, domaincreator.ErrInvalidAuthorID
	}

	title := req.Title
	description := req.Description
	playURL := req.PlayURL
	coverURL := req.CoverURL
	durationMs := req.DurationMs

	// If linked to a draft, validate draft and inherit unset fields
	if req.DraftID != nil && *req.DraftID > 0 {
		draft, err := s.draftRepo.GetByID(ctx, *req.DraftID)
		if err != nil {
			return nil, err
		}
		if draft.AuthorID != authorID {
			return nil, domaincreator.ErrDraftForbidden
		}
		if draft.Status == domaincreator.DraftStatusPublished {
			return nil, domaincreator.ErrDraftAlreadyPublished
		}
		if draft.Status == domaincreator.DraftStatusScheduled {
			return nil, domaincreator.ErrDraftAlreadyScheduled
		}

		if title == "" {
			title = draft.Title
		}
		if description == "" {
			description = draft.Description
		}
		if playURL == "" {
			playURL = draft.PlayURL
		}
		if coverURL == "" {
			coverURL = draft.CoverURL
		}
		if durationMs == 0 {
			durationMs = draft.DurationMs
		}

		// Transition draft status to scheduled
		draft.MarkScheduled()
		if err := s.draftRepo.Update(ctx, draft); err != nil {
			return nil, fmt.Errorf("update draft to scheduled state error: %w", err)
		}
	}

	taskID := uuid.NewString()
	task := &domaincreator.ScheduledTask{
		TaskID:      taskID,
		AuthorID:    authorID,
		DraftID:     req.DraftID,
		Title:       title,
		Description: description,
		PlayURL:     playURL,
		CoverURL:    coverURL,
		DurationMs:  durationMs,
		ScheduledAt: req.ScheduledAt,
		Status:      domaincreator.ScheduledStatusScheduled,
	}

	now := time.Now().UTC()
	if err := task.Validate(now); err != nil {
		// If validation fails and draft was marked scheduled, revert draft
		if req.DraftID != nil && *req.DraftID > 0 {
			if draft, loadErr := s.draftRepo.GetByID(ctx, *req.DraftID); loadErr == nil {
				draft.MarkDraft()
				_ = s.draftRepo.Update(ctx, draft)
			}
		}
		return nil, err
	}

	if err := s.taskRepo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("create scheduled task error: %w", err)
	}

	if s.scheduler != nil {
		if err := s.scheduler.Enqueue(ctx, task.TaskID, task.ScheduledAt); err != nil {
			// Enqueue error logged, task is safely persisted in MySQL for fallback poll
		}
	}

	return task, nil
}

func (s *service) ListScheduledTasks(ctx context.Context, authorID int64, limit int) ([]*domaincreator.ScheduledTask, error) {
	if authorID <= 0 {
		return nil, domaincreator.ErrInvalidAuthorID
	}
	return s.taskRepo.ListByAuthor(ctx, authorID, limit)
}

func (s *service) CancelScheduledTask(ctx context.Context, taskID string, authorID int64) error {
	if taskID == "" {
		return domaincreator.ErrTaskNotFound
	}
	if authorID <= 0 {
		return domaincreator.ErrInvalidAuthorID
	}

	if err := s.taskRepo.Cancel(ctx, taskID, authorID); err != nil {
		return err
	}

	if s.scheduler != nil {
		_ = s.scheduler.Remove(ctx, taskID)
	}

	return nil
}
