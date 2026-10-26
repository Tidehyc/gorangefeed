package upload

import (
	"context"
	"time"
)

// SessionRepository defines persistence contracts for upload sessions in authoritative storage (MySQL).
type SessionRepository interface {
	// Create persists a new upload session record.
	Create(ctx context.Context, session *UploadSession) error

	// FindByUploadID retrieves an upload session by its unique UUID.
	FindByUploadID(ctx context.Context, uploadID string) (*UploadSession, error)

	// FindActiveByUserAndFileHash finds an active (non-expired, uploading) session for the given user and file hash.
	FindActiveByUserAndFileHash(ctx context.Context, userID int64, fileHash string) (*UploadSession, error)

	// TransitionStatus atomically transitions session status from expectedStatus to newStatus, optionally setting playURL.
	// Returns true if the status transition succeeded (CAS matched).
	TransitionStatus(ctx context.Context, uploadID string, expectedStatus, newStatus UploadStatus, playURL string) (bool, error)
}

// BitmapStore defines high-performance atomic bit tracking contracts for uploaded chunk indices (Redis Bitmap).
type BitmapStore interface {
	// SetChunk atomically sets the bit at chunkIndex to 1.
	SetChunk(ctx context.Context, uploadID string, chunkIndex int) error

	// IsChunkUploaded checks if the bit at chunkIndex is 1.
	IsChunkUploaded(ctx context.Context, uploadID string, chunkIndex int) (bool, error)

	// CountUploadedChunks returns the total number of uploaded chunks (via BITCOUNT).
	CountUploadedChunks(ctx context.Context, uploadID string) (int, error)

	// GetUploadedChunks returns a slice of all uploaded chunk indices.
	GetUploadedChunks(ctx context.Context, uploadID string, totalChunks int) ([]int, error)

	// DeleteBitmap cleans up the bitmap key upon session completion or expiration.
	DeleteBitmap(ctx context.Context, uploadID string) error
}

// MergeLocker defines distributed mutual exclusion contract to prevent concurrent merge operations.
type MergeLocker interface {
	// TryLock attempts to acquire a mutex lock for uploadID with TTL.
	// Returns an unlock function if lock was successfully acquired, or ErrMergeConflict if already locked.
	TryLock(ctx context.Context, uploadID string, ttl time.Duration) (func() error, error)
}
