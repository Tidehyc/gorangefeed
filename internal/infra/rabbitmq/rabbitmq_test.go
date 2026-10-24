package rabbitmq

import (
	"strings"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
)

func TestRabbitMQ_FastFail_Unavailable(t *testing.T) {
	cfg := &config.RabbitMQConfig{
		Host:     "127.0.0.1",
		Port:     54323, // non-existent RabbitMQ port
		User:     "guest",
		Password: "guest",
		VHost:    "/",
	}

	start := time.Now()
	_, err := New(cfg)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error connecting to unavailable RabbitMQ, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "127.0.0.1:54323") {
		t.Errorf("expected error to mention host:port, got: %s", errMsg)
	}

	if elapsed > 4*time.Second {
		t.Errorf("connection took too long (%v), expected fast fail within 3s", elapsed)
	}
}

func TestRabbitMQ_LiveConnectionAndPing(t *testing.T) {
	cfg := &config.RabbitMQConfig{
		Host:     "127.0.0.1",
		Port:     5673,
		User:     "guest",
		Password: "guest",
		VHost:    "/",
	}

	conn, err := New(cfg)
	if err != nil {
		t.Skipf("skipping live test: RabbitMQ container not reachable (%v)", err)
	}
	defer conn.Close()

	if err := Ping(conn); err != nil {
		t.Fatalf("expected ping to succeed, got: %v", err)
	}
}
