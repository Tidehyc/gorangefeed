package upload_test

import (
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/domain/upload"
)

func TestUploadSession_Validate(t *testing.T) {
	validSession := func() *upload.UploadSession {
		return &upload.UploadSession{
			UploadID:    "test-upload-uuid-1234",
			UserID:      101,
			FileHash:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			FileName:    "sample_video.mp4",
			FileSize:    50 * 1024 * 1024,
			ChunkSize:   5 * 1024 * 1024,
			TotalChunks: 10,
			Status:      upload.UploadStatusUploading,
			ExpireAt:    time.Now().Add(24 * time.Hour),
		}
	}

	tests := []struct {
		name    string
		mutate  func(s *upload.UploadSession)
		wantErr error
	}{
		{
			name:   "valid session",
			mutate: func(s *upload.UploadSession) {},
		},
		{
			name:    "invalid user id",
			mutate:  func(s *upload.UploadSession) { s.UserID = 0 },
			wantErr: upload.ErrInvalidUserID,
		},
		{
			name:    "empty upload id",
			mutate:  func(s *upload.UploadSession) { s.UploadID = "   " },
			wantErr: upload.ErrSessionNotFound,
		},
		{
			name:    "empty file hash",
			mutate:  func(s *upload.UploadSession) { s.FileHash = "" },
			wantErr: upload.ErrInvalidFileHash,
		},
		{
			name:    "empty file name",
			mutate:  func(s *upload.UploadSession) { s.FileName = " " },
			wantErr: upload.ErrEmptyFileName,
		},
		{
			name:    "zero file size",
			mutate:  func(s *upload.UploadSession) { s.FileSize = 0 },
			wantErr: upload.ErrInvalidFileSize,
		},
		{
			name:    "file size exceeds limit",
			mutate:  func(s *upload.UploadSession) { s.FileSize = 201 * 1024 * 1024 },
			wantErr: upload.ErrFileSizeExceeded,
		},
		{
			name:    "invalid chunk size",
			mutate:  func(s *upload.UploadSession) { s.ChunkSize = 0 },
			wantErr: upload.ErrInvalidChunkSize,
		},
		{
			name:    "invalid total chunks zero",
			mutate:  func(s *upload.UploadSession) { s.TotalChunks = 0 },
			wantErr: upload.ErrInvalidTotalChunks,
		},
		{
			name:    "total chunks exceeds upper bound",
			mutate:  func(s *upload.UploadSession) { s.TotalChunks = 10001 },
			wantErr: upload.ErrInvalidTotalChunks,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := validSession()
			tc.mutate(s)
			err := s.Validate()
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestUploadSession_StateTransitions(t *testing.T) {
	s := &upload.UploadSession{
		UploadID:    "test-sess-1",
		UserID:      100,
		Status:      upload.UploadStatusUploading,
		ExpireAt:    time.Now().Add(1 * time.Hour),
		TotalChunks: 5,
	}

	if !s.IsActive() {
		t.Fatalf("expected session to be active")
	}
	if err := s.CanUploadChunk(); err != nil {
		t.Fatalf("expected can upload chunk, got %v", err)
	}
	if err := s.CanMerge(); err != nil {
		t.Fatalf("expected can merge, got %v", err)
	}

	// Test expiration
	s.ExpireAt = time.Now().Add(-1 * time.Minute)
	if s.IsActive() {
		t.Fatalf("expected expired session not to be active")
	}
	if !s.IsExpired() {
		t.Fatalf("expected session to be expired")
	}
	if err := s.CanUploadChunk(); err != upload.ErrSessionExpired {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
	if err := s.CanMerge(); err != upload.ErrSessionExpired {
		t.Fatalf("expected ErrSessionExpired for merge, got %v", err)
	}

	// Test status transitions
	s.ExpireAt = time.Now().Add(1 * time.Hour)
	s.Status = upload.UploadStatusMerging
	if !s.IsMerging() {
		t.Fatalf("expected is merging")
	}
	if err := s.CanUploadChunk(); err != upload.ErrSessionNotActive {
		t.Fatalf("expected ErrSessionNotActive when merging, got %v", err)
	}
	if err := s.CanMerge(); err != upload.ErrMergeConflict {
		t.Fatalf("expected ErrMergeConflict when already merging, got %v", err)
	}

	s.Status = upload.UploadStatusCompleted
	s.PlayURL = "/media/videos/100/20261026/xyz.mp4"
	if !s.IsCompleted() {
		t.Fatalf("expected is completed")
	}
}
