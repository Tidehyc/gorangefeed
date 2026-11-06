package moderation_test

import (
	"context"
	"testing"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	servicemoderation "github.com/Tidehyc/gorangefeed/internal/service/moderation"
)

type mockReviewRepo struct {
	tasks map[int64]*domainmoderation.ReviewTask
	logs  []*domainmoderation.ModerationLog
}

func newMockReviewRepo() *mockReviewRepo {
	return &mockReviewRepo{
		tasks: make(map[int64]*domainmoderation.ReviewTask),
		logs:  make([]*domainmoderation.ModerationLog, 0),
	}
}

func (m *mockReviewRepo) CreateTask(ctx context.Context, task *domainmoderation.ReviewTask) error {
	task.ID = int64(len(m.tasks) + 1)
	m.tasks[task.VideoID] = task
	return nil
}

func (m *mockReviewRepo) GetTaskByVideoID(ctx context.Context, videoID int64) (*domainmoderation.ReviewTask, error) {
	t, ok := m.tasks[videoID]
	if !ok {
		return nil, domainmoderation.ErrTaskNotFound
	}
	return t, nil
}

func (m *mockReviewRepo) GetTaskByID(ctx context.Context, id int64) (*domainmoderation.ReviewTask, error) {
	for _, t := range m.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, domainmoderation.ErrTaskNotFound
}

func (m *mockReviewRepo) UpdateTask(ctx context.Context, task *domainmoderation.ReviewTask) error {
	m.tasks[task.VideoID] = task
	return nil
}

func (m *mockReviewRepo) ListTasks(ctx context.Context, status domainmoderation.ReviewStatus, cursor int64, limit int) ([]*domainmoderation.ReviewTask, int64, bool, error) {
	var list []*domainmoderation.ReviewTask
	for _, t := range m.tasks {
		if status == 0 || t.Status == status {
			list = append(list, t)
		}
	}
	return list, 0, false, nil
}

func (m *mockReviewRepo) CreateLog(ctx context.Context, log *domainmoderation.ModerationLog) error {
	log.ID = int64(len(m.logs) + 1)
	m.logs = append(m.logs, log)
	return nil
}

func (m *mockReviewRepo) ListLogs(ctx context.Context, targetType string, targetID int64, limit int) ([]*domainmoderation.ModerationLog, error) {
	var list []*domainmoderation.ModerationLog
	for _, l := range m.logs {
		if (targetType == "" || l.TargetType == targetType) && (targetID == 0 || l.TargetID == targetID) {
			list = append(list, l)
		}
	}
	return list, nil
}

type mockVideoRepoForMod struct {
	updatedStatus map[int64]domainvideo.VideoStatus
	videos        map[int64]*domainvideo.Video
	updatedSeq    map[int64]int64
}

func (m *mockVideoRepoForMod) Create(ctx context.Context, video *domainvideo.Video) error { return nil }
func (m *mockVideoRepoForMod) CreateWithStats(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) error {
	return nil
}
func (m *mockVideoRepoForMod) CreateWithStatsAndOutbox(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats, outboxEvent *domainoutbox.Event) error {
	return nil
}
func (m *mockVideoRepoForMod) FindByID(ctx context.Context, id int64) (*domainvideo.Video, error) {
	if v, ok := m.videos[id]; ok {
		return v, nil
	}
	return nil, domainvideo.ErrVideoNotFound
}
func (m *mockVideoRepoForMod) FindDetailByID(ctx context.Context, id int64) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	return nil, nil, nil
}
func (m *mockVideoRepoForMod) FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	return nil, nil, nil
}
func (m *mockVideoRepoForMod) ListByAuthorCursor(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.Video, []*domainvideo.VideoStats, int64, bool, error) {
	return nil, nil, 0, false, nil
}
func (m *mockVideoRepoForMod) UpdateStatus(ctx context.Context, id int64, authorID int64, status domainvideo.VideoStatus) error {
	m.updatedStatus[id] = status
	return nil
}
func (m *mockVideoRepoForMod) UpdatePublishSeq(ctx context.Context, id int64, publishSeq int64) error {
	if m.updatedSeq == nil {
		m.updatedSeq = make(map[int64]int64)
	}
	m.updatedSeq[id] = publishSeq
	return nil
}

func (m *mockVideoRepoForMod) HideAllByAuthor(ctx context.Context, authorID int64) (int64, error) {
	return 0, nil
}

type mockSeqAllocatorForMod struct {
	seq   int64
	calls int
}

func (m *mockSeqAllocatorForMod) NextPublishSeq(ctx context.Context) (int64, error) {
	m.calls++
	return m.seq, nil
}

type mockTimelineRepoForMod struct {
	removedIDs   []int64
	appendedIDs  []int64
	appendedSeqs []int64
}

func (m *mockTimelineRepoForMod) GetFloor(ctx context.Context) (int64, error)     { return 0, nil }
func (m *mockTimelineRepoForMod) SetFloor(ctx context.Context, floor int64) error { return nil }
func (m *mockTimelineRepoForMod) Append(ctx context.Context, videoID int64) error {
	m.appendedIDs = append(m.appendedIDs, videoID)
	m.appendedSeqs = append(m.appendedSeqs, 0)
	return nil
}
func (m *mockTimelineRepoForMod) AppendWithSeq(ctx context.Context, videoID int64, seq int64) error {
	m.appendedIDs = append(m.appendedIDs, videoID)
	m.appendedSeqs = append(m.appendedSeqs, seq)
	return nil
}
func (m *mockTimelineRepoForMod) Remove(ctx context.Context, videoID int64) error {
	m.removedIDs = append(m.removedIDs, videoID)
	return nil
}
func (m *mockTimelineRepoForMod) Trim(ctx context.Context, floor int64) error { return nil }
func (m *mockTimelineRepoForMod) ListByCursor(ctx context.Context, cursor int64, limit int) ([]int64, error) {
	return nil, nil
}
func (m *mockTimelineRepoForMod) Rebuild(ctx context.Context, videoIDs []int64, floor int64) error {
	return nil
}

type mockAccountRepoForMod struct {
	userStatus map[int64]domainaccount.UserStatus
}

func (m *mockAccountRepoForMod) Create(ctx context.Context, user *domainaccount.User) error {
	return nil
}
func (m *mockAccountRepoForMod) FindByID(ctx context.Context, id int64) (*domainaccount.User, error) {
	return nil, nil
}
func (m *mockAccountRepoForMod) FindByUsername(ctx context.Context, username string) (*domainaccount.User, error) {
	return nil, nil
}
func (m *mockAccountRepoForMod) UpdateProfile(ctx context.Context, id int64, nickname, bio string) error {
	return nil
}
func (m *mockAccountRepoForMod) UpdateAvatar(ctx context.Context, id int64, avatarURL string) error {
	return nil
}
func (m *mockAccountRepoForMod) UpdateStatus(ctx context.Context, id int64, status domainaccount.UserStatus) error {
	m.userStatus[id] = status
	return nil
}
func (m *mockAccountRepoForMod) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	return nil
}
func (m *mockAccountRepoForMod) Anonymize(ctx context.Context, id int64) error {
	return nil
}

func TestModerationService_SubmitVideoForReview(t *testing.T) {
	ctx := context.Background()
	repo := newMockReviewRepo()
	videoRepo := &mockVideoRepoForMod{updatedStatus: make(map[int64]domainvideo.VideoStatus)}
	timelineRepo := &mockTimelineRepoForMod{}
	accountRepo := &mockAccountRepoForMod{userStatus: make(map[int64]domainaccount.UserStatus)}

	svc := servicemoderation.NewService(repo, nil, videoRepo, timelineRepo, accountRepo, nil, nil, nil)

	// 1. Clean video -> auto approved
	task1, err := svc.SubmitVideoForReview(ctx, 101, domainmoderation.ReviewPolicyPostReview, "日常记录：今天天气晴朗", "带小狗去户外草地玩")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !task1.IsApproved() {
		t.Fatalf("expected task1 to be auto approved, got %v", task1.Status)
	}

	// 2. High risk -> auto rejected
	task2, err := svc.SubmitVideoForReview(ctx, 102, domainmoderation.ReviewPolicyPostReview, "最新线上赌博平台上线", "快速提现真实可靠")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !task2.IsRejected() {
		t.Fatalf("expected task2 to be rejected, got %v", task2.Status)
	}
	if videoRepo.updatedStatus[102] != domainvideo.VideoStatusHidden {
		t.Fatalf("expected video 102 to be hidden")
	}
	if len(timelineRepo.removedIDs) != 1 || timelineRepo.removedIDs[0] != 102 {
		t.Fatalf("expected video 102 removed from timeline")
	}

	// 3. Medium risk -> pending human
	task3, err := svc.SubmitVideoForReview(ctx, 103, domainmoderation.ReviewPolicyPreReview, "轻松搞钱日入过万项目", "加微信详细咨询")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task3.Status != domainmoderation.ReviewStatusPendingHuman {
		t.Fatalf("expected task3 to be pending human, got %v", task3.Status)
	}
	if videoRepo.updatedStatus[103] != domainvideo.VideoStatusHidden {
		t.Fatalf("expected pre-review video 103 to be hidden pending audit")
	}
}

func TestModerationService_HumanReviewAndTakedown(t *testing.T) {
	ctx := context.Background()
	repo := newMockReviewRepo()
	videoRepo := &mockVideoRepoForMod{updatedStatus: make(map[int64]domainvideo.VideoStatus)}
	timelineRepo := &mockTimelineRepoForMod{}
	accountRepo := &mockAccountRepoForMod{userStatus: make(map[int64]domainaccount.UserStatus)}

	svc := servicemoderation.NewService(repo, nil, videoRepo, timelineRepo, accountRepo, nil, nil, nil)

	// Submit pending task
	task, _ := svc.SubmitVideoForReview(ctx, 201, domainmoderation.ReviewPolicyPreReview, "日入过万测试", "详情私聊")

	// 1. Human approve
	if err := svc.ReviewVideo(ctx, 201, 888, "approve", "cleared after review"); err != nil {
		t.Fatalf("unexpected error approving: %v", err)
	}
	if !task.IsApproved() {
		t.Fatalf("expected task approved")
	}
	if videoRepo.updatedStatus[201] != domainvideo.VideoStatusPublished {
		t.Fatalf("expected video 201 published")
	}

	// 2. Takedown
	if err := svc.TakedownVideo(ctx, 201, 999, "received copyright strike"); err != nil {
		t.Fatalf("unexpected error takedown: %v", err)
	}
	if !task.IsTakedown() {
		t.Fatalf("expected task takedown")
	}
	if videoRepo.updatedStatus[201] != domainvideo.VideoStatusHidden {
		t.Fatalf("expected video 201 hidden")
	}
}

func TestModerationService_BanUser(t *testing.T) {
	ctx := context.Background()
	repo := newMockReviewRepo()
	accountRepo := &mockAccountRepoForMod{userStatus: make(map[int64]domainaccount.UserStatus)}

	svc := servicemoderation.NewService(repo, nil, nil, nil, accountRepo, nil, nil, nil)

	if _, err := svc.BanUser(ctx, 501, 999, "repeated spam and scam attempts"); err != nil {
		t.Fatalf("unexpected error banning user: %v", err)
	}
	if accountRepo.userStatus[501] != domainaccount.UserStatusSuspended {
		t.Fatalf("expected user 501 to be suspended")
	}

	logs, err := svc.ListAuditLogs(ctx, domainmoderation.TargetTypeUser, 501, 10)
	if err != nil || len(logs) != 1 {
		t.Fatalf("expected 1 audit log for banned user, got %v", logs)
	}
}

func TestModerationService_ApproveAppendsTimelineWithPublishSeq(t *testing.T) {
	ctx := context.Background()
	repo := newMockReviewRepo()
	videoRepo := &mockVideoRepoForMod{
		updatedStatus: make(map[int64]domainvideo.VideoStatus),
		videos: map[int64]*domainvideo.Video{
			301: {ID: 301, PublishSeq: 5000},
		},
	}
	timelineRepo := &mockTimelineRepoForMod{}
	accountRepo := &mockAccountRepoForMod{userStatus: make(map[int64]domainaccount.UserStatus)}

	svc := servicemoderation.NewService(repo, nil, videoRepo, timelineRepo, accountRepo, nil, nil, nil)

	if _, err := svc.SubmitVideoForReview(ctx, 301, domainmoderation.ReviewPolicyPreReview, "日入过万测试", "详情私聊"); err != nil {
		t.Fatalf("unexpected error submitting for review: %v", err)
	}
	if err := svc.ReviewVideo(ctx, 301, 888, "approve", "cleared after review"); err != nil {
		t.Fatalf("unexpected error approving: %v", err)
	}

	if len(timelineRepo.appendedSeqs) != 1 {
		t.Fatalf("expected exactly 1 timeline append, got %v", timelineRepo.appendedSeqs)
	}
	if got := timelineRepo.appendedSeqs[0]; got != 5000 {
		t.Fatalf("expected timeline score to be publish_seq 5000, got %d (video ID must not be used as score)", got)
	}
}

func TestModerationService_ApproveAllocatesMissingPublishSeq(t *testing.T) {
	ctx := context.Background()
	repo := newMockReviewRepo()
	videoRepo := &mockVideoRepoForMod{
		updatedStatus: make(map[int64]domainvideo.VideoStatus),
		videos: map[int64]*domainvideo.Video{
			302: {ID: 302},
		},
	}
	timelineRepo := &mockTimelineRepoForMod{}
	accountRepo := &mockAccountRepoForMod{userStatus: make(map[int64]domainaccount.UserStatus)}
	seqAllocator := &mockSeqAllocatorForMod{seq: 9001}

	svc := servicemoderation.NewService(repo, nil, videoRepo, timelineRepo, accountRepo, nil, seqAllocator, nil)

	if _, err := svc.SubmitVideoForReview(ctx, 302, domainmoderation.ReviewPolicyPreReview, "日入过万测试", "详情私聊"); err != nil {
		t.Fatalf("unexpected error submitting for review: %v", err)
	}
	if err := svc.ReviewVideo(ctx, 302, 888, "approve", "cleared after review"); err != nil {
		t.Fatalf("unexpected error approving: %v", err)
	}

	if seqAllocator.calls != 1 {
		t.Fatalf("expected 1 sequence allocation, got %d", seqAllocator.calls)
	}
	if got := videoRepo.updatedSeq[302]; got != 9001 {
		t.Fatalf("expected allocated publish_seq 9001 persisted, got %d", got)
	}
	if len(timelineRepo.appendedSeqs) != 1 || timelineRepo.appendedSeqs[0] != 9001 {
		t.Fatalf("expected timeline append with seq 9001, got %v", timelineRepo.appendedSeqs)
	}
}
