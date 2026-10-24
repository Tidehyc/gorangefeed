package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/redis/go-redis/v9"
)

// New initializes a Redis client, configures pool size, and tests connection with fast-fail.
func New(cfg *config.RedisConfig) (*redis.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("redis config is nil")
	}

	opts := &redis.Options{
		Addr:        cfg.Addr(),
		Password:    cfg.Password,
		DB:          cfg.DB,
		PoolSize:    cfg.PoolSize,
		DialTimeout: 2 * time.Second,
		ReadTimeout: 2 * time.Second,
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to ping redis at %s: %w", cfg.Addr(), err)
	}

	return client, nil
}

// Ping checks Redis connectivity.
func Ping(ctx context.Context, rdb *redis.Client) error {
	if rdb == nil {
		return fmt.Errorf("redis client is nil")
	}
	return rdb.Ping(ctx).Err()
}
