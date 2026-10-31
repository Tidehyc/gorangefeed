package rabbitmq_test

import (
	"context"
	"os"
	"testing"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func setupTestRabbitMQ(t *testing.T) *amqp.Connection {
	t.Helper()

	var configPath string
	for _, candidate := range []string{
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
		t.Skipf("skipping rabbitmq test: config load failed (%v)", err)
		return nil
	}

	conn, err := rabbitmq.New(&cfg.RabbitMQ)
	if err != nil {
		t.Skipf("skipping rabbitmq test: connection failed (%v)", err)
		return nil
	}

	return conn
}

func TestRabbitMQ_PublisherAndConsumer(t *testing.T) {
	conn := setupTestRabbitMQ(t)
	if conn == nil {
		return
	}
	defer conn.Close()

	exchangeName := "test.exchange.gorangefeed.events"
	queueName := "test.queue.gorangefeed.following_feed"

	// 1. Initialize publisher
	publisher, err := rabbitmq.NewPublisher(conn, exchangeName)
	if err != nil {
		t.Fatalf("NewPublisher failed: %v", err)
	}
	defer publisher.Close()

	// 2. Initialize consumer
	consumer, err := rabbitmq.NewConsumer(conn, exchangeName, queueName, []string{rabbitmq.RoutingKeyVideoPublished}, 10)
	if err != nil {
		t.Fatalf("NewConsumer failed: %v", err)
	}

	receivedMsgChan := make(chan string, 1)
	readyChan := make(chan struct{})
	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	defer cancelConsumer()

	go func() {
		_ = consumer.StartWithReady(consumerCtx, readyChan, func(ctx context.Context, d amqp.Delivery) error {
			receivedMsgChan <- d.MessageId
			return nil
		})
	}()

	// Wait deterministically for consumer to be ready
	select {
	case <-readyChan:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for consumer to become ready")
	}

	// 3. Publish test event
	testEvent, err := domainoutbox.NewEvent(rabbitmq.RoutingKeyVideoPublished, map[string]any{
		"video_id":  12345,
		"author_id": 67890,
	})
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pubCancel()

	if err := publisher.Publish(pubCtx, testEvent); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// 4. Assert message delivered and consumed
	select {
	case msgID := <-receivedMsgChan:
		if msgID != testEvent.EventID {
			t.Errorf("expected message ID %s, got %s", testEvent.EventID, msgID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message delivery to consumer")
	}
}
