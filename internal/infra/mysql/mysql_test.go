package mysql

import (
	"strings"
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
)

func TestMySQL_FastFail_Unavailable(t *testing.T) {
	cfg := &config.MySQLConfig{
		Host:            "127.0.0.1",
		Port:            54321, // non-existent MySQL port
		User:            "root",
		Password:        "wrong",
		DBName:          "gorangefeed",
		Params:          "charset=utf8mb4&timeout=1s",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Minute,
	}

	start := time.Now()
	_, err := New(cfg)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error connecting to unavailable MySQL, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "127.0.0.1:54321") {
		t.Errorf("expected error message to contain host and port, got: %s", errMsg)
	}

	if elapsed > 5*time.Second {
		t.Errorf("connection took too long (%v), expected fast fail within 4s", elapsed)
	}
}
