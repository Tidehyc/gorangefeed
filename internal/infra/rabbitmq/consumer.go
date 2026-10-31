package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MessageHandler is the handler callback for an incoming RabbitMQ delivery.
type MessageHandler func(ctx context.Context, d amqp.Delivery) error

// Consumer manages queue consumption and automatic ACK/NACK handling.
type Consumer struct {
	conn          *amqp.Connection
	exchange      string
	queue         string
	routingKeys   []string
	prefetchCount int
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
		conn:          conn,
		exchange:      exchange,
		queue:         queue,
		routingKeys:   routingKeys,
		prefetchCount: prefetchCount,
	}, nil
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
				slog.Warn("rabbitmq message handler failed, rejecting", "queue", q.Name, "msg_id", d.MessageId, "error", err)
				_ = d.Nack(false, true) // requeue for retry
			} else {
				_ = d.Ack(false)
			}
		}
	}
}
