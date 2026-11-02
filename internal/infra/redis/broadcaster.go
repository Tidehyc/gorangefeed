package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	"github.com/redis/go-redis/v9"
)

// ChannelNotifications is the centralized Redis Pub/Sub topic for cross-instance notification dispatch.
const ChannelNotifications = "channel.gorangefeed.notifications"

// RedisBroadcaster implements domainnotification.Broadcaster using Redis Pub/Sub.
type RedisBroadcaster struct {
	rdb     *redis.Client
	channel string

	mu          sync.RWMutex
	pubsub      *redis.PubSub
	subscribers []chan *domainnotification.Notification
	cancel      context.CancelFunc
	closed      atomic.Bool
	listenOnce  sync.Once
}

// NewRedisBroadcaster creates a new Redis-backed notification broadcaster.
func NewRedisBroadcaster(rdb *redis.Client, channel string) (*RedisBroadcaster, error) {
	if rdb == nil {
		return nil, errors.New("redis client cannot be nil")
	}
	if channel == "" {
		channel = ChannelNotifications
	}
	return &RedisBroadcaster{
		rdb:         rdb,
		channel:     channel,
		subscribers: make([]chan *domainnotification.Notification, 0),
	}, nil
}

// Publish broadcasts a notification message to the Redis channel.
func (b *RedisBroadcaster) Publish(ctx context.Context, notif *domainnotification.Notification) error {
	if b.closed.Load() {
		return errors.New("broadcaster is closed")
	}
	if notif == nil {
		return errors.New("notification cannot be nil")
	}

	payload, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("failed to marshal notification for broadcast: %w", err)
	}

	if err := b.rdb.Publish(ctx, b.channel, payload).Err(); err != nil {
		return fmt.Errorf("failed to publish notification to redis channel %s: %w", b.channel, err)
	}
	return nil
}

// Subscribe subscribes to incoming cluster-wide notifications on this node.
// Multiple callers receive their own delivery channel.
func (b *RedisBroadcaster) Subscribe(ctx context.Context) (<-chan *domainnotification.Notification, error) {
	if b.closed.Load() {
		return nil, errors.New("broadcaster is closed")
	}

	ch := make(chan *domainnotification.Notification, 1024)

	var initErr error
	b.listenOnce.Do(func() {
		subCtx, cancel := context.WithCancel(context.Background())
		b.cancel = cancel

		pubsub := b.rdb.Subscribe(subCtx, b.channel)
		if _, err := pubsub.Receive(subCtx); err != nil {
			initErr = fmt.Errorf("failed to subscribe to redis channel %s: %w", b.channel, err)
			cancel()
			_ = pubsub.Close()
			return
		}
		b.pubsub = pubsub

		go b.listenLoop(subCtx, pubsub)
	})

	if initErr != nil {
		return nil, initErr
	}

	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()

	// Automatically unsubscribe when subscriber context terminates
	go func() {
		select {
		case <-ctx.Done():
			b.removeSubscriber(ch)
		}
	}()

	return ch, nil
}

func (b *RedisBroadcaster) listenLoop(ctx context.Context, pubsub *redis.PubSub) {
	msgCh := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			var notif domainnotification.Notification
			if err := json.Unmarshal([]byte(msg.Payload), &notif); err != nil {
				slog.Warn("failed to unmarshal notification from pubsub", "channel", b.channel, "error", err)
				continue
			}

			b.mu.RLock()
			for _, sub := range b.subscribers {
				select {
				case sub <- &notif:
				default:
					slog.Warn("subscriber channel full, dropping notification",
						"recipient_id", notif.RecipientID, "notification_id", notif.ID)
				}
			}
			b.mu.RUnlock()
		}
	}
}

func (b *RedisBroadcaster) removeSubscriber(target chan *domainnotification.Notification) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, sub := range b.subscribers {
		if sub == target {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			close(target)
			break
		}
	}
}

// Close closes the Redis PubSub connection and terminates all active subscriptions.
func (b *RedisBroadcaster) Close() error {
	if b.closed.Swap(true) {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cancel != nil {
		b.cancel()
	}

	var err error
	if b.pubsub != nil {
		err = b.pubsub.Close()
	}

	for _, sub := range b.subscribers {
		close(sub)
	}
	b.subscribers = nil

	return err
}

// MemoryBroadcaster implements an in-memory thread-safe domainnotification.Broadcaster.
type MemoryBroadcaster struct {
	mu          sync.RWMutex
	subscribers []chan *domainnotification.Notification
	closed      atomic.Bool
}

// NewMemoryBroadcaster creates a new in-memory Broadcaster.
func NewMemoryBroadcaster() *MemoryBroadcaster {
	return &MemoryBroadcaster{
		subscribers: make([]chan *domainnotification.Notification, 0),
	}
}

// Publish distributes the notification to all active in-memory subscriber channels.
func (m *MemoryBroadcaster) Publish(ctx context.Context, notif *domainnotification.Notification) error {
	if m.closed.Load() {
		return errors.New("memory broadcaster is closed")
	}
	if notif == nil {
		return errors.New("notification cannot be nil")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, sub := range m.subscribers {
		select {
		case sub <- notif:
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Non-blocking drop on slow subscriber
		}
	}
	return nil
}

// Subscribe registers a new subscriber channel.
func (m *MemoryBroadcaster) Subscribe(ctx context.Context) (<-chan *domainnotification.Notification, error) {
	if m.closed.Load() {
		return nil, errors.New("memory broadcaster is closed")
	}

	ch := make(chan *domainnotification.Notification, 1024)

	m.mu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.mu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			defer m.mu.Unlock()
			for i, sub := range m.subscribers {
				if sub == ch {
					m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
					close(ch)
					break
				}
			}
		}
	}()

	return ch, nil
}

// Close closes all subscriber channels and marks the broadcaster closed.
func (m *MemoryBroadcaster) Close() error {
	if m.closed.Swap(true) {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sub := range m.subscribers {
		close(sub)
	}
	m.subscribers = nil
	return nil
}
