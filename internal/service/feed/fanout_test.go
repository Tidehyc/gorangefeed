package feed

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	"gorm.io/gorm"
)

type mockRelationRepoForFanout struct {
	statsMap     map[int64]*domainrelation.RelationStats
	followersMap map[int64][]int64
	followingsMap map[int64][]int64
}

func (m *mockRelationRepoForFanout) Follow(ctx context.Context, followerID, vloggerID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	return true, nil
}
func (m *mockRelationRepoForFanout) Unfollow(ctx context.Context, followerID, vloggerID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	return true, nil
}
func (m *mockRelationRepoForFanout) GetRelation(ctx context.Context, userID, targetID int64) (*domainrelation.Friendship, error) {
	return nil, nil
}
func (m *mockRelationRepoForFanout) BatchIsFollowed(ctx context.Context, followerID int64, authorIDs []int64) (map[int64]bool, error) {
	res := make(map[int64]bool)
	for _, aid := range authorIDs {
		res[aid] = true
	}
	return res, nil
}
func (m *mockRelationRepoForFanout) ListFollowingIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error) {
	ids := m.followingsMap[userID]
	return ids, 0, false, nil
}
func (m *mockRelationRepoForFanout) ListFollowerIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error) {
	ids := m.followersMap[userID]
	return ids, 0, false, nil
}
func (m *mockRelationRepoForFanout) GetStats(ctx context.Context, userID int64) (*domainrelation.RelationStats, error) {
	if s, ok := m.statsMap[userID]; ok {
		return s, nil
	}
	return &domainrelation.RelationStats{UserID: userID}, nil
}

type mockFollowingRepoForFanout struct {
	mu           sync.Mutex
	inboxPushes  []inboxPushCall
	outboxPushes []outboxPushCall
	inboxZSets   map[int64][]int64
	authorZSets  map[int64][]int64
	inboxExists  map[int64]bool
	dbVideos     []int64
	prewarmCalls []prewarmCall
	pushInboxErr error
}

type inboxPushCall struct {
	followerIDs []int64
	videoID     int64
}

type outboxPushCall struct {
	authorID int64
	videoID  int64
}

type prewarmCall struct {
	userID   int64
	videoIDs []int64
}

func (m *mockFollowingRepoForFanout) PushToInboxes(ctx context.Context, followerIDs []int64, videoID int64, maxCapacity int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pushInboxErr != nil {
		return m.pushInboxErr
	}
	m.inboxPushes = append(m.inboxPushes, inboxPushCall{followerIDs: followerIDs, videoID: videoID})
	return nil
}

func (m *mockFollowingRepoForFanout) PushToAuthorOutbox(ctx context.Context, authorID int64, videoID int64, maxCapacity int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outboxPushes = append(m.outboxPushes, outboxPushCall{authorID: authorID, videoID: videoID})
	return nil
}

func (m *mockFollowingRepoForFanout) GetInboxVideoIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inboxZSets[userID], nil
}

func (m *mockFollowingRepoForFanout) GetAuthorOutboxVideoIDs(ctx context.Context, authorID int64, cursor int64, limit int) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.authorZSets[authorID], nil
}

func (m *mockFollowingRepoForFanout) CheckInboxExists(ctx context.Context, userID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inboxExists[userID], nil
}

func (m *mockFollowingRepoForFanout) PrewarmInbox(ctx context.Context, userID int64, videoIDs []int64, maxCapacity int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prewarmCalls = append(m.prewarmCalls, prewarmCall{userID: userID, videoIDs: videoIDs})
	return nil
}

func (m *mockFollowingRepoForFanout) ListFollowingVideosFromDB(ctx context.Context, authorIDs []int64, cursor int64, limit int) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dbVideos, nil
}

type mockDeduplicatorForFanout struct {
	mu          sync.Mutex
	consumedMap map[string]bool
}

func (m *mockDeduplicatorForFanout) MarkConsumed(ctx context.Context, eventID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.consumedMap == nil {
		m.consumedMap = make(map[string]bool)
	}
	if m.consumedMap[eventID] {
		return false, nil // already consumed
	}
	m.consumedMap[eventID] = true
	return true, nil
}

func (m *mockDeduplicatorForFanout) ExecuteWithDeduplication(ctx context.Context, eventID string, fn func(tx *gorm.DB) error) error {
	return nil
}

func (m *mockDeduplicatorForFanout) IsConsumed(ctx context.Context, eventID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.consumedMap[eventID], nil
}

func (m *mockDeduplicatorForFanout) PurgeConsumedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return 0, nil
}

func TestFanoutService_NormalAuthor_PushToInboxes(t *testing.T) {
	normalAuthorID := int64(1001)
	followers := []int64{2001, 2002, 2003}

	relationRepo := &mockRelationRepoForFanout{
		statsMap: map[int64]*domainrelation.RelationStats{
			normalAuthorID: {UserID: normalAuthorID, FollowerCount: 3}, // < 5000 -> Push
		},
		followersMap: map[int64][]int64{
			normalAuthorID: followers,
		},
	}
	followingRepo := &mockFollowingRepoForFanout{}
	dedup := &mockDeduplicatorForFanout{}

	fanoutSvc, err := NewFanoutService(relationRepo, followingRepo, dedup)
	if err != nil {
		t.Fatalf("NewFanoutService failed: %v", err)
	}

	payload, _ := json.Marshal(VideoPublishedPayload{
		VideoID:  777,
		AuthorID: normalAuthorID,
	})

	ctx := context.Background()
	eventID := "evt-publish-1"
	if err := fanoutSvc.HandleVideoPublished(ctx, eventID, payload); err != nil {
		t.Fatalf("HandleVideoPublished failed: %v", err)
	}

	// Assert pushed to inboxes
	if len(followingRepo.inboxPushes) != 1 {
		t.Fatalf("expected 1 inbox push call, got %d", len(followingRepo.inboxPushes))
	}
	call := followingRepo.inboxPushes[0]
	if call.videoID != 777 || len(call.followerIDs) != 3 {
		t.Errorf("unexpected inbox push: videoID=%d, followers=%v", call.videoID, call.followerIDs)
	}

	// Assert author outbox was NOT pushed
	if len(followingRepo.outboxPushes) != 0 {
		t.Errorf("expected 0 outbox push calls for normal author, got %d", len(followingRepo.outboxPushes))
	}
}

func TestFanoutService_BigVAuthor_PushToAuthorOutbox(t *testing.T) {
	bigVAuthorID := int64(5001)

	relationRepo := &mockRelationRepoForFanout{
		statsMap: map[int64]*domainrelation.RelationStats{
			bigVAuthorID: {UserID: bigVAuthorID, FollowerCount: 15000}, // >= 5000 -> Pull
		},
	}
	followingRepo := &mockFollowingRepoForFanout{}
	dedup := &mockDeduplicatorForFanout{}

	fanoutSvc, err := NewFanoutService(relationRepo, followingRepo, dedup)
	if err != nil {
		t.Fatalf("NewFanoutService failed: %v", err)
	}

	payload, _ := json.Marshal(VideoPublishedPayload{
		VideoID:  888,
		AuthorID: bigVAuthorID,
	})

	ctx := context.Background()
	eventID := "evt-publish-bigv"
	if err := fanoutSvc.HandleVideoPublished(ctx, eventID, payload); err != nil {
		t.Fatalf("HandleVideoPublished failed: %v", err)
	}

	// Assert pushed to author outbox
	if len(followingRepo.outboxPushes) != 1 {
		t.Fatalf("expected 1 outbox push call, got %d", len(followingRepo.outboxPushes))
	}
	if followingRepo.outboxPushes[0].authorID != bigVAuthorID || followingRepo.outboxPushes[0].videoID != 888 {
		t.Errorf("unexpected outbox push: %v", followingRepo.outboxPushes[0])
	}

	// Assert inbox pushes was NOT called
	if len(followingRepo.inboxPushes) != 0 {
		t.Errorf("expected 0 inbox pushes for big V author, got %d", len(followingRepo.inboxPushes))
	}
}

func TestFanoutService_DuplicateEvent_Skipped(t *testing.T) {
	authorID := int64(1001)
	relationRepo := &mockRelationRepoForFanout{
		statsMap: map[int64]*domainrelation.RelationStats{
			authorID: {UserID: authorID, FollowerCount: 10},
		},
	}
	followingRepo := &mockFollowingRepoForFanout{}
	dedup := &mockDeduplicatorForFanout{
		consumedMap: map[string]bool{"dup-evt": true}, // already consumed
	}

	fanoutSvc, err := NewFanoutService(relationRepo, followingRepo, dedup)
	if err != nil {
		t.Fatalf("NewFanoutService failed: %v", err)
	}

	payload, _ := json.Marshal(VideoPublishedPayload{
		VideoID:  999,
		AuthorID: authorID,
	})

	if err := fanoutSvc.HandleVideoPublished(context.Background(), "dup-evt", payload); err != nil {
		t.Fatalf("HandleVideoPublished should succeed on duplicate, got: %v", err)
	}

	// No push calls should have occurred
	if len(followingRepo.inboxPushes) != 0 || len(followingRepo.outboxPushes) != 0 {
		t.Errorf("expected no fanout pushes on duplicate event")
	}
}

// A fanout failure must not be recorded as consumed. Marking first is what turned a transient
// Redis error into a silently dropped publish: the redelivery saw the marker and skipped the work.
func TestFanoutService_FanoutFailure_DoesNotMarkConsumed(t *testing.T) {
	authorID := int64(1001)
	relationRepo := &mockRelationRepoForFanout{
		statsMap: map[int64]*domainrelation.RelationStats{
			authorID: {UserID: authorID, FollowerCount: 2},
		},
		followersMap: map[int64][]int64{
			authorID: {2001, 2002},
		},
	}
	followingRepo := &mockFollowingRepoForFanout{pushInboxErr: errors.New("redis is unreachable")}
	dedup := &mockDeduplicatorForFanout{}

	fanoutSvc, err := NewFanoutService(relationRepo, followingRepo, dedup)
	if err != nil {
		t.Fatalf("NewFanoutService failed: %v", err)
	}

	payload, _ := json.Marshal(VideoPublishedPayload{VideoID: 4242, AuthorID: authorID})
	const eventID = "evt-publish-fail-1"
	ctx := context.Background()

	if err := fanoutSvc.HandleVideoPublished(ctx, eventID, payload); err == nil {
		t.Fatalf("expected HandleVideoPublished to report the fanout failure")
	}
	if consumed, _ := dedup.IsConsumed(ctx, eventID); consumed {
		t.Fatalf("event %s was marked consumed even though the fanout failed", eventID)
	}

	// Redelivery once the store recovers actually delivers the video.
	followingRepo.mu.Lock()
	followingRepo.pushInboxErr = nil
	followingRepo.mu.Unlock()

	if err := fanoutSvc.HandleVideoPublished(ctx, eventID, payload); err != nil {
		t.Fatalf("redelivery failed: %v", err)
	}
	if len(followingRepo.inboxPushes) != 1 {
		t.Fatalf("expected the redelivery to fan out once, got %d calls", len(followingRepo.inboxPushes))
	}
	if consumed, _ := dedup.IsConsumed(ctx, eventID); !consumed {
		t.Fatalf("expected the event to be marked consumed after a successful fanout")
	}
}

// A payload that cannot be parsed will never parse on redelivery either, so it must be reported as
// permanent instead of burning the retry budget.
func TestFanoutService_MalformedPayload_IsPermanent(t *testing.T) {
	fanoutSvc, err := NewFanoutService(
		&mockRelationRepoForFanout{},
		&mockFollowingRepoForFanout{},
		&mockDeduplicatorForFanout{},
	)
	if err != nil {
		t.Fatalf("NewFanoutService failed: %v", err)
	}

	err = fanoutSvc.HandleVideoPublished(context.Background(), "evt-bad-payload", []byte(`{"video_id":`))
	if err == nil {
		t.Fatalf("expected an error for a malformed payload")
	}
	if !rabbitmq.IsPermanent(err) {
		t.Fatalf("expected a permanent error for a malformed payload, got %v", err)
	}
}
