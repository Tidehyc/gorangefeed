package upload

import "errors"

var (
	ErrSessionNotFound     = errors.New("upload session not found")
	ErrSessionExpired      = errors.New("upload session expired")
	ErrSessionNotActive    = errors.New("upload session is not in uploading status")
	ErrSessionForbidden    = errors.New("forbidden: session does not belong to current user")
	ErrInvalidChunkIndex   = errors.New("invalid chunk index")
	ErrChunkHashMismatch   = errors.New("chunk hash mismatch")
	ErrMissingChunks       = errors.New("not all chunks have been uploaded")
	ErrFileSizeExceeded    = errors.New("file size exceeds maximum allowed limit (200MB)")
	ErrInvalidFileSize     = errors.New("invalid file size: must be positive")
	ErrInvalidTotalChunks  = errors.New("invalid total chunks: must be positive and within bounds")
	ErrInvalidChunkSize    = errors.New("invalid chunk size: must be positive")
	ErrInvalidFileHash     = errors.New("invalid file hash: cannot be empty")
	ErrEmptyFileName       = errors.New("file name cannot be empty")
	ErrInvalidUserID       = errors.New("invalid user id: must be positive")
	ErrMergeConflict       = errors.New("concurrent merge conflict: session is already merging or completed")
	ErrCorruptedMergedFile = errors.New("merged file format validation failed: invalid MP4")
)
