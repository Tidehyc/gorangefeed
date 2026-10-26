package storage

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/google/uuid"
)

const (
	// MaxVideoSize is 200MB per FR-VID-01.
	MaxVideoSize = 200 * 1024 * 1024
	// MaxCoverSize is 10MB for video cover images.
	MaxCoverSize = 10 * 1024 * 1024
)

var allowedCoverExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
}

var allowedCoverMimes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// VideoStorage handles video media and cover upload validation, magic number inspection,
// path isolation, and clean-on-failure local persistence.
type VideoStorage struct {
	baseDir string
}

// NewVideoStorage creates a video storage instance with base directory.
func NewVideoStorage(baseDir string) (*VideoStorage, error) {
	cleanDir := filepath.Clean(baseDir)
	videoDir := filepath.Join(cleanDir, "videos")
	if err := os.MkdirAll(videoDir, 0o755); err != nil {
		return nil, fmt.Errorf("create video base dir %s failed: %w", videoDir, err)
	}
	coverDir := filepath.Join(cleanDir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cover base dir %s failed: %w", coverDir, err)
	}
	return &VideoStorage{baseDir: cleanDir}, nil
}

// BaseDir returns the root uploads directory.
func (s *VideoStorage) BaseDir() string {
	return s.baseDir
}

// SaveVideo validates and stores an uploaded MP4 video.
// Returns the relative URL path (e.g. "/media/videos/101/20261026/uuid.mp4") and file size in bytes.
func (s *VideoStorage) SaveVideo(userID int64, fileHeader *multipart.FileHeader) (string, int64, error) {
	if fileHeader == nil {
		return "", 0, response.ErrInvalidParam("Missing video file")
	}

	// 1. File size check
	if fileHeader.Size <= 0 || fileHeader.Size > MaxVideoSize {
		return "", 0, response.ErrInvalidParam("Video file size must be between 1 byte and 200MB")
	}

	// 2. Extension validation (strictly .mp4)
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".mp4" {
		return "", 0, response.ErrInvalidParam("Video format only supports .mp4")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return "", 0, fmt.Errorf("open uploaded video error: %w", err)
	}
	defer src.Close()

	// 3. Binary Magic Number inspection: verify MP4 ftyp box signature
	// According to ISO/IEC 14496-12, bytes 4-8 in an MP4 file must be 'ftyp'.
	buf := make([]byte, 512)
	n, err := io.ReadFull(src, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", 0, fmt.Errorf("read video header error: %w", err)
	}
	if n < 8 || !bytes.Equal(buf[4:8], []byte("ftyp")) {
		return "", 0, response.ErrInvalidParam("Invalid video format: binary signature does not match MP4 (ftyp box missing)")
	}

	// Rewind file reader to beginning for saving
	if seeker, ok := src.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return "", 0, fmt.Errorf("seek video beginning error: %w", err)
		}
	} else {
		return "", 0, fmt.Errorf("uploaded video file does not support seek")
	}

	// 4. Safe target directory: /videos/{userID}/{date}/
	date := time.Now().Format("20060102")
	targetDir := filepath.Join(s.baseDir, "videos", fmt.Sprintf("%d", userID), date)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create video upload dir error: %w", err)
	}

	filename := uuid.NewString() + ".mp4"
	targetPath := filepath.Join(targetDir, filename)

	dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", 0, fmt.Errorf("create destination video file error: %w", err)
	}

	// Copy and enforce clean-on-failure
	written, copyErr := io.Copy(dst, src)
	_ = dst.Close()
	if copyErr != nil {
		_ = os.Remove(targetPath) // Clean up partial/corrupted file
		return "", 0, fmt.Errorf("save video file error: %w", copyErr)
	}

	mediaURL := fmt.Sprintf("/media/videos/%d/%s/%s", userID, date, filename)
	return mediaURL, written, nil
}

// SaveCover validates and stores an uploaded video cover image.
// Returns the relative URL path (e.g. "/media/covers/101/20261026/uuid.jpg").
func (s *VideoStorage) SaveCover(userID int64, fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader == nil {
		return "", response.ErrInvalidParam("Missing cover file")
	}

	if fileHeader.Size <= 0 || fileHeader.Size > MaxCoverSize {
		return "", response.ErrInvalidParam("Cover file size must be between 1 byte and 10MB")
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedCoverExts[ext] {
		return "", response.ErrInvalidParam("Cover image only supports .jpg, .jpeg, .png, and .webp formats")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("open uploaded cover error: %w", err)
	}
	defer src.Close()

	buf := make([]byte, 512)
	n, err := src.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read cover header error: %w", err)
	}

	mimeType := http.DetectContentType(buf[:n])
	if !allowedCoverMimes[mimeType] {
		return "", response.ErrInvalidParam("Invalid image content: detected MIME type is " + mimeType)
	}

	if seeker, ok := src.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek cover beginning error: %w", err)
		}
	} else {
		return "", fmt.Errorf("uploaded cover file does not support seek")
	}

	date := time.Now().Format("20060102")
	targetDir := filepath.Join(s.baseDir, "covers", fmt.Sprintf("%d", userID), date)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create cover upload dir error: %w", err)
	}

	filename := uuid.NewString() + ext
	targetPath := filepath.Join(targetDir, filename)

	dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("create destination cover file error: %w", err)
	}

	_, copyErr := io.Copy(dst, src)
	_ = dst.Close()
	if copyErr != nil {
		_ = os.Remove(targetPath)
		return "", fmt.Errorf("save cover file error: %w", copyErr)
	}

	mediaURL := fmt.Sprintf("/media/covers/%d/%s/%s", userID, date, filename)
	return mediaURL, nil
}
