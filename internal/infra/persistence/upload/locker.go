package upload

import (
	"context"
	"fmt"
	"time"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Lua script to safely release distributed lock only if the held token matches.
var unlockScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`)

// MergeLocker implements domainupload.MergeLocker using Redis SetNX and Lua script unlock.
type MergeLocker struct {
	rdb *redis.Client
}

// NewMergeLocker creates a Redis-backed distributed merge locker.
func NewMergeLocker(rdb *redis.Client) domainupload.MergeLocker {
	return &MergeLocker{rdb: rdb}
}

func (l *MergeLocker) lockKey(uploadID string) string {
	return fmt.Sprintf("chunk_merge_lock:%s", uploadID)
}

// TryLock attempts to acquire a mutex lock for uploadID with TTL.
func (l *MergeLocker) TryLock(ctx context.Context, uploadID string, ttl time.Duration) (func() error, error) {
	token := uuid.NewString()
	key := l.lockKey(uploadID)

	ok, err := l.rdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domainupload.ErrMergeConflict
	}

	unlock := func() error {
		return unlockScript.Run(context.Background(), l.rdb, []string{key}, token).Err()
	}
	return unlock, nil
}
