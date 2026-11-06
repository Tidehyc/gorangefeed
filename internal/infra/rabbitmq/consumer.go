package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// defaultMaxDeliveryAttempts is how many times a delivery is retried before it is moved to the
// dead-letter queue. Without a bound, a message whose handler keeps failing is requeued forever
// and spins the consumer in a tight loop.
const defaultMaxDeliveryAttempts = 5

// attemptKeyTTL discards retry counters for deliveries that never come back.
const attemptKeyTTL = time.Hour

// MessageHandler is the handler callback for an incoming RabbitMQ delivery.
type MessageHandler func(ctx context.Context, d amqp.Delivery) error

// PermanentError marks a handler failure that a redelivery cannot fix (malformed payload, unknown
// routing key, ...). Such deliveries go straight to the dead-letter queue instead of burning
// through the retry budget.
type PermanentError struct {
	err error
}

// Permanent wraps err as a non-retryable handler failure.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{err: err}
}

func (e *PermanentError) Error() string { return e.err.Error() }

func (e *PermanentError) Unwrap() error { return e.err }

// IsPermanent reports whether err was wrapped by Permanent.
func IsPermanent(err error) bool {
	var target *PermanentError
	return errors.As(err, &target)
}

// AttemptTracker counts redeliveries of the same message so the consumer can bound retries.
type AttemptTracker interface {
	// Increment records one more attempt for key and returns the new count.
	Increment(ctx context.Context, key string) (int, error)
	// Reset forgets the attempts recorded for key.
	Reset(ctx context.Context, key string) error
}

type memoryAttemptTracker struct {
	mu        sync.Mutex
	entries   map[string]attemptEntry
	lastSweep time.Time
}

// attemptSweepInterval bounds how often expired counters are collected. Sweeping on every
// increment would make a systematic failure (every delivery retried) quadratic in the number of
// distinct keys in flight.
const attemptSweepInterval = time.Minute

type attemptEntry struct {
	count   int
	expires time.Time
}

func newMemoryAttemptTracker() *memoryAttemptTracker {
	return &memoryAttemptTracker{entries: make(map[string]attemptEntry)}
}

func (t *memoryAttemptTracker) Increment(_ context.Context, key string) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	if now.Sub(t.lastSweep) >= attemptSweepInterval {
		for k, e := range t.entries {
			if now.After(e.expires) {
				delete(t.entries, k)
			}
		}
		t.lastSweep = now
	}

	e := t.entries[key]
	if now.After(e.expires) {
		e = attemptEntry{}
	}
	e.count++
	e.expires = now.Add(attemptKeyTTL)
	t.entries[key] = e

	return e.count, nil
}

func (t *memoryAttemptTracker) Reset(_ context.Context, key string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
	return nil
}

// Consumer manages queue consumption and automatic ACK/NACK handling.
type Consumer struct {
	conn            *amqp.Connection
	exchange        string
	queue           string
	routingKeys     []string
	prefetchCount   int
	maxAttempts     int
	attempts        AttemptTracker
	deadLetterQueue string
}

// NewConsumer creates a new RabbitMQ consumer instance.
func NewConsumer(conn *amqp.Connection, exchange, queue string, routingKeys []string, prefetchCount int) (*Consumer, error) {
	if conn == nil || conn.IsClosed() {
		return nil, errors.New("rabbitmq connection is closed or nil")
	}
	if exchange == "" {
		exchange = ExchangeEvents
	}
	if queue == "" {
		return nil, errors.New("queue name cannot be empty")
	}
	if prefetchCount <= 0 {
		prefetchCount = 20
	}

	return &Consumer{
		conn:            conn,
		exchange:        exchange,
		queue:           queue,
		routingKeys:     routingKeys,
		prefetchCount:   prefetchCount,
		maxAttempts:     defaultMaxDeliveryAttempts,
		attempts:        newMemoryAttemptTracker(),
		deadLetterQueue: queue + ".dead",
	}, nil
}

// WithRetryPolicy overrides the default retry budget and attempt tracker. A nil tracker keeps the
// in-process default; maxAttempts <= 0 keeps the built-in bound.
func (c *Consumer) WithRetryPolicy(tracker AttemptTracker, maxAttempts int) *Consumer {
	if tracker != nil {
		c.attempts = tracker
	}
	if maxAttempts > 0 {
		c.maxAttempts = maxAttempts
	}
	return c
}

// Start declares the queue, binds the routing keys, and consumes deliveries until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context, handler MessageHandler) error {
	return c.StartWithReady(ctx, nil, handler)
}

// StartWithReady declares the queue, binds routing keys, closes ready channel when listening,
// and consumes deliveries until ctx is cancelled.
func (c *Consumer) StartWithReady(ctx context.Context, ready chan<- struct{}, handler MessageHandler) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open consumer channel: %w", err)
	}
	defer ch.Close()

	// Set QoS prefetch count
	if err := ch.Qos(c.prefetchCount, 0, false); err != nil {
		return fmt.Errorf("failed to set consumer QoS: %w", err)
	}

	// Declare durable exchange
	if err := ch.ExchangeDeclare(
		c.exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare durable queue
	q, err := ch.QueueDeclare(
		c.queue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", c.queue, err)
	}

	// Dead-letter queue for deliveries that exhausted their retries or failed permanently.
	// It is declared with plain arguments (no x-dead-letter-* on the main queue) so that adding
	// this behaviour does not change the main queue's declaration arguments, which would make an
	// already-declared durable queue fail with PRECONDITION_FAILED.
	if _, err := ch.QueueDeclare(
		c.deadLetterQueue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to declare dead-letter queue %s: %w", c.deadLetterQueue, err)
	}

	// Bind queue to all requested routing keys
	for _, rk := range c.routingKeys {
		if err := ch.QueueBind(q.Name, rk, c.exchange, false, nil); err != nil {
			return fmt.Errorf("failed to bind queue %s to routing key %s: %w", q.Name, rk, err)
		}
	}

	deliveries, err := ch.Consume(
		q.Name,
		"",    // consumer tag
		false, // auto-ack (manual ACK enabled)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming from queue %s: %w", q.Name, err)
	}

	slog.Info("rabbitmq consumer started", "queue", q.Name, "routing_keys", c.routingKeys)

	if ready != nil {
		close(ready)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("rabbitmq consumer stopping on context done", "queue", q.Name)
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq consumer deliveries channel closed")
			}

			if err := handler(ctx, d); err != nil {
				c.handleFailure(ctx, ch, d, err)
			} else {
				c.attempts.Reset(ctx, c.attemptKey(d))
				_ = d.Ack(false)
			}
		}
	}
}

// attemptKey identifies one logical message across redeliveries. MessageId is preferred; when the
// publisher did not set one, the body hash keeps the counter stable across redeliveries anyway.
func (c *Consumer) attemptKey(d amqp.Delivery) string {
	id := d.MessageId
	if id == "" {
		h := fnv.New64a()
		_, _ = h.Write(d.Body)
		id = strconv.FormatUint(h.Sum64(), 16)
	}
	return c.queue + ":" + id
}

// handleFailure decides between retrying a delivery and dead-lettering it. The retry budget is
// consumed before the decision so that a permanently broken message cannot loop forever.
func (c *Consumer) handleFailure(ctx context.Context, ch *amqp.Channel, d amqp.Delivery, handlerErr error) {
	attempts := 0
	if c.attempts != nil {
		if n, err := c.attempts.Increment(ctx, c.attemptKey(d)); err == nil {
			attempts = n
		}
	}

	permanent := IsPermanent(handlerErr)
	if c.shouldRequeue(attempts, permanent) {
		slog.Warn("rabbitmq message handler failed, requeueing",
			"queue", c.queue,
			"msg_id", d.MessageId,
			"attempt", attempts,
			"max_attempts", c.maxAttempts,
			"error", handlerErr,
		)
		_ = d.Nack(false, true)
		return
	}

	reason := "retry budget exhausted"
	if permanent {
		reason = "permanent failure"
	}

	if err := c.publishDeadLetter(ctx, ch, d, attempts, handlerErr); err != nil {
		// Dead-lettering failed: keep the message alive rather than dropping it silently.
		slog.Error("rabbitmq failed to dead-letter message, requeueing instead",
			"queue", c.queue,
			"dead_letter_queue", c.deadLetterQueue,
			"msg_id", d.MessageId,
			"error", err,
		)
		_ = d.Nack(false, true)
		return
	}

	c.attempts.Reset(ctx, c.attemptKey(d))

	slog.Error("rabbitmq message moved to dead-letter queue",
		"queue", c.queue,
		"dead_letter_queue", c.deadLetterQueue,
		"msg_id", d.MessageId,
		"routing_key", d.RoutingKey,
		"attempts", attempts,
		"reason", reason,
		"error", handlerErr,
	)

	_ = d.Ack(false)
}

// shouldRequeue reports whether a failed delivery gets another attempt. Permanent failures never
// do; everything else is bounded by the retry budget so a poison message cannot spin forever.
func (c *Consumer) shouldRequeue(attempts int, permanent bool) bool {
	if permanent {
		return false
	}
	return attempts < c.maxAttempts
}

// publishDeadLetter republishes the original body, key and headers to <queue>.dead, adding the
// reason and attempt count so the message can be replayed or inspected later.
func (c *Consumer) publishDeadLetter(ctx context.Context, ch *amqp.Channel, d amqp.Delivery, attempts int, handlerErr error) error {
	headers := amqp.Table{}
	for k, v := range d.Headers {
		headers[k] = v
	}
	headers["x-original-queue"] = c.queue
	headers["x-original-routing-key"] = d.RoutingKey
	headers["x-attempts"] = attempts
	if handlerErr != nil {
		headers["x-last-error"] = handlerErr.Error()
	}

	return ch.PublishWithContext(ctx,
		"",                // default exchange: route straight to the queue by name
		c.deadLetterQueue, // routing key
		false,             // mandatory
		false,             // immediate
		amqp.Publishing{
			ContentType:  d.ContentType,
			DeliveryMode: amqp.Persistent,
			MessageId:    d.MessageId,
			Timestamp:    time.Now(),
			Type:         d.Type,
			Headers:      headers,
			Body:         d.Body,
		},
	)
}
