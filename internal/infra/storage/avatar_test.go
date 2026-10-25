package storage

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tidehyc/gorangefeed/internal/infra/router"
	"github.com/gin-gonic/gin"
)

// createMultipartHeader creates an in-memory *multipart.FileHeader for testing.
func createMultipartHeader(filename string, content []byte) (*multipart.FileHeader, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if err := req.ParseMultipartForm(int64(len(content)) + 1024); err != nil {
		return nil, err
	}

	return req.MultipartForm.File["file"][0], nil
}

// 1x1 valid PNG image bytes
var validPNGBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
	0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
}

// Minimal valid JPEG header
var validJPEGBytes = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
	0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
}

func TestAvatarStorage_SaveAndMediaServing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gorangefeed_test_storage_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storage, err := NewAvatarStorage(tempDir)
	if err != nil {
		t.Fatalf("NewAvatarStorage failed: %v", err)
	}

	userID := int64(1001)

	// 1. Success case: Valid PNG
	fhPNG, err := createMultipartHeader("avatar.png", validPNGBytes)
	if err != nil {
		t.Fatalf("createMultipartHeader failed: %v", err)
	}

	url, err := storage.SaveAvatar(userID, fhPNG)
	if err != nil {
		t.Fatalf("SaveAvatar PNG failed: %v", err)
	}

	if !strings.HasPrefix(url, "/media/avatars/1001/") || !strings.HasSuffix(url, ".png") {
		t.Fatalf("unexpected media url: %s", url)
	}

	// Verify file was written to disk
	relFile := strings.TrimPrefix(url, "/media/")
	diskPath := filepath.Join(tempDir, relFile)
	if _, err := os.Stat(diskPath); err != nil {
		t.Fatalf("file not found on disk at %s: %v", diskPath, err)
	}

	// 2. Success case: Valid JPEG
	fhJPEG, err := createMultipartHeader("photo.jpg", validJPEGBytes)
	if err != nil {
		t.Fatalf("createMultipartHeader failed: %v", err)
	}
	urlJPEG, err := storage.SaveAvatar(userID, fhJPEG)
	if err != nil {
		t.Fatalf("SaveAvatar JPEG failed: %v", err)
	}
	if !strings.HasPrefix(urlJPEG, "/media/avatars/1001/") || !strings.HasSuffix(urlJPEG, ".jpg") {
		t.Fatalf("unexpected media url: %s", urlJPEG)
	}

	// 3. Error case: Disallowed file extension
	fhTxt, err := createMultipartHeader("evil.exe", validPNGBytes)
	if err != nil {
		t.Fatalf("createMultipartHeader failed: %v", err)
	}
	if _, err := storage.SaveAvatar(userID, fhTxt); err == nil {
		t.Fatalf("expected error for .exe extension")
	}

	// 4. Error case: Disguised file (ext is .png, but content is plain text)
	fhDisguised, err := createMultipartHeader("fake.png", []byte("plain text script alert(1)"))
	if err != nil {
		t.Fatalf("createMultipartHeader failed: %v", err)
	}
	if _, err := storage.SaveAvatar(userID, fhDisguised); err == nil {
		t.Fatalf("expected error for disguised image content")
	}

	// 5. Test Media Route & Range Request (HTTP 206)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.MountMediaRoutes(engine, tempDir)

	// Full content GET
	reqFull, _ := http.NewRequest(http.MethodGet, url, nil)
	wFull := httptest.NewRecorder()
	engine.ServeHTTP(wFull, reqFull)
	if wFull.Code != http.StatusOK {
		t.Fatalf("expected 200 for full media request, got %d", wFull.Code)
	}
	if !bytes.Equal(wFull.Body.Bytes(), validPNGBytes) {
		t.Fatalf("served bytes mismatch")
	}

	// Range request: first 10 bytes (HTTP 206)
	reqRange, _ := http.NewRequest(http.MethodGet, url, nil)
	reqRange.Header.Set("Range", "bytes=0-9")
	wRange := httptest.NewRecorder()
	engine.ServeHTTP(wRange, reqRange)
	if wRange.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content for range request, got %d", wRange.Code)
	}
	if wRange.Body.Len() != 10 {
		t.Fatalf("expected 10 bytes in range response, got %d", wRange.Body.Len())
	}
}

func TestAvatarStorage_FileSizeExceeded(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gorangefeed_test_storage_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storage, _ := NewAvatarStorage(tempDir)

	// Construct header with size exceeding 10MB
	fh := &multipart.FileHeader{
		Filename: "huge.png",
		Size:     MaxAvatarSize + 1,
	}

	if _, err := storage.SaveAvatar(1, fh); err == nil {
		t.Fatalf("expected error for file exceeding 10MB")
	}
}
