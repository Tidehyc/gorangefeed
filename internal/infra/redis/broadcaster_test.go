package redis_test

import (
	"context"
	"os"
	"testing"
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	infraredis "github.com/Tidehyc/gorangefeed/internal/infra/redis"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func setupTestRedisClient(t *testing.T) *redis.Client {
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
		t.Skipf("skipping redis broadcaster test: config load failed (%v)", err)
		return nil
	}

	rdb, err := infraredis.New(&cfg.Redis)
	if err != nil {
		t.Skipf("skipping redis broadcaster test: Redis connection failed (%v)", err)
		return nil
	}

	return rdb
}

func TestMemoryBroadcaster(t *testing.T) {
	t.Run("publish and receive across multiple subscribers", func(t *testing.T) {
		mb := infraredis.NewMemoryBroadcaster()
		defer mb.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		sub1, err := mb.Subscribe(ctx)
		if err != nil {
			t.Fatalf("sub1 subscribe failed: %v", err)
		}
		sub2, err := mb.Subscribe(ctx)
		if err != nil {
			t.Fatalf("sub2 subscribe failed: %v", err)
		}

		notif := &domainnotification.Notification{
			ID:          1001,
			RecipientID: 2001,
			ActorID:     3001,
			Type:        domainnotification.TypeLike,
			TargetType:  domainnotification.TargetVideo,
			TargetID:    4001,
			Title:       "点赞通知",
			Content:     "赞了你的视频",
			CreatedAt:   time.Now().UTC(),
		}

		if err := mb.Publish(ctx, notif); err != nil {
			t.Fatalf("publish failed: %v", err)
		}

		select {
		case msg := <-sub1:
			if msg.ID != notif.ID || msg.RecipientID != notif.RecipientID {
				t.Fatalf("sub1 received mismatch: %v", msg)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("sub1 timed out waiting for broadcast")
		}

		select {
		case msg := <-sub2:
			if msg.ID != notif.ID || msg.RecipientID != notif.RecipientID {
				t.Fatalf("sub2 received mismatch: %v", msg)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("sub2 timed out waiting for broadcast")
		}
	})

	t.Run("nil notification and closed handling", func(t *testing.T) {
		mb := infraredis.NewMemoryBroadcaster()
		ctx := context.Background()

		if err := mb.Publish(ctx, nil); err == nil {
			t.Fatalf("expected error publishing nil notification")
		}

		if err := mb.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}

		if err := mb.Publish(ctx, &domainnotification.Notification{}); err == nil {
			t.Fatalf("expected error publishing to closed broadcaster")
		}

		if _, err := mb.Subscribe(ctx); err == nil {
			t.Fatalf("expected error subscribing to closed broadcaster")
		}
	})

	t.Run("unsubscribe on context cancellation", func(t *testing.T) {
		mb := infraredis.NewMemoryBroadcaster()
		defer mb.Close()

		ctx, cancel := context.WithCancel(context.Background())
		sub, err := mb.Subscribe(ctx)
		if err != nil {
			t.Fatalf("subscribe failed: %v", err)
		}

		cancel()
		// Wait brief moment for goroutine cleanup
		time.Sleep(50 * time.Millisecond)

		// Channel should be closed
		select {
		case _, ok := <-sub:
			if ok {
				t.Fatalf("expected channel to be closed on context cancellation")
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for subscriber channel close")
		}
	})
}

func TestRedisBroadcaster(t *testing.T) {
	rdb := setupTestRedisClient(t)
	if rdb == nil {
		return
	}
	defer rdb.Close()

	testChannel := "test.channel." + uuid.New().String()

	rb, err := infraredis.NewRedisBroadcaster(rdb, testChannel)
	if err != nil {
		t.Fatalf("failed to create redis broadcaster: %v", err)
	}
	defer rb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub1, err := rb.Subscribe(ctx)
	if err != nil {
		t.Fatalf("failed to subscribe sub1: %v", err)
	}

	sub2, err := rb.Subscribe(ctx)
	if err != nil {
		t.Fatalf("failed to subscribe sub2: %v", err)
	}

	// Wait brief moment for Redis subscription to register
	time.Sleep(100 * time.Millisecond)

	notif := &domainnotification.Notification{
		ID:          9999,
		RecipientID: 8888,
		ActorID:     7777,
		Type:        domainnotification.TypeMention,
		TargetType:  domainnotification.TargetComment,
		TargetID:    6666,
		Title:       "提及通知",
		Content:     "在评论中提及了你: 来看这个！",
		EventID:     "ev_test_123",
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
		Actor: &domainnotification.ActorInfo{
			ID:        7777,
			Username:  "tester7",
			Nickname:  "Tester Seven",
			AvatarURL: "https://example.com/avatar.jpg",
		},
	}

	if err := rb.Publish(ctx, notif); err != nil {
		t.Fatalf("failed to publish to redis: %v", err)
	}

	// Verify both sub1 and sub2 receive the message
	for i, sub := range []<-chan *domainnotification.Notification{sub1, sub2} {
		select {
		case received, ok := <-sub:
			if !ok {
				t.Fatalf("sub %d channel closed prematurely", i+1)
			}
			if received.ID != notif.ID {
				t.Fatalf("sub %d: expected ID %d, got %d", i+1, notif.ID, received.ID)
			}
			if received.RecipientID != notif.RecipientID {
				t.Fatalf("sub %d: expected RecipientID %d, got %d", i+1, notif.RecipientID, received.RecipientID)
			}
			if received.Type != notif.Type {
				t.Fatalf("sub %d: expected Type %s, got %s", i+1, notif.Type, received.Type)
			}
			if received.Actor == nil || received.Actor.Username != "tester7" {
				t.Fatalf("sub %d: expected Actor tester7, got %v", i+1, received.Actor)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("sub %d timed out waiting for redis broadcast", i+1)
		}
	}
}
