package rabbitmq_test

import (
	"context"
	"os"
	"testing"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupDeduplicatorDB(t *testing.T) (*gorm.DB, rabbitmq.Deduplicator) {
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
		t.Skipf("skipping deduplicator test: config load failed (%v)", err)
		return nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping deduplicator test: MySQL connection failed (%v)", err)
		return nil, nil
	}

	return db, rabbitmq.NewDeduplicator(db)
}

func TestDeduplicator_MarkConsumed(t *testing.T) {
	db, dedup := setupDeduplicatorDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()

	eventID := uuid.New().String()
	defer func() {
		db.Exec("DELETE FROM consumed_events WHERE event_id = ?", eventID)
	}()

	// 1. First consumption: should return isNew = true
	isNew, err := dedup.MarkConsumed(ctx, eventID)
	if err != nil {
		t.Fatalf("first MarkConsumed failed: %v", err)
	}
	if !isNew {
		t.Errorf("expected isNew=true on first consumption, got false")
	}

	// 2. IsConsumed should return true
	consumed, err := dedup.IsConsumed(ctx, eventID)
	if err != nil {
		t.Fatalf("IsConsumed failed: %v", err)
	}
	if !consumed {
		t.Errorf("expected consumed=true, got false")
	}

	// 3. Second consumption (duplicate replay): should return isNew = false without error
	isNew, err = dedup.MarkConsumed(ctx, eventID)
	if err != nil {
		t.Fatalf("second MarkConsumed should succeed, got: %v", err)
	}
	if isNew {
		t.Errorf("expected isNew=false on second consumption, got true")
	}
}

func TestDeduplicator_ExecuteWithDeduplication(t *testing.T) {
	db, dedup := setupDeduplicatorDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()

	eventID := uuid.New().String()
	defer func() {
		db.Exec("DELETE FROM consumed_events WHERE event_id = ?", eventID)
	}()

	executedCount := 0

	// 1. First execution: handler should run
	err := dedup.ExecuteWithDeduplication(ctx, eventID, func(tx *gorm.DB) error {
		executedCount++
		return nil
	})
	if err != nil {
		t.Fatalf("first ExecuteWithDeduplication failed: %v", err)
	}
	if executedCount != 1 {
		t.Errorf("expected handler executed once, got %d", executedCount)
	}

	// 2. Second execution with same eventID: duplicate should safely skip handler
	err = dedup.ExecuteWithDeduplication(ctx, eventID, func(tx *gorm.DB) error {
		executedCount++
		return nil
	})
	if err != nil {
		t.Fatalf("second ExecuteWithDeduplication failed: %v", err)
	}
	if executedCount != 1 {
		t.Errorf("expected handler not executed on duplicate, executedCount=%d", executedCount)
	}
}
