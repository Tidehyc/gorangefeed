package upload_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/mysql"
	persistenceupload "github.com/Tidehyc/gorangefeed/internal/infra/persistence/upload"
	"github.com/Tidehyc/gorangefeed/internal/infra/redis"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func setupTestInfra(t *testing.T) (*gorm.DB, *goredis.Client, func()) {
	t.Helper()

	var configPath string
	for _, candidate := range []string{
		"../../../../configs/config.yaml",
		"../../../configs/config.yaml",
		"../../configs/config.yaml",
		"configs/config.yaml",
	} {
		if _, err := os.Stat(candidate); err == nil {
			configPath = candidate
			break
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Skipf("skipping persistence test: config load failed (%v)", err)
		return nil, nil, nil
	}

	db, err := mysql.New(&cfg.MySQL)
	if err != nil {
		t.Skipf("skipping persistence test: MySQL connection failed (%v)", err)
		return nil, nil, nil
	}

	rdb, err := redis.New(&cfg.Redis)
	if err != nil {
		t.Skipf("skipping persistence test: Redis connection failed (%v)", err)
		return nil, nil, nil
	}

	cleanup := func() {
		_ = rdb.Close()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	return db, rdb, cleanup
}

func TestUploadSessionRepository_Lifecycle(t *testing.T) {
	db, _, cleanup := setupTestInfra(t)
	if cleanup != nil {
		defer cleanup()
	}

	repo := persistenceupload.NewSessionRepository(db)
	ctx := context.Background()

	userID := time.Now().UnixNano()
	uploadID := uuid.NewString()
	fileHash := fmt.Sprintf("hash_%d", userID)

	session := &domainupload.UploadSession{
		UploadID:    uploadID,
		UserID:      userID,
		FileHash:    fileHash,
		FileName:    "test_video.mp4",
		FileSize:    15 * 1024 * 1024,
		ChunkSize:   5 * 1024 * 1024,
		TotalChunks: 3,
		Status:      domainupload.UploadStatusUploading,
		ExpireAt:    time.Now().Add(24 * time.Hour),
	}

	// 1. Create
	if err := repo.Create(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if session.ID <= 0 {
		t.Fatalf("expected positive session ID, got %d", session.ID)
	}

	// 2. FindByUploadID
	found, err := repo.FindByUploadID(ctx, uploadID)
	if err != nil {
		t.Fatalf("failed to find session: %v", err)
	}
	if found.UploadID != uploadID || found.UserID != userID || found.TotalChunks != 3 {
		t.Fatalf("mismatched session data: %+v", found)
	}

	// 3. FindActiveByUserAndFileHash (resumable)
	active, err := repo.FindActiveByUserAndFileHash(ctx, userID, fileHash)
	if err != nil {
		t.Fatalf("failed to find active session by file hash: %v", err)
	}
	if active.UploadID != uploadID {
		t.Fatalf("expected uploadID %s, got %s", uploadID, active.UploadID)
	}

	// 4. TransitionStatus (Uploading -> Merging) via CAS
	swapped, err := repo.TransitionStatus(ctx, uploadID, domainupload.UploadStatusUploading, domainupload.UploadStatusMerging, "")
	if err != nil {
		t.Fatalf("failed to transition status: %v", err)
	}
	if !swapped {
		t.Fatalf("expected CAS swap to succeed")
	}

	// 5. Repeat CAS with old status -> should return false
	swappedAgain, err := repo.TransitionStatus(ctx, uploadID, domainupload.UploadStatusUploading, domainupload.UploadStatusMerging, "")
	if err != nil {
		t.Fatalf("unexpected error on repeated CAS: %v", err)
	}
	if swappedAgain {
		t.Fatalf("expected CAS swap to fail when expected status does not match")
	}

	// 6. TransitionStatus (Merging -> Completed with playURL)
	finalURL := "/media/videos/101/20261026/test.mp4"
	swapped, err = repo.TransitionStatus(ctx, uploadID, domainupload.UploadStatusMerging, domainupload.UploadStatusCompleted, finalURL)
	if err != nil || !swapped {
		t.Fatalf("failed to complete session: swapped=%v, err=%v", swapped, err)
	}

	completed, err := repo.FindByUploadID(ctx, uploadID)
	if err != nil {
		t.Fatalf("failed to fetch completed session: %v", err)
	}
	if completed.Status != domainupload.UploadStatusCompleted || completed.PlayURL != finalURL {
		t.Fatalf("unexpected completed session state: %+v", completed)
	}

	// 7. FindActiveByUserAndFileHash should now return ErrSessionNotFound (since it's completed)
	_, err = repo.FindActiveByUserAndFileHash(ctx, userID, fileHash)
	if err != domainupload.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound for completed session, got %v", err)
	}
}

func TestBitmapStore_BitOperations(t *testing.T) {
	_, rdb, cleanup := setupTestInfra(t)
	if cleanup != nil {
		defer cleanup()
	}

	store := persistenceupload.NewBitmapStore(rdb)
	ctx := context.Background()
	uploadID := fmt.Sprintf("test_bit_%d", time.Now().UnixNano())

	defer func() {
		_ = store.DeleteBitmap(ctx, uploadID)
	}()

	// 1. Initially count should be 0
	count, err := store.CountUploadedChunks(ctx, uploadID)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}

	// 2. Set chunk 0 and chunk 2
	if err := store.SetChunk(ctx, uploadID, 0); err != nil {
		t.Fatalf("failed to set chunk 0: %v", err)
	}
	if err := store.SetChunk(ctx, uploadID, 2); err != nil {
		t.Fatalf("failed to set chunk 2: %v", err)
	}

	// 3. Verify chunk status
	is0, err := store.IsChunkUploaded(ctx, uploadID, 0)
	if err != nil || !is0 {
		t.Fatalf("expected chunk 0 to be uploaded, is0=%v, err=%v", is0, err)
	}
	is1, err := store.IsChunkUploaded(ctx, uploadID, 1)
	if err != nil || is1 {
		t.Fatalf("expected chunk 1 not to be uploaded, is1=%v, err=%v", is1, err)
	}
	is2, err := store.IsChunkUploaded(ctx, uploadID, 2)
	if err != nil || !is2 {
		t.Fatalf("expected chunk 2 to be uploaded, is2=%v, err=%v", is2, err)
	}

	// 4. Verify count
	count, err = store.CountUploadedChunks(ctx, uploadID)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	// 5. Verify GetUploadedChunks
	indices, err := store.GetUploadedChunks(ctx, uploadID, 5)
	if err != nil {
		t.Fatalf("failed to get uploaded chunks: %v", err)
	}
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 2 {
		t.Fatalf("unexpected chunk indices: %+v", indices)
	}

	// 6. Set chunk 1 and verify all 3
	if err := store.SetChunk(ctx, uploadID, 1); err != nil {
		t.Fatalf("failed to set chunk 1: %v", err)
	}
	count, err = store.CountUploadedChunks(ctx, uploadID)
	if err != nil || count != 3 {
		t.Fatalf("expected count 3, got %d (err=%v)", count, err)
	}
	indices, err = store.GetUploadedChunks(ctx, uploadID, 3)
	if err != nil || len(indices) != 3 || indices[0] != 0 || indices[1] != 1 || indices[2] != 2 {
		t.Fatalf("expected [0, 1, 2], got %+v", indices)
	}
}

func TestMergeLocker_DistributedMutex(t *testing.T) {
	_, rdb, cleanup := setupTestInfra(t)
	if cleanup != nil {
		defer cleanup()
	}

	locker := persistenceupload.NewMergeLocker(rdb)
	ctx := context.Background()
	uploadID := fmt.Sprintf("test_lock_%d", time.Now().UnixNano())

	// 1. First lock acquires successfully
	unlock, err := locker.TryLock(ctx, uploadID, 10*time.Second)
	if err != nil {
		t.Fatalf("expected lock to succeed, got %v", err)
	}

	// 2. Concurrent second lock should be rejected with ErrMergeConflict
	_, err2 := locker.TryLock(ctx, uploadID, 10*time.Second)
	if err2 != domainupload.ErrMergeConflict {
		t.Fatalf("expected ErrMergeConflict, got %v", err2)
	}

	// 3. Unlock first lock
	if err := unlock(); err != nil {
		t.Fatalf("failed to unlock: %v", err)
	}

	// 4. Third lock now succeeds
	unlock3, err3 := locker.TryLock(ctx, uploadID, 10*time.Second)
	if err3 != nil {
		t.Fatalf("expected third lock to succeed after unlock, got %v", err3)
	}
	_ = unlock3()
}
