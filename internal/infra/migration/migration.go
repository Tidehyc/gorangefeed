package migration

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunUp executes all pending migrations up to the latest version.
func RunUp(dsn string, migrationsPath string) (uint, bool, error) {
	m, err := newMigrate(dsn, migrationsPath)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, false, fmt.Errorf("migration up failed: %w", err)
	}

	ver, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}
	return ver, dirty, nil
}

// RunDown rolls back one migration step.
func RunDown(dsn string, migrationsPath string) (uint, bool, error) {
	m, err := newMigrate(dsn, migrationsPath)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()

	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, false, fmt.Errorf("migration down failed: %w", err)
	}

	ver, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}
	return ver, dirty, nil
}

// Version returns the current migration version and dirty status.
func Version(dsn string, migrationsPath string) (uint, bool, error) {
	m, err := newMigrate(dsn, migrationsPath)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()

	ver, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}
	return ver, dirty, nil
}

// Force sets the migration version and clears dirty status.
func Force(dsn string, migrationsPath string, version int) error {
	m, err := newMigrate(dsn, migrationsPath)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Force(version); err != nil {
		return fmt.Errorf("failed to force migration version %d: %w", version, err)
	}
	return nil
}

func newMigrate(dsn string, migrationsPath string) (*migrate.Migrate, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql: %w", err)
	}

	driver, err := mysql.WithInstance(db, &mysql.Config{
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create mysql migrate driver: %w", err)
	}

	sourceURL := migrationsPath
	if !strings.HasPrefix(sourceURL, "file://") {
		sourceURL = "file://" + sourceURL
	}

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "mysql", driver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize migrate instance: %w", err)
	}

	return m, nil
}
