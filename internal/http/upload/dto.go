package upload

import (
	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
)

// InitUploadRequest defines the payload for initializing a chunked upload session.
type InitUploadRequest struct {
	FileName    string `json:"file_name" binding:"required"`
	FileSize    int64  `json:"file_size" binding:"required,gt=0"`
	ChunkSize   int    `json:"chunk_size"`
	TotalChunks int    `json:"total_chunks"`
	FileHash    string `json:"file_hash" binding:"required"`
}

// InitUploadResponse defines the response after session initialization.
type InitUploadResponse struct {
	UploadID       string `json:"upload_id"`
	UploadedChunks []int  `json:"uploaded_chunks"`
	TotalChunks    int    `json:"total_chunks"`
	IsResumed      bool   `json:"is_resumed"`
}

// UploadChunkResponse defines the response after a single chunk is uploaded.
type UploadChunkResponse struct {
	ChunkIndex int `json:"chunk_index"`
}

// ChunkStatusResponse defines the response when querying session upload status.
type ChunkStatusResponse struct {
	UploadID       string                    `json:"upload_id"`
	TotalChunks    int                       `json:"total_chunks"`
	UploadedChunks []int                     `json:"uploaded_chunks"`
	Status         domainupload.UploadStatus `json:"status"`
	PlayURL        string                    `json:"play_url,omitempty"`
}

// CompleteUploadRequest defines the payload for finalizing chunk merge.
type CompleteUploadRequest struct {
	UploadID string `json:"upload_id" binding:"required"`
}

// CompleteUploadResponse defines the response after chunks are merged into final video.
type CompleteUploadResponse struct {
	UploadID string `json:"upload_id"`
	PlayURL  string `json:"play_url"`
	Size     int64  `json:"size"`
}
