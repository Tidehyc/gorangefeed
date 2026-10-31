package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Publisher handles reliable message publishing to RabbitMQ with publisher confirms.
type Publisher struct {
	conn     *amqp.Connection
	exchange string
	channel  *amqp.Channel
	confirms chan amqp.Confirmation
	mu       sync.Mutex
	timeout  time.Duration
}

// NewPublisher initializes a RabbitMQ publisher and declares the topic exchange.
func NewPublisher(conn *amqp.Connection, exchange string) (*Publisher, error) {
	if conn == nil || conn.IsClosed() {
		return nil, errors.New("rabbitmq connection is closed or nil")
	}
	if exchange == "" {
		exchange = ExchangeEvents
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open rabbitmq channel: %w", err)
	}

	// Declare durable topic exchange
	if err := ch.ExchangeDeclare(
		exchange,
		"topic",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // arguments
	); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("failed to declare exchange %s: %w", exchange, err)
	}

	// Put channel into confirm mode
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("failed to put channel into confirm mode: %w", err)
	}

	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))

	return &Publisher{
		conn:     conn,
		exchange: exchange,
		channel:  ch,
		confirms: confirms,
		timeout:  5 * time.Second,
	}, nil
}

// Publish sends an outbox event to RabbitMQ using its EventType as the routing key,
// waiting for Publisher Confirm ACK to guarantee persistence.
func (p *Publisher) Publish(ctx context.Context, event *domainoutbox.Event) error {
	if event == nil {
		return errors.New("cannot publish nil event")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.channel == nil || p.channel.IsClosed() {
		return errors.New("rabbitmq publisher channel is closed")
	}

	routingKey := event.EventType
	msg := amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		MessageId:    event.EventID,
		Timestamp:    event.CreatedAt,
		Body:         event.Payload,
	}

	publishCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	if err := p.channel.PublishWithContext(
		publishCtx,
		p.exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		msg,
	); err != nil {
		return fmt.Errorf("failed to publish event %s: %w", event.EventID, err)
	}

	select {
	case confirmed, ok := <-p.confirms:
		if !ok {
			return errors.New("publisher confirm channel closed")
		}
		if !confirmed.Ack {
			return fmt.Errorf("rabbitmq nack received for event %s", event.EventID)
		}
		return nil
	case <-publishCtx.Done():
		return fmt.Errorf("timeout or cancelled waiting for rabbitmq confirm of event %s: %w", event.EventID, publishCtx.Err())
	}
}

// Close closes the underlying AMQP channel.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.channel != nil && !p.channel.IsClosed() {
		return p.channel.Close()
	}
	return nil
}
