package creator_test

import (
	"context"
	"testing"
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	servicecreator "github.com/Tidehyc/gorangefeed/internal/service/creator"
)

type mockDraftRepo struct {
	drafts   map[int64]*domaincreator.Draft
	nextID   int64
	createFn func(ctx context.Context, d *domaincreator.Draft) error
}

func newMockDraftRepo() *mockDraftRepo {
	return &mockDraftRepo{
		drafts: make(map[int64]*domaincreator.Draft),
		nextID: 1,
	}
}

func (m *mockDraftRepo) Create(ctx context.Context, draft *domaincreator.Draft) error {
	if m.createFn != nil {
		return m.createFn(ctx, draft)
	}
	draft.ID = m.nextID
	m.nextID++
	draft.CreatedAt = time.Now()
	draft.UpdatedAt = time.Now()
	m.drafts[draft.ID] = draft
	return nil
}

func (m *mockDraftRepo) GetByID(ctx context.Context, id int64) (*domaincreator.Draft, error) {
	d, ok := m.drafts[id]
	if !ok || d.Status == domaincreator.DraftStatusDeleted {
		return nil, domaincreator.ErrDraftNotFound
	}
	return d, nil
}

func (m *mockDraftRepo) Update(ctx context.Context, draft *domaincreator.Draft) error {
	m.drafts[draft.ID] = draft
	return nil
}

func (m *mockDraftRepo) Delete(ctx context.Context, id int64, authorID int64) error {
	d, ok := m.drafts[id]
	if !ok || d.Status == domaincreator.DraftStatusDeleted {
		return domaincreator.ErrDraftNotFound
	}
	if d.AuthorID != authorID {
		return domaincreator.ErrDraftForbidden
	}
	d.Status = domaincreator.DraftStatusDeleted
	return nil
}

func (m *mockDraftRepo) ListByAuthor(ctx context.Context, authorID int64, cursor *domaincreator.DraftCursor, limit int) ([]*domaincreator.Draft, *domaincreator.DraftCursor, bool, error) {
	var list []*domaincreator.Draft
	for _, d := range m.drafts {
		if d.AuthorID == authorID && d.Status != domaincreator.DraftStatusDeleted {
			list = append(list, d)
		}
	}
	return list, nil, false, nil
}

func (m *mockDraftRepo) PublishDraft(ctx context.Context, draftID int64, authorID int64, publishFn func(ctx context.Context, draft *domaincreator.Draft) (*domainvideo.Video, error)) (*domainvideo.Video, error) {
	d, ok := m.drafts[draftID]
	if !ok || d.Status == domaincreator.DraftStatusDeleted {
		return nil, domaincreator.ErrDraftNotFound
	}
	if d.AuthorID != authorID {
		return nil, domaincreator.ErrDraftForbidden
	}
	if d.Status == domaincreator.DraftStatusPublished {
		return nil, domaincreator.ErrDraftAlreadyPublished
	}
	v, err := publishFn(ctx, d)
	if err != nil {
		return nil, err
	}
	d.Status = domaincreator.DraftStatusPublished
	return v, nil
}

type mockTaskRepoForSvc struct {
	tasks map[string]*domaincreator.ScheduledTask
}

func newMockTaskRepoForSvc() *mockTaskRepoForSvc {
	return &mockTaskRepoForSvc{
		tasks: make(map[string]*domaincreator.ScheduledTask),
	}
}

func (m *mockTaskRepoForSvc) Create(ctx context.Context, task *domaincreator.ScheduledTask) error {
	m.tasks[task.TaskID] = task
	return nil
}

func (m *mockTaskRepoForSvc) GetByTaskID(ctx context.Context, taskID string) (*domaincreator.ScheduledTask, error) {
	t, ok := m.tasks[taskID]
	if !ok {
		return nil, domaincreator.ErrTaskNotFound
	}
	return t, nil
}

func (m *mockTaskRepoForSvc) ListByAuthor(ctx context.Context, authorID int64, limit int) ([]*domaincreator.ScheduledTask, error) {
	var list []*domaincreator.ScheduledTask
	for _, t := range m.tasks {
		if t.AuthorID == authorID {
			list = append(list, t)
		}
	}
	return list, nil
}

func (m *mockTaskRepoForSvc) Cancel(ctx context.Context, taskID string, authorID int64) error {
	t, ok := m.tasks[taskID]
	if !ok {
		return domaincreator.ErrTaskNotFound
	}
	if t.AuthorID != authorID {
		return domaincreator.ErrTaskForbidden
	}
	if t.Status != domaincreator.ScheduledStatusScheduled {
		return domaincreator.ErrTaskNotScheduled
	}
	t.Status = domaincreator.ScheduledStatusCanceled
	return nil
}

func (m *mockTaskRepoForSvc) AcquireDueTask(ctx context.Context, taskID string) (bool, error) {
	return true, nil
}
func (m *mockTaskRepoForSvc) MarkPublished(ctx context.Context, taskID string, videoID int64) error {
	return nil
}
func (m *mockTaskRepoForSvc) MarkFailed(ctx context.Context, taskID string, errMsg string) error {
	return nil
}
func (m *mockTaskRepoForSvc) ListDueTasks(ctx context.Context, dueTime time.Time, limit int) ([]*domaincreator.ScheduledTask, error) {
	return nil, nil
}

type mockVideoRepoForSvc struct {
	createdVideo *domainvideo.Video
}

func (m *mockVideoRepoForSvc) CreateWithStats(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) error {
	return m.CreateWithStatsAndOutbox(ctx, video, stats, nil)
}
func (m *mockVideoRepoForSvc) CreateWithStatsAndOutbox(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats, outboxEvent *domainoutbox.Event) error {
	video.ID = 555
	m.createdVideo = video
	return nil
}
func (m *mockVideoRepoForSvc) FindByID(ctx context.Context, id int64) (*domainvideo.Video, error) {
	return nil, nil
}
func (m *mockVideoRepoForSvc) FindDetailByID(ctx context.Context, id int64) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	return nil, nil, nil
}
func (m *mockVideoRepoForSvc) FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	return nil, nil, nil
}
func (m *mockVideoRepoForSvc) ListByAuthorCursor(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.Video, []*domainvideo.VideoStats, int64, bool, error) {
	return nil, nil, 0, false, nil
}
func (m *mockVideoRepoForSvc) UpdateStatus(ctx context.Context, id int64, authorID int64, status domainvideo.VideoStatus) error {
	return nil
}
func (m *mockVideoRepoForSvc) UpdatePublishSeq(ctx context.Context, id int64, publishSeq int64) error {
	return nil
}

func (m *mockVideoRepoForSvc) HideAllByAuthor(ctx context.Context, authorID int64) (int64, error) {
	return 0, nil
}

type mockSeqGen struct {
	current int64
}

func (m *mockSeqGen) NextPublishSeq(ctx context.Context) (int64, error) {
	m.current++
	return m.current, nil
}
func (m *mockSeqGen) GetCurrentSeq(ctx context.Context) (int64, error) {
	return m.current, nil
}
func (m *mockSeqGen) InitializeSequence(ctx context.Context, minVal int64) error {
	if m.current < minVal {
		m.current = minVal
	}
	return nil
}

func TestCreatorService_DraftLifecycle(t *testing.T) {
	draftRepo := newMockDraftRepo()
	taskRepo := newMockTaskRepoForSvc()
	videoRepo := &mockVideoRepoForSvc{}
	seqGen := &mockSeqGen{}

	svc := servicecreator.NewService(draftRepo, taskRepo, videoRepo, seqGen, nil, nil)
	ctx := context.Background()
	authorID := int64(101)

	// 1. Save draft
	draft, err := svc.SaveDraft(ctx, authorID, servicecreator.SaveDraftRequest{
		Title:       "My First Draft",
		Description: "Draft Description",
		PlayURL:     "http://example.com/play.mp4",
		CoverURL:    "http://example.com/cover.jpg",
		DurationMs:  15000,
	})
	if err != nil {
		t.Fatalf("save draft failed: %v", err)
	}
	if draft.ID != 1 || draft.Title != "My First Draft" {
		t.Fatalf("unexpected draft: %+v", draft)
	}

	// 2. Get draft
	fetched, err := svc.GetDraft(ctx, draft.ID, authorID)
	if err != nil || fetched.ID != draft.ID {
		t.Fatalf("get draft failed: %v", err)
	}

	// 3. Update draft
	updated, err := svc.UpdateDraft(ctx, draft.ID, authorID, servicecreator.UpdateDraftRequest{
		Title:       "Updated Draft Title",
		Description: "Updated Description",
		PlayURL:     "http://example.com/new_play.mp4",
	})
	if err != nil || updated.Title != "Updated Draft Title" {
		t.Fatalf("update draft failed: %v", err)
	}

	// 4. List drafts
	list, _, _, err := svc.ListDrafts(ctx, authorID, "", 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list drafts failed: %v, count: %d", err, len(list))
	}

	// 5. Publish draft
	video, err := svc.PublishDraft(ctx, draft.ID, authorID)
	if err != nil {
		t.Fatalf("publish draft failed: %v", err)
	}
	if video.ID != 555 || video.PublishSeq != 1 {
		t.Fatalf("expected video id=555 and publishSeq=1, got id=%d seq=%d", video.ID, video.PublishSeq)
	}

	// Verify draft is marked published
	publishedDraft, _ := draftRepo.GetByID(ctx, draft.ID)
	if publishedDraft.Status != domaincreator.DraftStatusPublished {
		t.Fatalf("expected draft status published, got %v", publishedDraft.Status)
	}

	// Cannot edit or publish again
	_, err = svc.UpdateDraft(ctx, draft.ID, authorID, servicecreator.UpdateDraftRequest{Title: "Fail"})
	if err != domaincreator.ErrDraftAlreadyPublished {
		t.Fatalf("expected ErrDraftAlreadyPublished, got %v", err)
	}

	// 6. Delete draft
	draft2, _ := svc.SaveDraft(ctx, authorID, servicecreator.SaveDraftRequest{Title: "To Delete", PlayURL: "http://example.com/play2.mp4"})
	if err := svc.DeleteDraft(ctx, draft2.ID, authorID); err != nil {
		t.Fatalf("delete draft failed: %v", err)
	}
	_, err = svc.GetDraft(ctx, draft2.ID, authorID)
	if err != domaincreator.ErrDraftNotFound {
		t.Fatalf("expected ErrDraftNotFound after delete, got %v", err)
	}
}

func TestCreatorService_ScheduledPublishAndCancel(t *testing.T) {
	draftRepo := newMockDraftRepo()
	taskRepo := newMockTaskRepoForSvc()
	videoRepo := &mockVideoRepoForSvc{}
	seqGen := &mockSeqGen{}

	svc := servicecreator.NewService(draftRepo, taskRepo, videoRepo, seqGen, nil, nil)
	ctx := context.Background()
	authorID := int64(102)

	// Create draft
	draft, err := svc.SaveDraft(ctx, authorID, servicecreator.SaveDraftRequest{
		Title:   "Draft to Schedule",
		PlayURL: "http://example.com/play.mp4",
	})
	if err != nil {
		t.Fatalf("save draft failed: %v", err)
	}

	// 1. Invalid schedule time (less than 5 minutes)
	_, err = svc.SchedulePublish(ctx, authorID, servicecreator.SchedulePublishRequest{
		DraftID:     &draft.ID,
		ScheduledAt: time.Now().UTC().Add(2 * time.Minute),
	})
	if err != domaincreator.ErrInvalidScheduledTime {
		t.Fatalf("expected ErrInvalidScheduledTime, got %v", err)
	}

	// 2. Valid schedule publish (10 minutes in the future)
	task, err := svc.SchedulePublish(ctx, authorID, servicecreator.SchedulePublishRequest{
		DraftID:     &draft.ID,
		ScheduledAt: time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("schedule publish failed: %v", err)
	}
	if task.TaskID == "" || task.Status != domaincreator.ScheduledStatusScheduled {
		t.Fatalf("unexpected task: %+v", task)
	}

	// Verify draft status transitioned to scheduled
	scheduledDraft, _ := draftRepo.GetByID(ctx, draft.ID)
	if scheduledDraft.Status != domaincreator.DraftStatusScheduled {
		t.Fatalf("expected draft status scheduled, got %v", scheduledDraft.Status)
	}

	// 3. List scheduled tasks
	tasks, err := svc.ListScheduledTasks(ctx, authorID, 10)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("list tasks failed: %v, count: %d", err, len(tasks))
	}

	// 4. Cancel scheduled task
	if err := svc.CancelScheduledTask(ctx, task.TaskID, authorID); err != nil {
		t.Fatalf("cancel scheduled task failed: %v", err)
	}

	// Verify task is canceled
	tRecord, _ := taskRepo.GetByTaskID(ctx, task.TaskID)
	if tRecord.Status != domaincreator.ScheduledStatusCanceled {
		t.Fatalf("expected canceled status, got %v", tRecord.Status)
	}
}
