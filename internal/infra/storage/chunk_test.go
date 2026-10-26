package storage_test

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
)

func TestChunkStorage_SaveAndMerge(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "chunk_storage_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := storage.NewChunkStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create chunk storage: %v", err)
	}

	uploadID := "upload-test-uuid-001"
	authorID := int64(1001)

	// Chunk 0 contains ftyp box: 16 bytes
	chunk0Data := make([]byte, 16)
	chunk0Data[0] = 0x00
	chunk0Data[1] = 0x00
	chunk0Data[2] = 0x00
	chunk0Data[3] = 0x10
	copy(chunk0Data[4:8], "ftyp")
	copy(chunk0Data[8:12], "isom")
	copy(chunk0Data[12:16], "data")
	chunk0Hash := fmt.Sprintf("%x", md5.Sum(chunk0Data))

	// Chunk 1 contains subsequent bytes
	chunk1Data := []byte("continuation payload for video file")
	chunk1Hash := fmt.Sprintf("%x", md5.Sum(chunk1Data))

	// 1. Save chunk with hash mismatch -> should return ErrChunkHashMismatch and leave no file
	_, err = store.SaveChunk(uploadID, 0, "00000000000000000000000000000000", bytes.NewReader(chunk0Data))
	if err != domainupload.ErrChunkHashMismatch {
		t.Fatalf("expected ErrChunkHashMismatch, got %v", err)
	}
	chunk0Path := filepath.Join(tempDir, "tmp", uploadID, "0")
	if _, err := os.Stat(chunk0Path); !os.IsNotExist(err) {
		t.Fatalf("expected chunk file to be deleted on hash mismatch, but it exists")
	}

	// 2. Save chunk 0 with correct hash
	n0, err := store.SaveChunk(uploadID, 0, chunk0Hash, bytes.NewReader(chunk0Data))
	if err != nil || n0 != int64(len(chunk0Data)) {
		t.Fatalf("failed to save chunk 0: n=%d, err=%v", n0, err)
	}

	// 3. Save chunk 1 with correct hash
	n1, err := store.SaveChunk(uploadID, 1, chunk1Hash, bytes.NewReader(chunk1Data))
	if err != nil || n1 != int64(len(chunk1Data)) {
		t.Fatalf("failed to save chunk 1: n=%d, err=%v", n1, err)
	}

	// 4. Merge chunks into valid video
	playURL, totalSize, err := store.MergeChunks(uploadID, 2, authorID)
	if err != nil {
		t.Fatalf("failed to merge chunks: %v", err)
	}
	expectedTotalSize := int64(len(chunk0Data) + len(chunk1Data))
	if totalSize != expectedTotalSize {
		t.Fatalf("expected total size %d, got %d", expectedTotalSize, totalSize)
	}
	if playURL == "" {
		t.Fatalf("expected non-empty playURL")
	}

	// 5. Verify temporary directory is cleaned up
	tmpDir := filepath.Join(tempDir, "tmp", uploadID)
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("expected tmpDir to be cleaned up after merge, but it exists")
	}
}

func TestChunkStorage_MergeCorruptedSniffing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "chunk_storage_corrupt_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := storage.NewChunkStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create chunk storage: %v", err)
	}

	uploadID := "upload-corrupt-uuid"
	authorID := int64(1002)

	// Chunk contains text, NOT ftyp
	fakeData := []byte("plain text content not mp4")
	fakeHash := fmt.Sprintf("%x", md5.Sum(fakeData))

	_, err = store.SaveChunk(uploadID, 0, fakeHash, bytes.NewReader(fakeData))
	if err != nil {
		t.Fatalf("failed to save fake chunk: %v", err)
	}

	_, _, err = store.MergeChunks(uploadID, 1, authorID)
	if err != domainupload.ErrCorruptedMergedFile {
		t.Fatalf("expected ErrCorruptedMergedFile on non-mp4 merge, got %v", err)
	}
}
