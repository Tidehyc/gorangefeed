package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/google/uuid"
)

const (
	MaxAvatarSize = 10 * 1024 * 1024 // 10MB per FR-ACC-06
)

var allowedAvatarExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
}

var allowedMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// AvatarStorage handles avatar file validation and local disk persistence.
type AvatarStorage struct {
	baseDir string
}

// NewAvatarStorage initializes the storage with a base directory (e.g. "uploads").
func NewAvatarStorage(baseDir string) (*AvatarStorage, error) {
	cleanDir := filepath.Clean(baseDir)
	avatarDir := filepath.Join(cleanDir, "avatars")
	if err := os.MkdirAll(avatarDir, 0o755); err != nil {
		return nil, fmt.Errorf("create avatar base dir %s failed: %w", avatarDir, err)
	}
	return &AvatarStorage{baseDir: cleanDir}, nil
}

// BaseDir returns the root uploads directory for static mounting.
func (s *AvatarStorage) BaseDir() string {
	return s.baseDir
}

// SaveAvatar validates and saves an uploaded avatar image file.
// Returns the relative URL path for media retrieval (e.g. "/media/avatars/1001/uuid.png").
func (s *AvatarStorage) SaveAvatar(userID int64, fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader == nil {
		return "", response.ErrInvalidParam("Missing avatar file")
	}

	// 1. Size validation
	if fileHeader.Size <= 0 || fileHeader.Size > MaxAvatarSize {
		return "", response.ErrInvalidParam("Avatar file size must be between 1 byte and 10MB")
	}

	// 2. Extension validation
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedAvatarExts[ext] {
		return "", response.ErrInvalidParam("Avatar only supports .jpg, .jpeg, .png, and .webp formats")
	}

	// Open uploaded file
	src, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("open uploaded file error: %w", err)
	}
	defer src.Close()

	// 3. MIME type inspection (first 512 bytes)
	buf := make([]byte, 512)
	n, err := src.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read file header error: %w", err)
	}

	mimeType := http.DetectContentType(buf[:n])
	if !allowedMimeTypes[mimeType] {
		return "", response.ErrInvalidParam("Invalid image content: detected MIME type is " + mimeType)
	}

	// Rewind file reader to beginning
	if seeker, ok := src.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return "", fmt.Errorf("seek file beginning error: %w", err)
		}
	} else {
		return "", fmt.Errorf("uploaded file does not support seek")
	}

	// 4. Safe target path generation with UUID to prevent path traversal
	userDir := filepath.Join(s.baseDir, "avatars", fmt.Sprintf("%d", userID))
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		return "", fmt.Errorf("create user avatar dir error: %w", err)
	}

	filename := uuid.NewString() + ext
	targetPath := filepath.Join(userDir, filename)

	dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("create destination file error: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(targetPath)
		return "", fmt.Errorf("save avatar file error: %w", err)
	}

	// Relative media path adhering to DESIGN.md
	mediaURL := fmt.Sprintf("/media/avatars/%d/%s", userID, filename)
	return mediaURL, nil
}
