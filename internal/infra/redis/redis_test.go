package redis

import (
	"strings"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
)

func TestRedis_FastFail_Unavailable(t *testing.T) {
	cfg := &config.RedisConfig{
		Host:     "127.0.0.1",
		Port:     54322, // non-existent Redis port
		Password: "",
		DB:       0,
		PoolSize: 10,
	}

	start := time.Now()
	_, err := New(cfg)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error connecting to unavailable Redis, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "127.0.0.1:54322") {
		t.Errorf("expected error to mention address, got: %s", errMsg)
	}

	if elapsed > 4*time.Second {
		t.Errorf("connection took too long (%v), expected fast fail within 3s", elapsed)
	}
}
