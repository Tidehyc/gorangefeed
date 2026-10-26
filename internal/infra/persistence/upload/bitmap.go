package upload

import (
	"context"
	"errors"
	"fmt"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	"github.com/redis/go-redis/v9"
)

// BitmapStore implements domainupload.BitmapStore via Redis Bitmaps for atomic bit operations.
type BitmapStore struct {
	rdb *redis.Client
}

// NewBitmapStore creates a new Redis-backed chunk bitmap store.
func NewBitmapStore(rdb *redis.Client) domainupload.BitmapStore {
	return &BitmapStore{rdb: rdb}
}

func (b *BitmapStore) bitmapKey(uploadID string) string {
	return fmt.Sprintf("chunk_upload_bitmap:%s", uploadID)
}

// SetChunk atomically sets the bit at chunkIndex to 1 and refreshes key TTL.
func (b *BitmapStore) SetChunk(ctx context.Context, uploadID string, chunkIndex int) error {
	key := b.bitmapKey(uploadID)
	pipe := b.rdb.Pipeline()
	pipe.SetBit(ctx, key, int64(chunkIndex), 1)
	pipe.Expire(ctx, key, domainupload.SessionTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// IsChunkUploaded checks if the bit at chunkIndex is 1.
func (b *BitmapStore) IsChunkUploaded(ctx context.Context, uploadID string, chunkIndex int) (bool, error) {
	key := b.bitmapKey(uploadID)
	val, err := b.rdb.GetBit(ctx, key, int64(chunkIndex)).Result()
	if err != nil {
		return false, err
	}
	return val == 1, nil
}

// CountUploadedChunks returns the total number of set bits via BITCOUNT.
func (b *BitmapStore) CountUploadedChunks(ctx context.Context, uploadID string) (int, error) {
	key := b.bitmapKey(uploadID)
	count, err := b.rdb.BitCount(ctx, key, nil).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return int(count), nil
}

// GetUploadedChunks decodes the Redis bitmap bytes into an integer slice of completed chunk indices.
func (b *BitmapStore) GetUploadedChunks(ctx context.Context, uploadID string, totalChunks int) ([]int, error) {
	key := b.bitmapKey(uploadID)
	rawBytes, err := b.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return []int{}, nil
		}
		return nil, err
	}

	indices := make([]int, 0, totalChunks)
	for i := 0; i < totalChunks; i++ {
		byteIndex := i / 8
		bitOffset := 7 - (i % 8)
		if byteIndex < len(rawBytes) && (rawBytes[byteIndex]&(1<<bitOffset)) != 0 {
			indices = append(indices, i)
		}
	}
	return indices, nil
}

// DeleteBitmap cleans up the bitmap key upon session completion or cancellation.
func (b *BitmapStore) DeleteBitmap(ctx context.Context, uploadID string) error {
	return b.rdb.Del(ctx, b.bitmapKey(uploadID)).Err()
}
