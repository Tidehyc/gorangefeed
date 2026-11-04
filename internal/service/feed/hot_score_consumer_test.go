package feed

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
)

type mockHotStoreForConsumer struct {
	mu     sync.Mutex
	scores map[int64]float64
	calls  []scoreCall
}

type scoreCall struct {
	videoID    int64
	delta      float64
	occurredAt time.Time
}

func (m *mockHotStoreForConsumer) IncrScore(ctx context.Context, videoID int64, delta float64, occurredAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.scores == nil {
		m.scores = make(map[int64]float64)
	}
	m.scores[videoID] += delta
	m.calls = append(m.calls, scoreCall{videoID: videoID, delta: delta, occurredAt: occurredAt})
	return nil
}

func (m *mockHotStoreForConsumer) MergeWindow(ctx context.Context, asOf time.Time, windowMinutes int) (string, error) {
	return "test_window", nil
}

func (m *mockHotStoreForConsumer) ListBySnapshot(ctx context.Context, asOf time.Time, offset int, limit int) ([]int64, error) {
	return nil, nil
}

func (m *mockHotStoreForConsumer) GetSnapshotTotal(ctx context.Context, asOf time.Time) (int64, error) {
	return 0, nil
}

func TestHotScoreConsumer_HandleEngagementEvent_Scoring(t *testing.T) {
	mockStore := &mockHotStoreForConsumer{}
	mockDedup := &mockDeduplicatorForFanout{}

	consumer, err := NewHotScoreConsumer(mockStore, mockDedup)
	if err != nil {
		t.Fatalf("failed to create HotScoreConsumer: %v", err)
	}

	ctx := context.Background()
	videoID := int64(1001)

	// 1. Like event: +3.0
	likePayload, _ := json.Marshal(EngagementPayload{VideoID: videoID})
	err = consumer.HandleEngagementEvent(ctx, "ev_like_1", rabbitmq.RoutingKeyInteractionLiked, likePayload)
	if err != nil {
		t.Fatalf("HandleEngagementEvent like failed: %v", err)
	}
	if mockStore.scores[videoID] != 3.0 {
		t.Fatalf("expected score 3.0 after like, got %f", mockStore.scores[videoID])
	}

	// 2. Comment event: +5.0 (Total: 8.0)
	commentPayload, _ := json.Marshal(EngagementPayload{VideoID: videoID})
	err = consumer.HandleEngagementEvent(ctx, "ev_comment_1", rabbitmq.RoutingKeyInteractionCommented, commentPayload)
	if err != nil {
		t.Fatalf("HandleEngagementEvent comment failed: %v", err)
	}
	if mockStore.scores[videoID] != 8.0 {
		t.Fatalf("expected score 8.0 after comment, got %f", mockStore.scores[videoID])
	}

	// 3. Complete event: +2.0 (Total: 10.0)
	completePayload, _ := json.Marshal(EngagementPayload{VideoID: videoID})
	err = consumer.HandleEngagementEvent(ctx, "ev_complete_1", rabbitmq.RoutingKeyVideoCompleted, completePayload)
	if err != nil {
		t.Fatalf("HandleEngagementEvent complete failed: %v", err)
	}
	if mockStore.scores[videoID] != 10.0 {
		t.Fatalf("expected score 10.0 after complete, got %f", mockStore.scores[videoID])
	}

	// 4. Play event: +0.5 (Total: 10.5)
	playPayload, _ := json.Marshal(EngagementPayload{VideoID: videoID})
	err = consumer.HandleEngagementEvent(ctx, "ev_play_1", rabbitmq.RoutingKeyVideoViewed, playPayload)
	if err != nil {
		t.Fatalf("HandleEngagementEvent play failed: %v", err)
	}
	if mockStore.scores[videoID] != 10.5 {
		t.Fatalf("expected score 10.5 after play, got %f", mockStore.scores[videoID])
	}

	// 5. Unlike event: -3.0 (Total: 7.5)
	unlikePayload, _ := json.Marshal(EngagementPayload{VideoID: videoID})
	err = consumer.HandleEngagementEvent(ctx, "ev_unlike_1", rabbitmq.RoutingKeyInteractionUnliked, unlikePayload)
	if err != nil {
		t.Fatalf("HandleEngagementEvent unlike failed: %v", err)
	}
	if mockStore.scores[videoID] != 7.5 {
		t.Fatalf("expected score 7.5 after unlike, got %f", mockStore.scores[videoID])
	}

	// 6. Delete comment: -5.0 (Total: 2.5)
	delCommentPayload, _ := json.Marshal(EngagementPayload{VideoID: videoID})
	err = consumer.HandleEngagementEvent(ctx, "ev_del_comment_1", rabbitmq.RoutingKeyInteractionCommentDelete, delCommentPayload)
	if err != nil {
		t.Fatalf("HandleEngagementEvent del comment failed: %v", err)
	}
	if mockStore.scores[videoID] != 2.5 {
		t.Fatalf("expected score 2.5 after delete comment, got %f", mockStore.scores[videoID])
	}

	// 7. Favorite event: +9.0 (3x like weight, Total: 11.5)
	favPayload, _ := json.Marshal(EngagementPayload{VideoID: videoID})
	err = consumer.HandleEngagementEvent(ctx, "ev_fav_1", rabbitmq.RoutingKeyInteractionFavorited, favPayload)
	if err != nil {
		t.Fatalf("HandleEngagementEvent favorited failed: %v", err)
	}
	if mockStore.scores[videoID] != 11.5 {
		t.Fatalf("expected score 11.5 after favorite, got %f", mockStore.scores[videoID])
	}

	// 8. Unfavorite event: -9.0 (Total: 2.5)
	unfavPayload, _ := json.Marshal(EngagementPayload{VideoID: videoID})
	err = consumer.HandleEngagementEvent(ctx, "ev_unfav_1", rabbitmq.RoutingKeyInteractionUnfavorited, unfavPayload)
	if err != nil {
		t.Fatalf("HandleEngagementEvent unfavorited failed: %v", err)
	}
	if mockStore.scores[videoID] != 2.5 {
		t.Fatalf("expected score 2.5 after unfavorite, got %f", mockStore.scores[videoID])
	}
}

func TestHotScoreConsumer_Deduplication(t *testing.T) {
	mockStore := &mockHotStoreForConsumer{}
	mockDedup := &mockDeduplicatorForFanout{}

	consumer, err := NewHotScoreConsumer(mockStore, mockDedup)
	if err != nil {
		t.Fatalf("failed to create HotScoreConsumer: %v", err)
	}

	ctx := context.Background()
	videoID := int64(2002)
	payload, _ := json.Marshal(EngagementPayload{VideoID: videoID})

	// First delivery: should increment +3.0
	err = consumer.HandleEngagementEvent(ctx, "ev_dup_1", rabbitmq.RoutingKeyInteractionLiked, payload)
	if err != nil {
		t.Fatalf("first delivery failed: %v", err)
	}
	if mockStore.scores[videoID] != 3.0 {
		t.Fatalf("expected score 3.0, got %f", mockStore.scores[videoID])
	}

	// Replay duplicate delivery: should be skipped idempotently
	err = consumer.HandleEngagementEvent(ctx, "ev_dup_1", rabbitmq.RoutingKeyInteractionLiked, payload)
	if err != nil {
		t.Fatalf("second duplicate delivery failed: %v", err)
	}
	// Score must remain 3.0, call count must remain 1
	if mockStore.scores[videoID] != 3.0 {
		t.Fatalf("expected score to remain 3.0 after duplicate replay, got %f", mockStore.scores[videoID])
	}
	if len(mockStore.calls) != 1 {
		t.Fatalf("expected exactly 1 call to IncrScore, got %d", len(mockStore.calls))
	}
}

func TestHotScoreConsumer_ValidationAndEdgeCases(t *testing.T) {
	mockStore := &mockHotStoreForConsumer{}
	mockDedup := &mockDeduplicatorForFanout{}

	// Nil dependencies
	_, err := NewHotScoreConsumer(nil, mockDedup)
	if err == nil {
		t.Fatalf("expected error for nil hotStore")
	}
	_, err = NewHotScoreConsumer(mockStore, nil)
	if err == nil {
		t.Fatalf("expected error for nil deduplicator")
	}

	consumer, _ := NewHotScoreConsumer(mockStore, mockDedup)
	ctx := context.Background()

	// Empty event ID
	err = consumer.HandleEngagementEvent(ctx, "", rabbitmq.RoutingKeyInteractionLiked, []byte(`{}`))
	if err == nil {
		t.Fatalf("expected error for empty event ID")
	}

	// Zero video ID payload should be safely skipped
	err = consumer.HandleEngagementEvent(ctx, "ev_zero_vid", rabbitmq.RoutingKeyInteractionLiked, []byte(`{"video_id":0}`))
	if err != nil {
		t.Fatalf("expected nil for zero video ID payload, got %v", err)
	}
	if len(mockStore.calls) != 0 {
		t.Fatalf("expected 0 IncrScore calls, got %d", len(mockStore.calls))
	}

	// Unknown routing key should be safely skipped
	err = consumer.HandleEngagementEvent(ctx, "ev_unknown_rk", "unknown.routing.key", []byte(`{"video_id":123}`))
	if err != nil {
		t.Fatalf("expected nil for unknown routing key, got %v", err)
	}
}
