package rabbitmq

import (
	"fmt"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

// New initializes an AMQP connection with dial timeout.
// Note: Unlike database/sql or redis, DialConfig performs a synchronous TCP and AMQP handshake (eager connection).
func New(cfg *config.RabbitMQConfig) (*amqp.Connection, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rabbitmq config is nil")
	}

	dialConfig := amqp.Config{
		Dial: amqp.DefaultDial(2 * time.Second),
	}

	conn, err := amqp.DialConfig(cfg.URL(), dialConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect rabbitmq (%s:%d): %w", cfg.Host, cfg.Port, err)
	}

	return conn, nil
}

// Ping checks if the RabbitMQ connection is active (used by readiness probes at runtime).
func Ping(conn *amqp.Connection) error {
	if conn == nil || conn.IsClosed() {
		return fmt.Errorf("rabbitmq connection is closed or nil")
	}

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq channel probe failed: %w", err)
	}
	_ = ch.Close()
	return nil
}
