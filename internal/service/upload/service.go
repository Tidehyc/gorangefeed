package upload

import (
	"context"
	"io"
	"math"
	"strings"
	"time"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	"github.com/google/uuid"
)

// Storage defines the file storage operations required by the upload service.
type Storage interface {
	SaveChunk(uploadID string, chunkIndex int, expectedMD5 string, src io.Reader) (int64, error)
	MergeChunks(uploadID string, totalChunks int, authorID int64) (string, int64, error)
	CleanSession(uploadID string) error
}

// InitResult represents the response for session initialization.
type InitResult struct {
	UploadID       string `json:"upload_id"`
	UploadedChunks []int  `json:"uploaded_chunks"`
	TotalChunks    int    `json:"total_chunks"`
	IsResumed      bool   `json:"is_resumed"`
}

// StatusResult represents the response for session status inquiry.
type StatusResult struct {
	UploadID       string                    `json:"upload_id"`
	TotalChunks    int                       `json:"total_chunks"`
	UploadedChunks []int                     `json:"uploaded_chunks"`
	Status         domainupload.UploadStatus `json:"status"`
	PlayURL        string                    `json:"play_url,omitempty"`
}

// CompleteResult represents the response after successful chunk merging.
type CompleteResult struct {
	UploadID string `json:"upload_id"`
	PlayURL  string `json:"play_url"`
	Size     int64  `json:"size"`
}

// Service defines the chunked upload and resumable transfer use case contract.
type Service interface {
	InitSession(ctx context.Context, userID int64, filename string, fileSize int64, chunkSize int, totalChunks int, fileHash string) (*InitResult, error)
	UploadChunk(ctx context.Context, userID int64, uploadID string, chunkIndex int, chunkHash string, src io.Reader) error
	GetStatus(ctx context.Context, userID int64, uploadID string) (*StatusResult, error)
	Complete(ctx context.Context, userID int64, uploadID string) (*CompleteResult, error)
}

type service struct {
	sessionRepo domainupload.SessionRepository
	bitmapStore domainupload.BitmapStore
	mergeLocker domainupload.MergeLocker
	storage     Storage
}

// NewService constructs a new chunk upload service instance.
func NewService(
	sessionRepo domainupload.SessionRepository,
	bitmapStore domainupload.BitmapStore,
	mergeLocker domainupload.MergeLocker,
	storage Storage,
) Service {
	return &service{
		sessionRepo: sessionRepo,
		bitmapStore: bitmapStore,
		mergeLocker: mergeLocker,
		storage:     storage,
	}
}

func (s *service) InitSession(
	ctx context.Context,
	userID int64,
	filename string,
	fileSize int64,
	chunkSize int,
	totalChunks int,
	fileHash string,
) (*InitResult, error) {
	if chunkSize <= 0 {
		chunkSize = domainupload.DefaultChunkSize
	}
	if totalChunks <= 0 && fileSize > 0 {
		totalChunks = int(math.Ceil(float64(fileSize) / float64(chunkSize)))
	}

	session := &domainupload.UploadSession{
		UploadID:    uuid.NewString(),
		UserID:      userID,
		FileHash:    strings.TrimSpace(fileHash),
		FileName:    strings.TrimSpace(filename),
		FileSize:    fileSize,
		ChunkSize:   chunkSize,
		TotalChunks: totalChunks,
		Status:      domainupload.UploadStatusUploading,
		ExpireAt:    time.Now().Add(domainupload.SessionTTL),
	}

	if err := session.Validate(); err != nil {
		return nil, err
	}

	// Check for existing active session by the same user and file hash (instant resume)
	if active, err := s.sessionRepo.FindActiveByUserAndFileHash(ctx, userID, session.FileHash); err == nil && active != nil {
		uploadedChunks, _ := s.bitmapStore.GetUploadedChunks(ctx, active.UploadID, active.TotalChunks)
		if uploadedChunks == nil {
			uploadedChunks = []int{}
		}
		return &InitResult{
			UploadID:       active.UploadID,
			UploadedChunks: uploadedChunks,
			TotalChunks:    active.TotalChunks,
			IsResumed:      true,
		}, nil
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	return &InitResult{
		UploadID:       session.UploadID,
		UploadedChunks: []int{},
		TotalChunks:    session.TotalChunks,
		IsResumed:      false,
	}, nil
}

func (s *service) UploadChunk(
	ctx context.Context,
	userID int64,
	uploadID string,
	chunkIndex int,
	chunkHash string,
	src io.Reader,
) error {
	session, err := s.sessionRepo.FindByUploadID(ctx, uploadID)
	if err != nil {
		return err
	}
	if session.UserID != userID {
		return domainupload.ErrSessionForbidden
	}
	if err := session.CanUploadChunk(); err != nil {
		return err
	}
	if chunkIndex < 0 || chunkIndex >= session.TotalChunks {
		return domainupload.ErrInvalidChunkIndex
	}

	// Fast-path: check if chunk is already marked as uploaded in Redis bitmap
	alreadyUploaded, err := s.bitmapStore.IsChunkUploaded(ctx, uploadID, chunkIndex)
	if err == nil && alreadyUploaded {
		return nil
	}

	// Persist chunk to temporary storage with MD5 verification
	if _, err := s.storage.SaveChunk(uploadID, chunkIndex, chunkHash, src); err != nil {
		return err
	}

	// Atomically set bit in Redis bitmap
	return s.bitmapStore.SetChunk(ctx, uploadID, chunkIndex)
}

func (s *service) GetStatus(ctx context.Context, userID int64, uploadID string) (*StatusResult, error) {
	session, err := s.sessionRepo.FindByUploadID(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	if session.UserID != userID {
		return nil, domainupload.ErrSessionForbidden
	}

	uploadedChunks, err := s.bitmapStore.GetUploadedChunks(ctx, uploadID, session.TotalChunks)
	if err != nil {
		return nil, err
	}
	if uploadedChunks == nil {
		uploadedChunks = []int{}
	}

	return &StatusResult{
		UploadID:       session.UploadID,
		TotalChunks:    session.TotalChunks,
		UploadedChunks: uploadedChunks,
		Status:         session.Status,
		PlayURL:        session.PlayURL,
	}, nil
}

func (s *service) Complete(ctx context.Context, userID int64, uploadID string) (*CompleteResult, error) {
	session, err := s.sessionRepo.FindByUploadID(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	if session.UserID != userID {
		return nil, domainupload.ErrSessionForbidden
	}

	// Idempotent fast-path: if already completed, return existing play URL
	if session.IsCompleted() {
		return &CompleteResult{
			UploadID: uploadID,
			PlayURL:  session.PlayURL,
			Size:     session.FileSize,
		}, nil
	}

	if err := session.CanMerge(); err != nil {
		return nil, err
	}

	// Verify all chunks have been received
	uploadedCount, err := s.bitmapStore.CountUploadedChunks(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	if uploadedCount < session.TotalChunks {
		return nil, domainupload.ErrMissingChunks
	}

	// Acquire distributed merge lock to eliminate concurrency / double-complete race conditions
	unlock, err := s.mergeLocker.TryLock(ctx, uploadID, 60*time.Second)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = unlock()
	}()

	// CAS transition MySQL status: Uploading -> Merging
	swapped, err := s.sessionRepo.TransitionStatus(ctx, uploadID, domainupload.UploadStatusUploading, domainupload.UploadStatusMerging, "")
	if err != nil {
		return nil, err
	}
	if !swapped {
		return nil, domainupload.ErrMergeConflict
	}

	// Stream merge chunks into final video file and validate MP4 format
	playURL, totalWritten, err := s.storage.MergeChunks(uploadID, session.TotalChunks, userID)
	if err != nil {
		// Rollback status to Uploading so user/client can retry or re-upload corrupted chunk
		_, _ = s.sessionRepo.TransitionStatus(ctx, uploadID, domainupload.UploadStatusMerging, domainupload.UploadStatusUploading, "")
		return nil, err
	}

	// Final transition MySQL status: Merging -> Completed with playURL
	if _, err := s.sessionRepo.TransitionStatus(ctx, uploadID, domainupload.UploadStatusMerging, domainupload.UploadStatusCompleted, playURL); err != nil {
		return nil, err
	}

	// Cleanup Redis bitmap
	_ = s.bitmapStore.DeleteBitmap(ctx, uploadID)

	return &CompleteResult{
		UploadID: uploadID,
		PlayURL:  playURL,
		Size:     totalWritten,
	}, nil
}
