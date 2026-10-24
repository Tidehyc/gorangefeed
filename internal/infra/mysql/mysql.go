package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// New initializes a GORM MySQL database connection with connection pool settings and connectivity verification.
func New(cfg *config.MySQLConfig) (*gorm.DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mysql config is nil")
	}

	dsn := cfg.DSN()
	gormConfig := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	}

	db, err := gorm.Open(mysql.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection (%s:%d/%s): %w", cfg.Host, cfg.Port, cfg.DBName, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// Apply connection pool settings
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	// Fast fail check: Ping with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to ping mysql (%s:%d/%s): %w", cfg.Host, cfg.Port, cfg.DBName, err)
	}

	return db, nil
}

// Ping checks the MySQL database health.
func Ping(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB error: %w", err)
	}
	return sqlDB.PingContext(ctx)
}
