package migration

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	_ "github.com/go-sql-driver/mysql"
)

func TestMigration_VersionAdvanceAndDirtyManagement(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Skipf("skipping migration test: config load failed (%v)", err)
	}

	dsn := cfg.MySQL.DSN()
	probeDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skip("skipping migration test: unable to connect to MySQL")
	}
	defer probeDB.Close()

	if err := probeDB.Ping(); err != nil {
		t.Skipf("skipping migration test: MySQL not reachable (%v)", err)
	}

	const testTable = "schema_migrations_test"
	cleanup := func() {
		_, _ = probeDB.Exec("DROP TABLE IF EXISTS " + testTable)
	}
	cleanup()
	defer cleanup()

	// Create temporary migration files for this isolated test
	tmpDir := t.TempDir()
	upContent := "SELECT 1;"
	downContent := "SELECT 1;"

	if err := os.WriteFile(filepath.Join(tmpDir, "000001_test_init.up.sql"), []byte(upContent), 0644); err != nil {
		t.Fatalf("failed to create test up migration: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "000001_test_init.down.sql"), []byte(downContent), 0644); err != nil {
		t.Fatalf("failed to create test down migration: %v", err)
	}

	// 1. Force to clean baseline (version 1)
	if err := ForceWithTable(dsn, tmpDir, 1, testTable); err != nil {
		t.Fatalf("force version failed: %v", err)
	}

	ver, dirty, err := VersionWithTable(dsn, tmpDir, testTable)
	if err != nil || ver != 1 || dirty {
		t.Fatalf("expected version 1 dirty=false after force, got ver=%d dirty=%v err=%v", ver, dirty, err)
	}

	// 2. Test RunDown: rolls back step
	ver, dirty, err = RunDownWithTable(dsn, tmpDir, testTable)
	if err != nil {
		t.Fatalf("run down failed: %v", err)
	}
	if ver != 0 || dirty {
		t.Fatalf("expected version 0 dirty=false after down, got ver=%d dirty=%v", ver, dirty)
	}

	// 3. Test RunUp: advances version
	ver, dirty, err = RunUpWithTable(dsn, tmpDir, testTable)
	if err != nil {
		t.Fatalf("run up failed: %v", err)
	}
	if ver != 1 || dirty {
		t.Fatalf("expected version 1 dirty=false after up, got ver=%d dirty=%v", ver, dirty)
	}
}
