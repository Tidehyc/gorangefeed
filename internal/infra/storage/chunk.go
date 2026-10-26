package storage

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	"github.com/google/uuid"
)

// ChunkStorage manages temporary chunk files, validation, streaming merge, and MP4 sniffing.
type ChunkStorage struct {
	baseDir string
}

// NewChunkStorage initializes a chunk storage instance and ensures required directory hierarchy.
func NewChunkStorage(baseDir string) (*ChunkStorage, error) {
	cleanDir := filepath.Clean(baseDir)
	tmpDir := filepath.Join(cleanDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, fmt.Errorf("create tmp storage dir %s failed: %w", tmpDir, err)
	}
	videoDir := filepath.Join(cleanDir, "videos")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		return nil, fmt.Errorf("create video storage dir %s failed: %w", videoDir, err)
	}
	return &ChunkStorage{baseDir: cleanDir}, nil
}

// sanitizeID ensures identifier contains only alphanumeric characters or hyphens to prevent path traversal.
func sanitizeID(id string) error {
	if strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return fmt.Errorf("invalid path characters in id: %s", id)
	}
	return nil
}

// SaveChunk writes a single chunk to the temporary directory while validating its MD5 hash.
func (s *ChunkStorage) SaveChunk(uploadID string, chunkIndex int, expectedMD5 string, src io.Reader) (int64, error) {
	if err := sanitizeID(uploadID); err != nil {
		return 0, err
	}
	if chunkIndex < 0 {
		return 0, domainupload.ErrInvalidChunkIndex
	}

	tmpDir := filepath.Join(s.baseDir, "tmp", uploadID)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return 0, fmt.Errorf("failed to create session tmp dir: %w", err)
	}

	chunkPath := filepath.Join(tmpDir, fmt.Sprintf("%d", chunkIndex))
	dst, err := os.OpenFile(chunkPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("failed to create chunk file: %w", err)
	}

	hasher := md5.New()
	multiWriter := io.MultiWriter(dst, hasher)

	written, err := io.Copy(multiWriter, src)
	_ = dst.Close()
	if err != nil {
		_ = os.Remove(chunkPath)
		return 0, fmt.Errorf("failed to write chunk file: %w", err)
	}

	actualHash := fmt.Sprintf("%x", hasher.Sum(nil))
	if expectedMD5 != "" && !strings.EqualFold(actualHash, expectedMD5) {
		_ = os.Remove(chunkPath)
		return 0, domainupload.ErrChunkHashMismatch
	}

	return written, nil
}

// MergeChunks sequentially concatenates all chunks into a final MP4 video file, validates its magic number,
// and deletes temporary chunks.
func (s *ChunkStorage) MergeChunks(uploadID string, totalChunks int, authorID int64) (string, int64, error) {
	if err := sanitizeID(uploadID); err != nil {
		return "", 0, err
	}
	if totalChunks <= 0 {
		return "", 0, domainupload.ErrInvalidTotalChunks
	}

	dateFolder := time.Now().Format("20060102")
	authorFolder := strconv.FormatInt(authorID, 10)
	destDir := filepath.Join(s.baseDir, "videos", authorFolder, dateFolder)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("failed to create dest video dir: %w", err)
	}

	videoID := uuid.NewString()
	destPath := filepath.Join(destDir, videoID+".mp4")
	relURL := fmt.Sprintf("/media/videos/%d/%s/%s.mp4", authorID, dateFolder, videoID)

	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create destination file: %w", err)
	}

	var totalWritten int64
	tmpDir := filepath.Join(s.baseDir, "tmp", uploadID)

	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(tmpDir, fmt.Sprintf("%d", i))
		cf, err := os.Open(chunkPath)
		if err != nil {
			_ = destFile.Close()
			_ = os.Remove(destPath)
			return "", 0, fmt.Errorf("missing chunk %d: %w", i, err)
		}

		n, err := io.Copy(destFile, cf)
		_ = cf.Close()
		if err != nil {
			_ = destFile.Close()
			_ = os.Remove(destPath)
			return "", 0, fmt.Errorf("error copying chunk %d: %w", i, err)
		}
		totalWritten += n
	}

	if err := destFile.Sync(); err != nil {
		_ = destFile.Close()
		_ = os.Remove(destPath)
		return "", 0, fmt.Errorf("failed to sync merged file: %w", err)
	}
	_ = destFile.Close()

	// Sniff magic number on merged artifact (verify ISO/IEC 14496-12 ftyp box)
	mergedFile, err := os.Open(destPath)
	if err != nil {
		_ = os.Remove(destPath)
		return "", 0, fmt.Errorf("failed to reopen merged file for sniffing: %w", err)
	}
	headerBuf := make([]byte, 16)
	readBytes, err := io.ReadFull(mergedFile, headerBuf)
	_ = mergedFile.Close()
	if err != nil || readBytes < 12 || string(headerBuf[4:8]) != "ftyp" {
		_ = os.Remove(destPath)
		return "", 0, domainupload.ErrCorruptedMergedFile
	}

	// Clean up temporary chunks directory
	_ = os.RemoveAll(tmpDir)

	return relURL, totalWritten, nil
}

// CleanSession removes the temporary directory and all constituent chunks for uploadID.
func (s *ChunkStorage) CleanSession(uploadID string) error {
	if err := sanitizeID(uploadID); err != nil {
		return err
	}
	tmpDir := filepath.Join(s.baseDir, "tmp", uploadID)
	return os.RemoveAll(tmpDir)
}
