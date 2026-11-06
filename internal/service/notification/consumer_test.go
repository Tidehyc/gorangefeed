package notification_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	infrarabbitmq "github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	infraredis "github.com/Tidehyc/gorangefeed/internal/infra/redis"
	servicenotification "github.com/Tidehyc/gorangefeed/internal/service/notification"
)

type mockDeduplicator struct {
	consumed map[string]bool
}

func newMockDeduplicator() *mockDeduplicator {
	return &mockDeduplicator{
		consumed: make(map[string]bool),
	}
}

func (m *mockDeduplicator) MarkConsumed(ctx context.Context, eventID string) (bool, error) {
	if m.consumed[eventID] {
		return false, nil
	}
	m.consumed[eventID] = true
	return true, nil
}

func (m *mockDeduplicator) IsConsumed(ctx context.Context, eventID string) (bool, error) {
	return m.consumed[eventID], nil
}

func (m *mockDeduplicator) PurgeConsumedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return 0, nil
}

func TestConsumer_HandleLikeEvent(t *testing.T) {
	repo := newMockRepo()
	mb := infraredis.NewMemoryBroadcaster()
	defer mb.Close()
	dedup := newMockDeduplicator()

	mockFinder := &mockAccountFinder{
		byID: map[int64]*domainaccount.User{
			200: {ID: 200, Username: "liker_bob", Nickname: "Bob", AvatarURL: "https://avatar.com/bob.jpg"},
		},
	}

	consumer, err := servicenotification.NewConsumer(repo, mb, dedup, mockFinder)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sub, err := mb.Subscribe(ctx)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	t.Run("valid like event creates notification and broadcasts with actor", func(t *testing.T) {
		payload, _ := json.Marshal(servicenotification.LikeEventPayload{
			UserID:   200,
			VideoID:  500,
			AuthorID: 100,
			LikedAt:  time.Now().UTC(),
		})

		err := consumer.HandleEvent(ctx, "ev_like_001", infrarabbitmq.RoutingKeyInteractionLiked, payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repo.items) != 1 {
			t.Fatalf("expected 1 notification in repo, got %d", len(repo.items))
		}

		var created *domainnotification.Notification
		for _, n := range repo.items {
			created = n
		}

		if created.RecipientID != 100 || created.ActorID != 200 || created.Type != domainnotification.TypeLike {
			t.Fatalf("unexpected notification: %+v", created)
		}

		select {
		case broadcastMsg := <-sub:
			if broadcastMsg.Actor == nil || broadcastMsg.Actor.Username != "liker_bob" {
				t.Fatalf("expected hydrated actor liker_bob, got: %v", broadcastMsg.Actor)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for broadcast")
		}
	})

	t.Run("self-like is safely ignored", func(t *testing.T) {
		initialCount := len(repo.items)
		payload, _ := json.Marshal(servicenotification.LikeEventPayload{
			UserID:   100,
			VideoID:  500,
			AuthorID: 100,
		})

		err := consumer.HandleEvent(ctx, "ev_like_002", infrarabbitmq.RoutingKeyInteractionLiked, payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(repo.items) != initialCount {
			t.Fatalf("expected self-like to be skipped, item count changed")
		}
	})
}

func TestConsumer_HandleCommentEvent_WithMentions(t *testing.T) {
	repo := newMockRepo()
	mb := infraredis.NewMemoryBroadcaster()
	defer mb.Close()
	dedup := newMockDeduplicator()

	mockFinder := &mockAccountFinder{
		byUsername: map[string]*domainaccount.User{
			"alice":   {ID: 301, Username: "alice", Nickname: "Alice"},
			"charlie": {ID: 302, Username: "charlie", Nickname: "Charlie"},
		},
		byID: map[int64]*domainaccount.User{
			200: {ID: 200, Username: "commenter_dan", Nickname: "Dan"},
			301: {ID: 301, Username: "alice", Nickname: "Alice"},
			302: {ID: 302, Username: "charlie", Nickname: "Charlie"},
		},
	}

	consumer, err := servicenotification.NewConsumer(repo, mb, dedup, mockFinder)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	ctx := context.Background()

	t.Run("comment with mentions creates comment and mention notifications", func(t *testing.T) {
		payload, _ := json.Marshal(servicenotification.CommentEventPayload{
			UserID:      200,
			VideoID:     500,
			AuthorID:    100,
			Content:     "Great video! Look at this @alice and @charlie!",
			CommentedAt: time.Now().UTC(),
		})

		err := consumer.HandleEvent(ctx, "ev_comment_001", infrarabbitmq.RoutingKeyInteractionCommented, payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Expected 3 notifications: 1 for author (100) + 1 for alice (301) + 1 for charlie (302)
		if len(repo.items) != 3 {
			t.Fatalf("expected 3 notifications, got %d", len(repo.items))
		}

		var hasAuthorComment, hasAliceMention, hasCharlieMention bool
		for _, n := range repo.items {
			if n.RecipientID == 100 && n.Type == domainnotification.TypeComment {
				hasAuthorComment = true
			}
			if n.RecipientID == 301 && n.Type == domainnotification.TypeMention {
				hasAliceMention = true
			}
			if n.RecipientID == 302 && n.Type == domainnotification.TypeMention {
				hasCharlieMention = true
			}
		}

		if !hasAuthorComment || !hasAliceMention || !hasCharlieMention {
			t.Fatalf("missing expected notification types (author: %v, alice: %v, charlie: %v)",
				hasAuthorComment, hasAliceMention, hasCharlieMention)
		}
	})
}

func TestConsumer_HandleFollowEvent(t *testing.T) {
	repo := newMockRepo()
	mb := infraredis.NewMemoryBroadcaster()
	defer mb.Close()
	dedup := newMockDeduplicator()

	mockFinder := &mockAccountFinder{
		byID: map[int64]*domainaccount.User{
			100: {ID: 100, Username: "follower_fan", Nickname: "Fan"},
		},
	}

	consumer, err := servicenotification.NewConsumer(repo, mb, dedup, mockFinder)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	ctx := context.Background()

	payload, _ := json.Marshal(servicenotification.FollowEventPayload{
		FollowerID: 100,
		VloggerID:  200,
		FollowedAt: time.Now().UTC(),
	})

	err = consumer.HandleEvent(ctx, "ev_follow_001", infrarabbitmq.RoutingKeyRelationFollowed, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.items) != 1 {
		t.Fatalf("expected 1 notification in repo, got %d", len(repo.items))
	}

	var created *domainnotification.Notification
	for _, n := range repo.items {
		created = n
	}

	if created.RecipientID != 200 || created.ActorID != 100 || created.Type != domainnotification.TypeFollow {
		t.Fatalf("unexpected follow notification: %+v", created)
	}
}

func TestConsumer_Deduplication(t *testing.T) {
	repo := newMockRepo()
	mb := infraredis.NewMemoryBroadcaster()
	defer mb.Close()
	dedup := newMockDeduplicator()

	consumer, err := servicenotification.NewConsumer(repo, mb, dedup, nil)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	ctx := context.Background()
	payload, _ := json.Marshal(servicenotification.LikeEventPayload{
		UserID:   200,
		VideoID:  500,
		AuthorID: 100,
	})

	// First delivery
	if err := consumer.HandleEvent(ctx, "ev_dup_test", infrarabbitmq.RoutingKeyInteractionLiked, payload); err != nil {
		t.Fatalf("first delivery failed: %v", err)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(repo.items))
	}

	// Replay delivery with same event ID
	if err := consumer.HandleEvent(ctx, "ev_dup_test", infrarabbitmq.RoutingKeyInteractionLiked, payload); err != nil {
		t.Fatalf("replay delivery failed: %v", err)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected still 1 notification after duplicate delivery, got %d", len(repo.items))
	}
}

// A persistence failure must leave the event unmarked. Marking first is what turned a transient
// MySQL error into a notification the recipient never saw: the redelivery skipped it as consumed.
func TestConsumer_ProcessingFailure_DoesNotMarkConsumed(t *testing.T) {
	repo := newMockRepo()
	repo.createErr = errors.New("mysql is unreachable")
	dedup := newMockDeduplicator()

	consumer, err := servicenotification.NewConsumer(repo, nil, dedup, nil)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	ctx := context.Background()
	const eventID = "ev_persist_fail_1"
	payload, _ := json.Marshal(servicenotification.LikeEventPayload{
		UserID:   200,
		VideoID:  500,
		AuthorID: 100,
	})

	if err := consumer.HandleEvent(ctx, eventID, infrarabbitmq.RoutingKeyInteractionLiked, payload); err == nil {
		t.Fatalf("expected HandleEvent to report the persistence failure")
	}
	if consumed, _ := dedup.IsConsumed(ctx, eventID); consumed {
		t.Fatalf("event %s was marked consumed even though nothing was persisted", eventID)
	}

	// Redelivery once MySQL recovers persists the notification.
	repo.createErr = nil
	if err := consumer.HandleEvent(ctx, eventID, infrarabbitmq.RoutingKeyInteractionLiked, payload); err != nil {
		t.Fatalf("redelivery failed: %v", err)
	}
	if len(repo.items) != 1 {
		t.Fatalf("expected the redelivery to persist 1 notification, got %d", len(repo.items))
	}
	if consumed, _ := dedup.IsConsumed(ctx, eventID); !consumed {
		t.Fatalf("expected the event to be marked consumed after a successful persist")
	}
}

// A payload that cannot be parsed will never parse on redelivery either.
func TestConsumer_MalformedPayload_IsPermanent(t *testing.T) {
	dedup := newMockDeduplicator()
	consumer, err := servicenotification.NewConsumer(newMockRepo(), nil, dedup, nil)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	err = consumer.HandleEvent(context.Background(), "ev_bad_payload", infrarabbitmq.RoutingKeyInteractionLiked, []byte(`{"user_id":`))
	if err == nil {
		t.Fatalf("expected an error for a malformed payload")
	}
	if !infrarabbitmq.IsPermanent(err) {
		t.Fatalf("expected a permanent error for a malformed payload, got %v", err)
	}
}
