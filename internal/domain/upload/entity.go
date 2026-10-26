package upload

import (
	"strings"
	"time"
)

// UploadStatus represents the lifecycle state of a chunked upload session.
type UploadStatus int8

const (
	// UploadStatusUploading indicates session is active and accepting chunk uploads.
	UploadStatusUploading UploadStatus = 1
	// UploadStatusMerging indicates all chunks are received and file is being concatenated.
	UploadStatusMerging UploadStatus = 2
	// UploadStatusCompleted indicates chunks have been merged into the final video file.
	UploadStatusCompleted UploadStatus = 3
	// UploadStatusExpired indicates the session timed out without completing.
	UploadStatusExpired UploadStatus = 4
)

const (
	// DefaultChunkSize is the standard 5MB chunk size for upload slicing.
	DefaultChunkSize = 5 << 20
	// MaxFileSize is the 200MB hard limit for uploaded videos.
	MaxFileSize = 200 << 20
	// MaxChunks is the safety upper bound for chunks in a single session.
	MaxChunks = 10000
	// SessionTTL defines the validity duration of an upload session.
	SessionTTL = 24 * time.Hour
)

// UploadSession represents a chunked file upload session aggregate root.
type UploadSession struct {
	ID          int64        `json:"id"`
	UploadID    string       `json:"upload_id"`
	UserID      int64        `json:"user_id"`
	FileHash    string       `json:"file_hash"`
	FileName    string       `json:"file_name"`
	FileSize    int64        `json:"file_size"`
	ChunkSize   int          `json:"chunk_size"`
	TotalChunks int          `json:"total_chunks"`
	Status      UploadStatus `json:"status"`
	PlayURL     string       `json:"play_url,omitempty"`
	ExpireAt    time.Time    `json:"expire_at"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// IsActive returns true if session is in uploading state and not past expiration time.
func (s *UploadSession) IsActive() bool {
	return s.Status == UploadStatusUploading && time.Now().Before(s.ExpireAt)
}

// IsMerging returns true if session is currently merging chunks.
func (s *UploadSession) IsMerging() bool {
	return s.Status == UploadStatusMerging
}

// IsCompleted returns true if session has completed successfully.
func (s *UploadSession) IsCompleted() bool {
	return s.Status == UploadStatusCompleted
}

// IsExpired returns true if session status is expired or current time is past ExpireAt.
func (s *UploadSession) IsExpired() bool {
	return s.Status == UploadStatusExpired || time.Now().After(s.ExpireAt)
}

// CanUploadChunk validates whether session can accept a chunk upload.
func (s *UploadSession) CanUploadChunk() error {
	if s.IsExpired() {
		return ErrSessionExpired
	}
	if s.Status != UploadStatusUploading {
		return ErrSessionNotActive
	}
	return nil
}

// CanMerge validates whether session can proceed to chunk merging.
func (s *UploadSession) CanMerge() error {
	if s.IsExpired() {
		return ErrSessionExpired
	}
	if s.Status != UploadStatusUploading {
		return ErrMergeConflict
	}
	return nil
}

// Validate performs domain invariant checks on initial session attributes.
func (s *UploadSession) Validate() error {
	if s.UserID <= 0 {
		return ErrInvalidUserID
	}
	if strings.TrimSpace(s.UploadID) == "" {
		return ErrSessionNotFound
	}
	if strings.TrimSpace(s.FileHash) == "" {
		return ErrInvalidFileHash
	}
	if strings.TrimSpace(s.FileName) == "" {
		return ErrEmptyFileName
	}
	if s.FileSize <= 0 {
		return ErrInvalidFileSize
	}
	if s.FileSize > MaxFileSize {
		return ErrFileSizeExceeded
	}
	if s.ChunkSize <= 0 {
		return ErrInvalidChunkSize
	}
	if s.TotalChunks <= 0 || s.TotalChunks > MaxChunks {
		return ErrInvalidTotalChunks
	}
	return nil
}
