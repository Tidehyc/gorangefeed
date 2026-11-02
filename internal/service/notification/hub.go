package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	TicketTTLSeconds      = 60
	ClientChannelCapacity = 64
	CatchUpLimit          = 50
	KeepaliveInterval     = 15 * time.Second
)

// luaConsumeTicket atomically retrieves and deletes a short-lived ticket.
var luaConsumeTicket = redis.NewScript(`
local val = redis.call('GET', KEYS[1])
if val then
    redis.call('DEL', KEYS[1])
    return val
else
    return nil
end
`)

// SSEHub manages active Server-Sent Events client streams, cluster-wide fanout,
// ticket exchanges, and reconnection catch-up on a single API node.
type SSEHub struct {
	repo        domainnotification.Repository
	broadcaster domainnotification.Broadcaster
	rdb         *redis.Client
	nodeID      string

	mu      sync.RWMutex
	clients map[int64]map[chan *SSEMessage]struct{}

	draining atomic.Bool
	closed   atomic.Bool
	cancel   context.CancelFunc

	// Fallback in-memory ticket cache for unit test isolation when Redis is not present
	ticketMu      sync.Mutex
	memoryTickets map[string]int64
}

// NewSSEHub creates a new SSEHub instance.
func NewSSEHub(
	repo domainnotification.Repository,
	broadcaster domainnotification.Broadcaster,
	rdb *redis.Client,
) (*SSEHub, error) {
	if repo == nil {
		return nil, errors.New("notification repository cannot be nil")
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "node"
	}
	nodeID := fmt.Sprintf("%s-%s", hostname, uuid.New().String()[:8])

	return &SSEHub{
		repo:          repo,
		broadcaster:   broadcaster,
		rdb:           rdb,
		nodeID:        nodeID,
		clients:       make(map[int64]map[chan *SSEMessage]struct{}),
		memoryTickets: make(map[string]int64),
	}, nil
}

// NodeID returns this hub instance's unique cluster node identifier.
func (h *SSEHub) NodeID() string {
	return h.nodeID
}

// Start begins listening to cluster broadcasts and reporting node heartbeats.
func (h *SSEHub) Start(ctx context.Context) error {
	initMetrics()

	hubCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel

	if h.broadcaster != nil {
		broadcastCh, err := h.broadcaster.Subscribe(hubCtx)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to subscribe to broadcast channel: %w", err)
		}
		go h.listenBroadcastLoop(hubCtx, broadcastCh)
	}

	if h.rdb != nil {
		go h.nodeHeartbeatLoop(hubCtx)
	}

	return nil
}

// CreateTicket issues a 60-second single-use ticket for secure browser SSE handshake.
func (h *SSEHub) CreateTicket(ctx context.Context, userID int64) (string, int, error) {
	if userID <= 0 {
		return "", 0, domainnotification.ErrInvalidRecipientID
	}

	ticket := uuid.New().String()
	key := fmt.Sprintf("sse:ticket:%s", ticket)

	if h.rdb != nil {
		if err := h.rdb.Set(ctx, key, userID, TicketTTLSeconds*time.Second).Err(); err != nil {
			return "", 0, fmt.Errorf("failed to store sse ticket in redis: %w", err)
		}
	} else {
		h.ticketMu.Lock()
		h.memoryTickets[ticket] = userID
		h.ticketMu.Unlock()
	}

	return ticket, TicketTTLSeconds, nil
}

// ConsumeTicket atomically validates and burns the single-use ticket, returning the user ID.
func (h *SSEHub) ConsumeTicket(ctx context.Context, ticket string) (int64, error) {
	if ticket == "" {
		return 0, domainnotification.ErrTicketExpired
	}

	if h.rdb != nil {
		key := fmt.Sprintf("sse:ticket:%s", ticket)
		res, err := luaConsumeTicket.Run(ctx, h.rdb, []string{key}).Result()
		if err != nil || res == nil {
			return 0, domainnotification.ErrTicketExpired
		}

		uidStr, ok := res.(string)
		if !ok {
			return 0, domainnotification.ErrTicketExpired
		}
		uid, err := strconv.ParseInt(uidStr, 10, 64)
		if err != nil || uid <= 0 {
			return 0, domainnotification.ErrTicketExpired
		}
		return uid, nil
	}

	// Fallback in-memory
	h.ticketMu.Lock()
	defer h.ticketMu.Unlock()
	uid, ok := h.memoryTickets[ticket]
	if !ok {
		return 0, domainnotification.ErrTicketExpired
	}
	delete(h.memoryTickets, ticket)
	return uid, nil
}

// Register connects a new client channel for the user.
func (h *SSEHub) Register(userID int64) chan *SSEMessage {
	ch := make(chan *SSEMessage, ClientChannelCapacity)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.draining.Load() || h.closed.Load() {
		close(ch)
		return ch
	}

	if h.clients[userID] == nil {
		h.clients[userID] = make(map[chan *SSEMessage]struct{})
	}
	h.clients[userID][ch] = struct{}{}
	sseConnectedClients.Inc()

	return ch
}

// Unregister disconnects and closes the client channel.
func (h *SSEHub) Unregister(userID int64, ch chan *SSEMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if userClients, ok := h.clients[userID]; ok {
		if _, exists := userClients[ch]; exists {
			delete(userClients, ch)
			close(ch)
			sseConnectedClients.Dec()
		}
		if len(userClients) == 0 {
			delete(h.clients, userID)
		}
	}
}

// ConnectedCount returns the current total number of active client connections on this node.
func (h *SSEHub) ConnectedCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, subs := range h.clients {
		count += len(subs)
	}
	return count
}

// ConnectedUsersCount returns the number of distinct users connected to this node.
func (h *SSEHub) ConnectedUsersCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.clients)
}

// GetCatchUpNotifications queries missed notifications using the 60s overlapping window.
func (h *SSEHub) GetCatchUpNotifications(ctx context.Context, recipientID int64, lastEventID string) ([]*domainnotification.Notification, error) {
	if lastEventID == "" || recipientID <= 0 {
		return nil, nil
	}

	lastTime, lastID, err := domainnotification.ParseSSEEventID(lastEventID)
	if err != nil {
		// Ignore malformed Last-Event-ID safely without aborting stream
		return nil, nil
	}

	// 60-second overlapping window before last received timestamp
	windowStart := lastTime.Add(-60 * time.Second)
	return h.repo.ListAfter(ctx, recipientID, windowStart, lastID, CatchUpLimit)
}

// Drain sends protocol restart comments to all active clients and closes their streams,
// triggering seamless client drift reconnection to peer nodes.
func (h *SSEHub) Drain(ctx context.Context) {
	if h.draining.Swap(true) {
		return
	}

	h.mu.Lock()
	allChannels := make([]chan *SSEMessage, 0)
	for _, userSubs := range h.clients {
		for ch := range userSubs {
			allChannels = append(allChannels, ch)
		}
	}
	h.clients = make(map[int64]map[chan *SSEMessage]struct{})
	h.mu.Unlock()

	restartingMsg := NewRestartingSSEMessage()
	for _, ch := range allChannels {
		select {
		case ch <- restartingMsg:
		default:
		}
		close(ch)
	}

	sseConnectedClients.Set(0)

	if h.rdb != nil {
		_ = h.rdb.HDel(ctx, "sse:nodes", h.nodeID).Err()
	}
}

// Close terminates hub background processing.
func (h *SSEHub) Close() error {
	if h.closed.Swap(true) {
		return nil
	}

	if h.cancel != nil {
		h.cancel()
	}

	h.Drain(context.Background())
	return nil
}

func (h *SSEHub) listenBroadcastLoop(ctx context.Context, broadcastCh <-chan *domainnotification.Notification) {
	for {
		select {
		case <-ctx.Done():
			return
		case notif, ok := <-broadcastCh:
			if !ok {
				return
			}
			sseBroadcastReceivedTotal.Inc()

			h.mu.RLock()
			userSubs, exists := h.clients[notif.RecipientID]
			if !exists || len(userSubs) == 0 {
				h.mu.RUnlock()
				continue
			}

			targets := make([]chan *SSEMessage, 0, len(userSubs))
			for ch := range userSubs {
				targets = append(targets, ch)
			}
			h.mu.RUnlock()

			msg, err := NewNotificationSSEMessage(notif)
			if err != nil {
				slog.Warn("failed to serialize notification to SSE message", "error", err)
				continue
			}

			latency := time.Since(notif.CreatedAt).Seconds()
			if latency > 0 {
				sseDispatchLatency.Observe(latency)
			}

			for _, ch := range targets {
				select {
				case ch <- msg:
					sseBroadcastSentTotal.Inc()
				default:
					slog.Warn("client channel full, dropping SSE message", "recipient_id", notif.RecipientID)
				}
			}
		}
	}
}

func (h *SSEHub) nodeHeartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count := h.ConnectedCount()
			_ = h.rdb.HSet(ctx, "sse:nodes", h.nodeID, count).Err()
			_ = h.rdb.Expire(ctx, "sse:nodes", 30*time.Second).Err()
		}
	}
}
