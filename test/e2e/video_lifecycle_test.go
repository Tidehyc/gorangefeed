package e2e_test

import (
	"bytes"
	"fmt"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpaccount "github.com/Tidehyc/gorangefeed/internal/http/account"
	httpvideo "github.com/Tidehyc/gorangefeed/internal/http/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/gin-gonic/gin"
)

// registerAndLogin registers a new user and logs in, returning (userID, accessToken).
func registerAndLogin(t *testing.T, engine *gin.Engine, username, password string) (int64, string) {
	t.Helper()
	ip := fmt.Sprintf("192.168.20.%d", time.Now().Nanosecond()%200+10)

	// 1. Register
	wReg := httptest.NewRecorder()
	engine.ServeHTTP(wReg, newJSONRequest(http.MethodPost, "/api/v1/auth/register", httpaccount.RegisterRequest{
		Username: username,
		Password: password,
	}, ip))
	if wReg.Code != http.StatusCreated {
		t.Fatalf("failed to register user %s: code=%d, body=%s", username, wReg.Code, wReg.Body.String())
	}
	var regResp struct {
		Data httpaccount.UserProfileResponse `json:"data"`
	}
	parseJSONResponse(t, wReg, &regResp)
	userID := regResp.Data.ID

	// 2. Login
	wLogin := httptest.NewRecorder()
	engine.ServeHTTP(wLogin, newJSONRequest(http.MethodPost, "/api/v1/auth/login", httpaccount.LoginRequest{
		Username: username,
		Password: password,
	}, ip))
	if wLogin.Code != http.StatusOK {
		t.Fatalf("failed to login user %s: code=%d, body=%s", username, wLogin.Code, wLogin.Body.String())
	}
	var loginResp struct {
		Data httpaccount.LoginResponse `json:"data"`
	}
	parseJSONResponse(t, wLogin, &loginResp)

	return userID, loginResp.Data.Tokens.AccessToken
}

// newMultipartUploadRequest constructs an HTTP multipart/form-data upload request.
func newMultipartUploadRequest(method, path, fieldName, filename string, fileContent []byte, token string) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(fileContent); err != nil {
		return nil, fmt.Errorf("write part content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

// makeTestMP4 generates a synthetic MP4 buffer with ISO/IEC 14496-12 ftyp box header.
func makeTestMP4(size int) []byte {
	if size < 16 {
		size = 16
	}
	buf := make([]byte, size)
	// Box size: 16 bytes
	buf[0] = 0x00
	buf[1] = 0x00
	buf[2] = 0x00
	buf[3] = 0x10
	// Major brand box type: "ftyp"
	copy(buf[4:8], "ftyp")
	// Compatible brand: "isom"
	copy(buf[8:12], "isom")
	// Fill remaining bytes with deterministic pattern
	for i := 12; i < size; i++ {
		buf[i] = byte(i % 256)
	}
	return buf
}

// makeTestPNG generates a synthetic PNG buffer with standard 8-byte PNG signature.
func makeTestPNG() []byte {
	header := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	buf := make([]byte, 256)
	copy(buf, header)
	for i := len(header); i < len(buf); i++ {
		buf[i] = byte(i % 128)
	}
	return buf
}

// countFilesWithExt counts files with a specific extension under a directory tree.
func countFilesWithExt(root, ext string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ext) {
			count++
		}
		return nil
	})
	return count, err
}

func TestM2_VideoLifecycle_EndToEnd(t *testing.T) {
	engine, tempDir, cleanup := setupE2ETest(t)
	if cleanup != nil {
		defer cleanup()
	}

	suffix := time.Now().Format("0405_000")
	authorUser := fmt.Sprintf("author_%s", suffix)
	viewerUser := fmt.Sprintf("viewer_%s", suffix)
	testPass := "StrongPass123"

	// ==========================================
	// Phase 1: User Registration & Authentication
	// ==========================================
	authorID, authorToken := registerAndLogin(t, engine, authorUser, testPass)
	viewerID, viewerToken := registerAndLogin(t, engine, viewerUser, testPass)
	if authorID <= 0 || authorToken == "" {
		t.Fatalf("invalid author credentials: id=%d, token=%s", authorID, authorToken)
	}
	if viewerID <= 0 || viewerToken == "" {
		t.Fatalf("invalid viewer credentials: id=%d, token=%s", viewerID, viewerToken)
	}

	// ==========================================
	// Phase 2: Upload Validation & Magic Number & Dirty File Cleanup
	// ==========================================

	// 2.1 Unauthenticated upload -> 401 Unauthorized
	{
		req, err := newMultipartUploadRequest(http.MethodPost, "/api/v1/videos/upload", "file", "test.mp4", makeTestMP4(1024), "")
		if err != nil {
			t.Fatalf("failed to create upload request: %v", err)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated upload, got %d: %s", w.Code, w.Body.String())
		}
		var errResp response.Response
		parseJSONResponse(t, w, &errResp)
		if errResp.Error == nil || errResp.Error.Code != response.CodeUnauthorized {
			t.Fatalf("expected code %q, got %+v", response.CodeUnauthorized, errResp.Error)
		}
	}

	// 2.2 Upload Fake MP4 (text disguised as .mp4) -> 400 Bad Request + verify no dirty file on disk
	{
		fakeMP4Content := []byte("This is plain text and definitely NOT an MP4 binary payload.")
		req, err := newMultipartUploadRequest(http.MethodPost, "/api/v1/videos/upload", "file", "fake.mp4", fakeMP4Content, authorToken)
		if err != nil {
			t.Fatalf("failed to create fake mp4 request: %v", err)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for fake mp4 upload, got %d: %s", w.Code, w.Body.String())
		}
		var errResp response.Response
		parseJSONResponse(t, w, &errResp)
		if errResp.Error == nil || errResp.Error.Code != response.CodeInvalidParam {
			t.Fatalf("expected code %q, got %+v", response.CodeInvalidParam, errResp.Error)
		}

		// Verify clean-on-failure: no orphaned .mp4 files remain in storage directory
		mp4Count, err := countFilesWithExt(filepath.Join(tempDir, "videos"), ".mp4")
		if err != nil {
			t.Fatalf("failed to scan videos directory: %v", err)
		}
		if mp4Count != 0 {
			t.Fatalf("dirty file leak: expected 0 .mp4 files after failed upload, found %d", mp4Count)
		}
	}

	// 2.3 Upload Fake Cover -> 400 Bad Request
	{
		fakeCoverContent := []byte("Not an image at all.")
		req, err := newMultipartUploadRequest(http.MethodPost, "/api/v1/videos/cover", "file", "cover.txt", fakeCoverContent, authorToken)
		if err != nil {
			t.Fatalf("failed to create fake cover request: %v", err)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for fake cover upload, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 2.4 Legitimate MP4 Video 1 Upload -> 201 Created
	const video1Size = 2048
	video1Bytes := makeTestMP4(video1Size)
	var playURL1 string
	{
		req, err := newMultipartUploadRequest(http.MethodPost, "/api/v1/videos/upload", "file", "video1.mp4", video1Bytes, authorToken)
		if err != nil {
			t.Fatalf("failed to create valid video request: %v", err)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for valid video upload, got %d: %s", w.Code, w.Body.String())
		}
		var upResp struct {
			Data httpvideo.UploadVideoResponse `json:"data"`
		}
		parseJSONResponse(t, w, &upResp)
		playURL1 = upResp.Data.PlayURL
		if !strings.HasPrefix(playURL1, fmt.Sprintf("/media/videos/%d/", authorID)) || !strings.HasSuffix(playURL1, ".mp4") {
			t.Fatalf("unexpected playURL: %s", playURL1)
		}
		if upResp.Data.Size != video1Size {
			t.Fatalf("expected size %d, got %d", video1Size, upResp.Data.Size)
		}

		// Verify file exists on local disk
		localVideoPath := filepath.Join(tempDir, strings.TrimPrefix(playURL1, "/media/"))
		if _, err := os.Stat(localVideoPath); err != nil {
			t.Fatalf("uploaded video file does not exist on disk at %s: %v", localVideoPath, err)
		}
	}

	// 2.5 Legitimate Cover Upload -> 201 Created
	coverBytes := makeTestPNG()
	var coverURL1 string
	{
		req, err := newMultipartUploadRequest(http.MethodPost, "/api/v1/videos/cover", "file", "cover.png", coverBytes, authorToken)
		if err != nil {
			t.Fatalf("failed to create cover upload request: %v", err)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for valid cover upload, got %d: %s", w.Code, w.Body.String())
		}
		var upResp struct {
			Data httpvideo.UploadCoverResponse `json:"data"`
		}
		parseJSONResponse(t, w, &upResp)
		coverURL1 = upResp.Data.CoverURL
		if !strings.HasPrefix(coverURL1, fmt.Sprintf("/media/covers/%d/", authorID)) || !strings.HasSuffix(coverURL1, ".png") {
			t.Fatalf("unexpected coverURL: %s", coverURL1)
		}

		// Verify cover file exists on disk
		localCoverPath := filepath.Join(tempDir, strings.TrimPrefix(coverURL1, "/media/"))
		if _, err := os.Stat(localCoverPath); err != nil {
			t.Fatalf("uploaded cover file does not exist on disk at %s: %v", localCoverPath, err)
		}
	}

	// ==========================================
	// Phase 3: Video Publish & Idempotency
	// ==========================================

	// 3.1 Publish with invalid params (empty title) -> 400 Bad Request
	{
		req := newJSONRequest(http.MethodPost, "/api/v1/videos", httpvideo.PublishVideoRequest{
			Title:   "",
			PlayURL: playURL1,
		}, "")
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty title, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 3.2 Publish Video 1 with Idempotency-Key -> 201 Created
	idempotencyKey1 := fmt.Sprintf("e2e_idem_key_1_%s", suffix)
	var video1ID int64
	{
		req := newJSONRequest(http.MethodPost, "/api/v1/videos", httpvideo.PublishVideoRequest{
			Title:       "First GoRangeFeed E2E Video",
			Description: "Full lifecycle integration test video 1",
			PlayURL:     playURL1,
			CoverURL:    coverURL1,
			DurationMs:  18000,
		}, "")
		req.Header.Set("Authorization", "Bearer "+authorToken)
		req.Header.Set("Idempotency-Key", idempotencyKey1)

		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for video publish, got %d: %s", w.Code, w.Body.String())
		}
		var pubResp struct {
			Data *domainvideo.VideoDetail `json:"data"`
		}
		parseJSONResponse(t, w, &pubResp)
		if pubResp.Data == nil || pubResp.Data.Video == nil || pubResp.Data.Video.ID <= 0 {
			t.Fatalf("invalid publish response: %+v", pubResp)
		}
		video1ID = pubResp.Data.Video.ID

		// Verify fields and default stats
		if pubResp.Data.Video.AuthorID != authorID || pubResp.Data.Video.Title != "First GoRangeFeed E2E Video" {
			t.Fatalf("unexpected video data: %+v", pubResp.Data.Video)
		}
		if pubResp.Data.Author == nil || pubResp.Data.Author.Username != authorUser {
			t.Fatalf("unexpected author profile: %+v", pubResp.Data.Author)
		}
		if pubResp.Data.Stats == nil || pubResp.Data.Stats.ViewCount != 0 || pubResp.Data.Stats.LikeCount != 0 {
			t.Fatalf("unexpected stats initialization: %+v", pubResp.Data.Stats)
		}
		if pubResp.Data.Video.Status != domainvideo.VideoStatusPublished {
			t.Fatalf("expected status %v, got %v", domainvideo.VideoStatusPublished, pubResp.Data.Video.Status)
		}
	}

	// 3.3 Idempotency Replay: Send identical publish request with same Idempotency-Key -> returns same video (201 Created)
	{
		req := newJSONRequest(http.MethodPost, "/api/v1/videos", httpvideo.PublishVideoRequest{
			Title:       "Changed Title Attempt",
			Description: "This should NOT override the original video",
			PlayURL:     playURL1,
			CoverURL:    coverURL1,
			DurationMs:  18000,
		}, "")
		req.Header.Set("Authorization", "Bearer "+authorToken)
		req.Header.Set("Idempotency-Key", idempotencyKey1)

		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for idempotent replay, got %d: %s", w.Code, w.Body.String())
		}
		var pubResp struct {
			Data *domainvideo.VideoDetail `json:"data"`
		}
		parseJSONResponse(t, w, &pubResp)
		if pubResp.Data.Video == nil || pubResp.Data.Video.ID != video1ID {
			t.Fatalf("idempotency violation: expected existing ID %d, got %+v", video1ID, pubResp.Data)
		}
		if pubResp.Data.Video.Title != "First GoRangeFeed E2E Video" {
			t.Fatalf("idempotency violation: original title was mutated to %s", pubResp.Data.Video.Title)
		}
	}

	// 3.4 Upload and Publish Video 2 (for cursor pagination testing)
	const video2Size = 3072
	video2Bytes := makeTestMP4(video2Size)
	var video2ID int64
	{
		reqUp, err := newMultipartUploadRequest(http.MethodPost, "/api/v1/videos/upload", "file", "video2.mp4", video2Bytes, authorToken)
		if err != nil {
			t.Fatalf("failed to create video 2 upload request: %v", err)
		}
		wUp := httptest.NewRecorder()
		engine.ServeHTTP(wUp, reqUp)
		if wUp.Code != http.StatusCreated {
			t.Fatalf("failed to upload video 2: code=%d, body=%s", wUp.Code, wUp.Body.String())
		}
		var upResp struct {
			Data httpvideo.UploadVideoResponse `json:"data"`
		}
		parseJSONResponse(t, wUp, &upResp)
		playURL2 := upResp.Data.PlayURL

		idempotencyKey2 := fmt.Sprintf("e2e_idem_key_2_%s", suffix)
		reqPub := newJSONRequest(http.MethodPost, "/api/v1/videos", httpvideo.PublishVideoRequest{
			Title:       "Second GoRangeFeed E2E Video",
			Description: "Second video for cursor pagination testing",
			PlayURL:     playURL2,
			CoverURL:    coverURL1,
			DurationMs:  25000,
		}, "")
		reqPub.Header.Set("Authorization", "Bearer "+authorToken)
		reqPub.Header.Set("Idempotency-Key", idempotencyKey2)

		wPub := httptest.NewRecorder()
		engine.ServeHTTP(wPub, reqPub)
		if wPub.Code != http.StatusCreated {
			t.Fatalf("failed to publish video 2: code=%d, body=%s", wPub.Code, wPub.Body.String())
		}
		var pubResp struct {
			Data *domainvideo.VideoDetail `json:"data"`
		}
		parseJSONResponse(t, wPub, &pubResp)
		if pubResp.Data == nil || pubResp.Data.Video == nil {
			t.Fatalf("invalid publish video 2 response: %+v", pubResp)
		}
		video2ID = pubResp.Data.Video.ID
		if video2ID <= video1ID {
			t.Fatalf("expected monotonically increasing ID: video2ID (%d) <= video1ID (%d)", video2ID, video1ID)
		}
	}

	// ==========================================
	// Phase 4: HTTP 206 Range Stream Serving
	// ==========================================

	// 4.1 Full Content Request (GET playURL1) -> 200 OK
	{
		req := httptest.NewRequest(http.MethodGet, playURL1, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for full video streaming, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Accept-Ranges") != "bytes" {
			t.Fatalf("expected Accept-Ranges: bytes, got %q", w.Header().Get("Accept-Ranges"))
		}
		if int64(w.Body.Len()) != video1Size {
			t.Fatalf("expected body length %d, got %d", video1Size, w.Body.Len())
		}
		if !bytes.Equal(w.Body.Bytes(), video1Bytes) {
			t.Fatalf("full streaming body bytes mismatch with original uploaded bytes")
		}
	}

	// 4.2 First Range Request (bytes 0-1023) -> 206 Partial Content
	{
		req := httptest.NewRequest(http.MethodGet, playURL1, nil)
		req.Header.Set("Range", "bytes=0-1023")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content for range bytes=0-1023, got %d", w.Code)
		}
		expectedRangeHeader := fmt.Sprintf("bytes 0-1023/%d", video1Size)
		if w.Header().Get("Content-Range") != expectedRangeHeader {
			t.Fatalf("expected Content-Range %q, got %q", expectedRangeHeader, w.Header().Get("Content-Range"))
		}
		if w.Body.Len() != 1024 {
			t.Fatalf("expected 1024 bytes body, got %d", w.Body.Len())
		}
		if !bytes.Equal(w.Body.Bytes(), video1Bytes[0:1024]) {
			t.Fatalf("chunk 0-1023 content mismatch with source binary")
		}
	}

	// 4.3 Second Range Request (bytes 1024-2047) -> 206 Partial Content
	{
		req := httptest.NewRequest(http.MethodGet, playURL1, nil)
		req.Header.Set("Range", "bytes=1024-2047")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content for range bytes=1024-2047, got %d", w.Code)
		}
		expectedRangeHeader := fmt.Sprintf("bytes 1024-2047/%d", video1Size)
		if w.Header().Get("Content-Range") != expectedRangeHeader {
			t.Fatalf("expected Content-Range %q, got %q", expectedRangeHeader, w.Header().Get("Content-Range"))
		}
		if w.Body.Len() != 1024 {
			t.Fatalf("expected 1024 bytes body, got %d", w.Body.Len())
		}
		if !bytes.Equal(w.Body.Bytes(), video1Bytes[1024:2048]) {
			t.Fatalf("chunk 1024-2047 content mismatch with source binary")
		}
	}

	// 4.4 Out-of-bounds Range Request (bytes 5000-6000) -> 416 Range Not Satisfiable
	{
		req := httptest.NewRequest(http.MethodGet, playURL1, nil)
		req.Header.Set("Range", "bytes=5000-6000")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Fatalf("expected 416 for out-of-bounds range, got %d", w.Code)
		}
		expectedRangeHeader := fmt.Sprintf("bytes */%d", video1Size)
		if w.Header().Get("Content-Range") != expectedRangeHeader {
			t.Fatalf("expected Content-Range %q, got %q", expectedRangeHeader, w.Header().Get("Content-Range"))
		}
	}

	// ==========================================
	// Phase 5: Video Detail & Author Videos Cursor Pagination
	// ==========================================

	// 5.1 Video Detail (Anonymous SoftJWTAuth) -> 200 OK
	{
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d", video1ID), nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for anonymous video detail, got %d: %s", w.Code, w.Body.String())
		}
		var detailResp struct {
			Data *domainvideo.VideoDetail `json:"data"`
		}
		parseJSONResponse(t, w, &detailResp)
		if detailResp.Data.Video == nil || detailResp.Data.Video.ID != video1ID || detailResp.Data.IsLiked {
			t.Fatalf("unexpected detail: %+v", detailResp.Data)
		}
		if detailResp.Data.Author == nil || detailResp.Data.Author.ID != authorID {
			t.Fatalf("unexpected author in detail: %+v", detailResp.Data.Author)
		}
	}

	// 5.2 Video Detail (Viewer SoftJWTAuth) -> 200 OK
	{
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d", video1ID), nil)
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for viewer video detail, got %d: %s", w.Code, w.Body.String())
		}
		var detailResp struct {
			Data *domainvideo.VideoDetail `json:"data"`
		}
		parseJSONResponse(t, w, &detailResp)
		if detailResp.Data.Video == nil || detailResp.Data.Video.ID != video1ID {
			t.Fatalf("video ID mismatch: expected %d, got %+v", video1ID, detailResp.Data)
		}
	}

	// 5.3 Non-existent Video Detail -> 404 Not Found
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/999999999", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-existent video, got %d: %s", w.Code, w.Body.String())
		}
		var errResp response.Response
		parseJSONResponse(t, w, &errResp)
		if errResp.Error == nil || errResp.Error.Code != response.CodeNotFound {
			t.Fatalf("expected code %q, got %+v", response.CodeNotFound, errResp.Error)
		}
	}

	// 5.4 Author Works Stream Pagination (cursor pagination)
	// Page 1: cursor=0, limit=1 -> returns video2 (newest first), has_more=true, next_cursor=video2ID
	var nextCursor int64
	{
		url := fmt.Sprintf("/api/v1/users/%d/videos?cursor=0&limit=1", authorID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for author videos page 1, got %d: %s", w.Code, w.Body.String())
		}
		var listResp struct {
			Data httpvideo.ListAuthorVideosResponse `json:"data"`
		}
		parseJSONResponse(t, w, &listResp)
		if len(listResp.Data.Videos) != 1 {
			t.Fatalf("expected 1 video in page 1, got %d", len(listResp.Data.Videos))
		}
		if listResp.Data.Videos[0].Video == nil || listResp.Data.Videos[0].Video.ID != video2ID {
			t.Fatalf("expected first item to be newest video %d, got %+v", video2ID, listResp.Data.Videos[0])
		}
		if !listResp.Data.HasMore {
			t.Fatalf("expected has_more to be true for page 1")
		}
		if listResp.Data.NextCursor != video2ID {
			t.Fatalf("expected next_cursor %d, got %d", video2ID, listResp.Data.NextCursor)
		}
		nextCursor = listResp.Data.NextCursor
	}

	// Page 2: cursor=video2ID, limit=1 -> returns video1, has_more=false, next_cursor=0
	{
		url := fmt.Sprintf("/api/v1/users/%d/videos?cursor=%d&limit=1", authorID, nextCursor)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for author videos page 2, got %d: %s", w.Code, w.Body.String())
		}
		var listResp struct {
			Data httpvideo.ListAuthorVideosResponse `json:"data"`
		}
		parseJSONResponse(t, w, &listResp)
		if len(listResp.Data.Videos) != 1 {
			t.Fatalf("expected 1 video in page 2, got %d", len(listResp.Data.Videos))
		}
		if listResp.Data.Videos[0].Video == nil || listResp.Data.Videos[0].Video.ID != video1ID {
			t.Fatalf("expected second item to be video %d, got %+v", video1ID, listResp.Data.Videos[0])
		}
		if listResp.Data.HasMore {
			t.Fatalf("expected has_more to be false at end of stream")
		}
		if listResp.Data.NextCursor != 0 {
			t.Fatalf("expected next_cursor=0 at end of stream, got %d", listResp.Data.NextCursor)
		}
	}

	// 5.5 Viewer Works Stream (no videos published) -> empty list
	{
		url := fmt.Sprintf("/api/v1/users/%d/videos", viewerID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for viewer videos, got %d: %s", w.Code, w.Body.String())
		}
		var listResp struct {
			Data httpvideo.ListAuthorVideosResponse `json:"data"`
		}
		parseJSONResponse(t, w, &listResp)
		if len(listResp.Data.Videos) != 0 || listResp.Data.HasMore {
			t.Fatalf("expected empty list for viewer, got %+v", listResp.Data)
		}
	}

	// ==========================================
	// Phase 6: Video Takedown (Soft Delete) & Integrity
	// ==========================================

	// 6.1 Unauthorized Takedown Attempt: Viewer tries to take down Author's video -> 403 Forbidden
	{
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d", video1ID), nil)
		req.Header.Set("Authorization", "Bearer "+viewerToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for unauthorized video delete, got %d: %s", w.Code, w.Body.String())
		}
		var errResp response.Response
		parseJSONResponse(t, w, &errResp)
		if errResp.Error == nil || errResp.Error.Code != response.CodeForbidden {
			t.Fatalf("expected code %q, got %+v", response.CodeForbidden, errResp.Error)
		}
	}

	// 6.2 Legitimate Takedown: Author deletes own video -> 200 OK
	{
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d", video1ID), nil)
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for author delete, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 6.3 Takedown Idempotency: Author deletes already deleted video -> 200 OK
	{
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d", video1ID), nil)
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for idempotent delete, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 6.4 Query Deleted Video Detail -> 404 Not Found
	{
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d", video1ID), nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for taken-down video detail, got %d: %s", w.Code, w.Body.String())
		}
		var errResp response.Response
		parseJSONResponse(t, w, &errResp)
		if errResp.Error == nil || errResp.Error.Code != response.CodeNotFound {
			t.Fatalf("expected code %q, got %+v", response.CodeNotFound, errResp.Error)
		}
	}

	// 6.5 Author Works Stream: Deleted video is excluded, only video 2 is returned
	{
		url := fmt.Sprintf("/api/v1/users/%d/videos?limit=10", authorID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for author videos after takedown, got %d: %s", w.Code, w.Body.String())
		}
		var listResp struct {
			Data httpvideo.ListAuthorVideosResponse `json:"data"`
		}
		parseJSONResponse(t, w, &listResp)
		if len(listResp.Data.Videos) != 1 {
			t.Fatalf("expected 1 video remaining in author stream, got %d", len(listResp.Data.Videos))
		}
		if listResp.Data.Videos[0].Video == nil || listResp.Data.Videos[0].Video.ID != video2ID {
			t.Fatalf("expected only video %d in author stream, got %+v", video2ID, listResp.Data.Videos[0])
		}
		if listResp.Data.HasMore {
			t.Fatalf("expected has_more to be false")
		}
	}
}
