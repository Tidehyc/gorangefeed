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

// DefaultMigrationsTable is the default schema migrations table name.
const DefaultMigrationsTable = "schema_migrations"

// RunUp executes all pending migrations up to the latest version.
func RunUp(dsn string, migrationsPath string) (uint, bool, error) {
	return RunUpWithTable(dsn, migrationsPath, DefaultMigrationsTable)
}

// RunUpWithTable executes all pending migrations up to the latest version using the specified table.
func RunUpWithTable(dsn string, migrationsPath string, table string) (uint, bool, error) {
	m, err := newMigrate(dsn, migrationsPath, table)
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
	return RunDownWithTable(dsn, migrationsPath, DefaultMigrationsTable)
}

// RunDownWithTable rolls back one migration step using the specified table.
func RunDownWithTable(dsn string, migrationsPath string, table string) (uint, bool, error) {
	m, err := newMigrate(dsn, migrationsPath, table)
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
	return VersionWithTable(dsn, migrationsPath, DefaultMigrationsTable)
}

// VersionWithTable returns the current migration version and dirty status using the specified table.
func VersionWithTable(dsn string, migrationsPath string, table string) (uint, bool, error) {
	m, err := newMigrate(dsn, migrationsPath, table)
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
	return ForceWithTable(dsn, migrationsPath, version, DefaultMigrationsTable)
}

// ForceWithTable sets the migration version and clears dirty status using the specified table.
func ForceWithTable(dsn string, migrationsPath string, version int, table string) error {
	m, err := newMigrate(dsn, migrationsPath, table)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Force(version); err != nil {
		return fmt.Errorf("failed to force migration version %d: %w", version, err)
	}
	return nil
}

func newMigrate(dsn string, migrationsPath string, table string) (*migrate.Migrate, error) {
	if table == "" {
		table = DefaultMigrationsTable
	}

	// golang-migrate requires multiStatements=true to execute multi-statement SQL migration files
	if !strings.Contains(dsn, "multiStatements=") {
		if strings.Contains(dsn, "?") {
			dsn += "&multiStatements=true"
		} else {
			dsn += "?multiStatements=true"
		}
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql: %w", err)
	}

	driver, err := mysql.WithInstance(db, &mysql.Config{
		MigrationsTable: table,
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
