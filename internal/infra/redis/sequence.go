package redis

import (
	"context"
	"errors"
	"fmt"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	"github.com/redis/go-redis/v9"
)

const DefaultSequencePublishKey = "seq:video:published"

type sequenceGenerator struct {
	rdb redis.Cmdable
	key string
}

// NewSequenceGenerator constructs a SequenceGenerator using Redis INCR.
func NewSequenceGenerator(rdb redis.Cmdable) domaincreator.SequenceGenerator {
	return NewCustomSequenceGenerator(rdb, DefaultSequencePublishKey)
}

// NewCustomSequenceGenerator constructs a SequenceGenerator with a custom key (useful for test isolation).
func NewCustomSequenceGenerator(rdb redis.Cmdable, key string) domaincreator.SequenceGenerator {
	return &sequenceGenerator{
		rdb: rdb,
		key: key,
	}
}

func (s *sequenceGenerator) NextPublishSeq(ctx context.Context) (int64, error) {
	val, err := s.rdb.Incr(ctx, s.key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incr sequence error (%s): %w", s.key, err)
	}
	return val, nil
}

func (s *sequenceGenerator) GetCurrentSeq(ctx context.Context) (int64, error) {
	val, err := s.rdb.Get(ctx, s.key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("redis get sequence error (%s): %w", s.key, err)
	}
	return val, nil
}
