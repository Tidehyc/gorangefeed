package notification_test

import (
	"context"
	"testing"
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	infraredis "github.com/Tidehyc/gorangefeed/internal/infra/redis"
	servicenotification "github.com/Tidehyc/gorangefeed/internal/service/notification"
)

func TestSSEHub_RegisterAndUnregister(t *testing.T) {
	repo := newMockRepo()
	mb := infraredis.NewMemoryBroadcaster()
	defer mb.Close()

	hub, err := servicenotification.NewSSEHub(repo, mb, nil)
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}
	defer hub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := hub.Start(ctx); err != nil {
		t.Fatalf("failed to start hub: %v", err)
	}

	ch1 := hub.Register(100)
	ch2 := hub.Register(100)
	ch3 := hub.Register(200)

	if hub.ConnectedCount() != 3 {
		t.Fatalf("expected 3 connections, got %d", hub.ConnectedCount())
	}
	if hub.ConnectedUsersCount() != 2 {
		t.Fatalf("expected 2 distinct users, got %d", hub.ConnectedUsersCount())
	}

	hub.Unregister(100, ch1)
	if hub.ConnectedCount() != 2 {
		t.Fatalf("expected 2 connections after unregister, got %d", hub.ConnectedCount())
	}

	// ch1 should be closed
	select {
	case _, ok := <-ch1:
		if ok {
			t.Fatalf("expected unmarshaled ch1 to be closed")
		}
	default:
	}

	hub.Unregister(100, ch2)
	if hub.ConnectedUsersCount() != 1 {
		t.Fatalf("expected 1 user left, got %d", hub.ConnectedUsersCount())
	}

	hub.Unregister(200, ch3)
	if hub.ConnectedCount() != 0 {
		t.Fatalf("expected 0 connections left, got %d", hub.ConnectedCount())
	}
}

func TestSSEHub_TicketLifecycle(t *testing.T) {
	repo := newMockRepo()
	hub, err := servicenotification.NewSSEHub(repo, nil, nil)
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()

	// 1. Create ticket
	ticket, ttl, err := hub.CreateTicket(ctx, 888)
	if err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}
	if ticket == "" || ttl != 60 {
		t.Fatalf("expected ticket with ttl 60, got %s, %d", ticket, ttl)
	}

	// 2. Consume ticket first time -> success
	uid, err := hub.ConsumeTicket(ctx, ticket)
	if err != nil {
		t.Fatalf("failed to consume ticket: %v", err)
	}
	if uid != 888 {
		t.Fatalf("expected uid 888, got %d", uid)
	}

	// 3. Consume ticket second time -> must fail (single-use guarantee)
	_, err = hub.ConsumeTicket(ctx, ticket)
	if err != domainnotification.ErrTicketExpired {
		t.Fatalf("expected ErrTicketExpired on reuse, got %v", err)
	}

	// 4. Empty ticket
	_, err = hub.ConsumeTicket(ctx, "")
	if err != domainnotification.ErrTicketExpired {
		t.Fatalf("expected ErrTicketExpired on empty ticket, got %v", err)
	}
}

func TestSSEHub_BroadcastDelivery(t *testing.T) {
	repo := newMockRepo()
	mb := infraredis.NewMemoryBroadcaster()
	defer mb.Close()

	hub, err := servicenotification.NewSSEHub(repo, mb, nil)
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}
	defer hub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := hub.Start(ctx); err != nil {
		t.Fatalf("failed to start hub: %v", err)
	}

	chRecipient := hub.Register(100)
	defer hub.Unregister(100, chRecipient)

	chOther := hub.Register(200)
	defer hub.Unregister(200, chOther)

	notif := &domainnotification.Notification{
		ID:          999,
		RecipientID: 100,
		ActorID:     300,
		Type:        domainnotification.TypeLike,
		TargetType:  domainnotification.TargetVideo,
		TargetID:    400,
		Title:       "点赞通知",
		Content:     "赞了你的视频",
		CreatedAt:   time.Now().UTC(),
		Actor: &domainnotification.ActorInfo{
			ID:       300,
			Username: "fan_bob",
			Nickname: "Bob",
		},
	}

	if err := mb.Publish(ctx, notif); err != nil {
		t.Fatalf("failed to publish to memory broadcaster: %v", err)
	}

	// Recipient should receive formatted SSE message
	select {
	case msg := <-chRecipient:
		if msg.Event != "notification" {
			t.Fatalf("expected event 'notification', got %s", msg.Event)
		}
		rawBytes := msg.Format()
		if len(rawBytes) == 0 {
			t.Fatalf("expected non-empty wire bytes")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for recipient notification")
	}

	// Non-matching recipient should NOT receive anything
	select {
	case msg := <-chOther:
		t.Fatalf("unexpected message delivered to other user: %v", msg)
	case <-time.After(100 * time.Millisecond):
		// Expected: nothing received
	}
}

func TestSSEHub_CatchUpNotifications(t *testing.T) {
	repo := newMockRepo()
	hub, err := servicenotification.NewSSEHub(repo, nil, nil)
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}
	defer hub.Close()

	ctx := context.Background()

	// Malformed Last-Event-ID should return nil gracefully
	res, err := hub.GetCatchUpNotifications(ctx, 100, "malformed_id")
	if err != nil || res != nil {
		t.Fatalf("expected nil for malformed Last-Event-ID, got %v, %v", res, err)
	}

	// Empty Last-Event-ID
	res, err = hub.GetCatchUpNotifications(ctx, 100, "")
	if err != nil || res != nil {
		t.Fatalf("expected nil for empty Last-Event-ID, got %v, %v", res, err)
	}
}

func TestSSEHub_Drain(t *testing.T) {
	repo := newMockRepo()
	hub, err := servicenotification.NewSSEHub(repo, nil, nil)
	if err != nil {
		t.Fatalf("failed to create hub: %v", err)
	}
	defer hub.Close()

	ch1 := hub.Register(100)
	ch2 := hub.Register(200)

	hub.Drain(context.Background())

	// Both should receive restarting message and be closed
	for i, ch := range []chan *servicenotification.SSEMessage{ch1, ch2} {
		select {
		case msg, ok := <-ch:
			if !ok {
				t.Fatalf("ch %d closed without receiving drain comment", i+1)
			}
			if msg.Comment != "server_restarting" {
				t.Fatalf("expected 'server_restarting' comment, got %s", msg.Comment)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("ch %d timed out waiting for drain comment", i+1)
		}

		// Next read should be closed
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatalf("ch %d expected to be closed after drain message", i+1)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("ch %d timed out waiting for channel close", i+1)
		}
	}

	if hub.ConnectedCount() != 0 {
		t.Fatalf("expected 0 connected after drain, got %d", hub.ConnectedCount())
	}
}
