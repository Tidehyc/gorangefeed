package storage_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
)

// createMultipartFileHeader creates an in-memory *multipart.FileHeader for testing.
func createMultipartFileHeader(t *testing.T, fieldName, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("failed to write content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if err := req.ParseMultipartForm(10 << 20); err != nil {
		t.Fatalf("failed to parse multipart form: %v", err)
	}

	fileHeaders := req.MultipartForm.File[fieldName]
	if len(fileHeaders) == 0 {
		t.Fatalf("no file found for field %s", fieldName)
	}
	return fileHeaders[0]
}

func fakeMP4Bytes() []byte {
	// Standard MP4 box header: size (4 bytes), type "ftyp", major brand "mp42", minor version (4 bytes), compatible brands
	header := []byte{
		0x00, 0x00, 0x00, 0x18, // 24 bytes box size
		'f', 't', 'y', 'p', // ftyp
		'm', 'p', '4', '2', // major brand
		0x00, 0x00, 0x00, 0x00, // minor version
		'i', 's', 'o', 'm', // compatible brand 1
		'm', 'p', '4', '2', // compatible brand 2
	}
	payload := bytes.Repeat([]byte{0x01, 0x02, 0x03, 0x04}, 256)
	return append(header, payload...)
}

func fakeJPEGBytes() []byte {
	// JPEG SOI marker
	return []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x01, 0x00, 0x48}
}

func TestVideoStorage_SaveVideo(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewVideoStorage(tempDir)
	if err != nil {
		t.Fatalf("NewVideoStorage failed: %v", err)
	}

	// 1. Success with valid MP4
	validMP4 := fakeMP4Bytes()
	fh := createMultipartFileHeader(t, "file", "sample.mp4", validMP4)
	urlPath, size, err := store.SaveVideo(1001, fh)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.HasPrefix(urlPath, "/media/videos/1001/") || !strings.HasSuffix(urlPath, ".mp4") {
		t.Fatalf("unexpected media URL format: %q", urlPath)
	}
	if size != int64(len(validMP4)) {
		t.Fatalf("expected size %d, got %d", len(validMP4), size)
	}

	// Verify file was written to disk
	savedRel := strings.TrimPrefix(urlPath, "/media/")
	savedDiskPath := filepath.Join(tempDir, savedRel)
	if _, err := os.Stat(savedDiskPath); err != nil {
		t.Fatalf("expected file to exist on disk at %s: %v", savedDiskPath, err)
	}

	// 2. Reject non-mp4 extension
	fhNonMP4 := createMultipartFileHeader(t, "file", "sample.avi", validMP4)
	_, _, err = store.SaveVideo(1001, fhNonMP4)
	if err == nil {
		t.Fatal("expected error on non-mp4 extension, got nil")
	}

	// 3. Reject fake MP4 (extension .mp4 but content is plain text)
	fhFake := createMultipartFileHeader(t, "file", "fake.mp4", []byte("plain text disguised as mp4"))
	_, _, err = store.SaveVideo(1001, fhFake)
	if err == nil {
		t.Fatal("expected error on ftyp magic number mismatch, got nil")
	}

	// 4. Missing file
	_, _, err = store.SaveVideo(1001, nil)
	if err == nil {
		t.Fatal("expected error on nil file header, got nil")
	}
}

func TestVideoStorage_SaveCover(t *testing.T) {
	tempDir := t.TempDir()
	store, err := storage.NewVideoStorage(tempDir)
	if err != nil {
		t.Fatalf("NewVideoStorage failed: %v", err)
	}

	// 1. Success with valid JPEG
	validJPEG := fakeJPEGBytes()
	fh := createMultipartFileHeader(t, "file", "cover.jpg", validJPEG)
	urlPath, err := store.SaveCover(1001, fh)
	if err != nil {
		t.Fatalf("expected cover success, got %v", err)
	}
	if !strings.HasPrefix(urlPath, "/media/covers/1001/") {
		t.Fatalf("unexpected cover media URL format: %q", urlPath)
	}

	// 2. Reject invalid extension
	fhTxt := createMultipartFileHeader(t, "file", "cover.txt", []byte("not an image"))
	_, err = store.SaveCover(1001, fhTxt)
	if err == nil {
		t.Fatal("expected error on invalid extension, got nil")
	}

	// 3. Reject invalid MIME (jpg extension but plain text)
	fhBadMime := createMultipartFileHeader(t, "file", "cover.jpg", []byte("hello world"))
	_, err = store.SaveCover(1001, fhBadMime)
	if err == nil {
		t.Fatal("expected error on invalid image mime, got nil")
	}
}
