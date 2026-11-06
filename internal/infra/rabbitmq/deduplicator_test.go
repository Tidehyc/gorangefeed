package rabbitmq_test

import (
	"context"
	"os"
	"testing"
	"time"

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

func TestDeduplicator_PurgeConsumedBefore(t *testing.T) {
	db, dedup := setupDeduplicatorDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()

	oldEventID := uuid.New().String()
	freshEventID := uuid.New().String()
	defer func() {
		db.Exec("DELETE FROM consumed_events WHERE event_id IN (?, ?)", oldEventID, freshEventID)
	}()

	if _, err := dedup.MarkConsumed(ctx, oldEventID); err != nil {
		t.Fatalf("MarkConsumed(old) failed: %v", err)
	}
	if _, err := dedup.MarkConsumed(ctx, freshEventID); err != nil {
		t.Fatalf("MarkConsumed(fresh) failed: %v", err)
	}

	// Age one marker past the retention cutoff, leave the other alone.
	if err := db.Exec(
		"UPDATE consumed_events SET consumed_at = ? WHERE event_id = ?",
		time.Now().Add(-30*24*time.Hour), oldEventID,
	).Error; err != nil {
		t.Fatalf("failed to backdate marker: %v", err)
	}

	removed, err := dedup.PurgeConsumedBefore(ctx, time.Now().Add(-7*24*time.Hour))
	if err != nil {
		t.Fatalf("PurgeConsumedBefore failed: %v", err)
	}
	if removed < 1 {
		t.Fatalf("expected at least the backdated marker to be purged, removed=%d", removed)
	}

	oldConsumed, err := dedup.IsConsumed(ctx, oldEventID)
	if err != nil {
		t.Fatalf("IsConsumed(old) failed: %v", err)
	}
	if oldConsumed {
		t.Errorf("expected the old marker to be purged")
	}

	freshConsumed, err := dedup.IsConsumed(ctx, freshEventID)
	if err != nil {
		t.Fatalf("IsConsumed(fresh) failed: %v", err)
	}
	if !freshConsumed {
		t.Errorf("expected the fresh marker to survive the purge")
	}
}
