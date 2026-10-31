package outbox_test

import (
	"context"
	"os"
	"testing"
	"time"

	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceoutbox "github.com/Tidehyc/gorangefeed/internal/infra/persistence/outbox"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) (*gorm.DB, domainoutbox.Repository) {
	t.Helper()

	var configPath string
	for _, candidate := range []string{
		"../../../../configs/config.yaml",
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
		t.Skipf("skipping persistence test: config load failed (%v)", err)
		return nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping persistence test: MySQL connection failed (%v)", err)
		return nil, nil
	}

	repo := persistenceoutbox.NewRepository(db)
	return db, repo
}

func TestGormOutboxRepository_ClaimAndLifecycle(t *testing.T) {
	db, repo := setupTestDB(t)
	if db == nil {
		return
	}
	ctx := context.Background()

	// 1. Save an event in tx
	event, err := domainoutbox.NewEvent("test.event", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("failed to create domain event: %v", err)
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		return repo.SaveInTx(ctx, tx, event)
	})
	if err != nil {
		t.Fatalf("SaveInTx failed: %v", err)
	}

	// 2. Claim lease
	token := uuid.New().String()
	claimed, err := repo.ClaimLease(ctx, token, 30*time.Second, 10)
	if err != nil {
		t.Fatalf("ClaimLease failed: %v", err)
	}
	if len(claimed) == 0 {
		t.Fatalf("expected at least 1 claimed event")
	}

	var found *domainoutbox.Event
	for _, e := range claimed {
		if e.EventID == event.EventID {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatalf("event %s not found in claimed batch", event.EventID)
	}
	if found.Status != domainoutbox.StatusSending {
		t.Errorf("expected status sending, got %s", found.Status)
	}

	// 3. Mark sent
	if err := repo.MarkSent(ctx, event.EventID); err != nil {
		t.Fatalf("MarkSent failed: %v", err)
	}

	// 4. Verify status is sent
	var m persistenceoutbox.OutboxModel
	if err := db.Where("event_id = ?", event.EventID).First(&m).Error; err != nil {
		t.Fatalf("failed to find outbox record: %v", err)
	}
	if m.Status != string(domainoutbox.StatusSent) {
		t.Errorf("expected status sent, got %s", m.Status)
	}
}
