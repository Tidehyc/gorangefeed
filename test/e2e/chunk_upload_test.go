package e2e_test

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	httpupload "github.com/Tidehyc/gorangefeed/internal/http/upload"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
)

// newChunkUploadRequest builds a multipart form request for POST /api/v1/videos/chunk/upload.
func newChunkUploadRequest(uploadID string, chunkIndex int, chunkHash string, content []byte, token string) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("upload_id", uploadID)
	_ = writer.WriteField("chunk_index", strconv.Itoa(chunkIndex))
	_ = writer.WriteField("chunk_hash", chunkHash)
	part, err := writer.CreateFormFile("file", fmt.Sprintf("chunk_%d.bin", chunkIndex))
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/videos/chunk/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func computeMD5(data []byte) string {
	return fmt.Sprintf("%x", md5.Sum(data))
}

func TestM2_5_ChunkUpload_EndToEnd(t *testing.T) {
	engine, tempDir, cleanup := setupE2ETest(t)
	if cleanup != nil {
		defer cleanup()
	}

	suffix := time.Now().Format("0405_000")
	authorUser := fmt.Sprintf("chunk_author_%s", suffix)
	viewerUser := fmt.Sprintf("chunk_viewer_%s", suffix)
	testPass := "StrongPass123"

	// ==========================================
	// Phase 1: Accounts Setup
	// ==========================================
	authorID, authorToken := registerAndLogin(t, engine, authorUser, testPass)
	_, viewerToken := registerAndLogin(t, engine, viewerUser, testPass)

	// ==========================================
	// Phase 2: Authentication & Input Validation
	// ==========================================

	// 2.1 Unauthenticated call to /init -> 401 Unauthorized
	{
		req := newJSONRequest(http.MethodPost, "/api/v1/videos/chunk/init", httpupload.InitUploadRequest{
			FileName: "test.mp4",
			FileSize: 1024,
			FileHash: "dummy",
		}, "")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for unauthenticated init, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 2.2 File size exceeding limit (> 200MB) -> 400 Bad Request
	{
		req := newJSONRequest(http.MethodPost, "/api/v1/videos/chunk/init", httpupload.InitUploadRequest{
			FileName: "giant.mp4",
			FileSize: 250 * 1024 * 1024,
			FileHash: "hash_giant",
		}, "")
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for oversized init, got %d: %s", w.Code, w.Body.String())
		}
	}

	// ==========================================
	// Phase 3: Session Init & Instant Resume
	// ==========================================
	const totalSize = 3000
	const chunkSize = 1000
	const totalChunks = 3

	// Generate synthetic MP4 bytes (chunk 0 contains ftyp box header)
	videoBytes := makeTestMP4(totalSize)
	chunk0Data := videoBytes[0:1000]
	chunk1Data := videoBytes[1000:2000]
	chunk2Data := videoBytes[2000:3000]

	chunk0MD5 := computeMD5(chunk0Data)
	chunk1MD5 := computeMD5(chunk1Data)
	chunk2MD5 := computeMD5(chunk2Data)
	fullVideoMD5 := computeMD5(videoBytes)

	// 3.1 Initial Session Init -> 200 OK, is_resumed = false
	var uploadID string
	{
		req := newJSONRequest(http.MethodPost, "/api/v1/videos/chunk/init", httpupload.InitUploadRequest{
			FileName:    "resumable_clip.mp4",
			FileSize:    totalSize,
			ChunkSize:   chunkSize,
			TotalChunks: totalChunks,
			FileHash:    fullVideoMD5,
		}, "")
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for init, got %d: %s", w.Code, w.Body.String())
		}
		var initResp struct {
			Data httpupload.InitUploadResponse `json:"data"`
		}
		parseJSONResponse(t, w, &initResp)
		uploadID = initResp.Data.UploadID
		if uploadID == "" || initResp.Data.TotalChunks != totalChunks || initResp.Data.IsResumed {
			t.Fatalf("unexpected init response: %+v", initResp.Data)
		}
		if len(initResp.Data.UploadedChunks) != 0 {
			t.Fatalf("expected 0 uploaded chunks initially, got %+v", initResp.Data.UploadedChunks)
		}
	}

	// 3.2 Second Init with identical file hash -> 200 OK, is_resumed = true, returns same upload_id
	{
		req := newJSONRequest(http.MethodPost, "/api/v1/videos/chunk/init", httpupload.InitUploadRequest{
			FileName:    "resumable_clip.mp4",
			FileSize:    totalSize,
			ChunkSize:   chunkSize,
			TotalChunks: totalChunks,
			FileHash:    fullVideoMD5,
		}, "")
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for resumed init, got %d: %s", w.Code, w.Body.String())
		}
		var initResp struct {
			Data httpupload.InitUploadResponse `json:"data"`
		}
		parseJSONResponse(t, w, &initResp)
		if initResp.Data.UploadID != uploadID || !initResp.Data.IsResumed {
			t.Fatalf("expected resumed session %s, got %+v", uploadID, initResp.Data)
		}
	}

	// ==========================================
	// Phase 4: Chunk Upload, Hash Verification & Ownership Protection
	// ==========================================

	// 4.1 Upload chunk 0 with corrupted hash -> 400 Bad Request
	{
		req, err := newChunkUploadRequest(uploadID, 0, "bad_md5_hash_00000000000000000", chunk0Data, authorToken)
		if err != nil {
			t.Fatalf("failed to create chunk upload request: %v", err)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for corrupted chunk hash, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 4.2 Upload chunk 0 with legitimate hash -> 200 OK
	{
		req, err := newChunkUploadRequest(uploadID, 0, chunk0MD5, chunk0Data, authorToken)
		if err != nil {
			t.Fatalf("failed to create chunk upload request: %v", err)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for chunk 0 upload, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 4.3 Query Status -> uploaded_chunks should now contain [0]
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/chunk/status?upload_id="+uploadID, nil)
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for status, got %d: %s", w.Code, w.Body.String())
		}
		var statusResp struct {
			Data httpupload.ChunkStatusResponse `json:"data"`
		}
		parseJSONResponse(t, w, &statusResp)
		if len(statusResp.Data.UploadedChunks) != 1 || statusResp.Data.UploadedChunks[0] != 0 {
			t.Fatalf("expected uploaded_chunks [0], got %+v", statusResp.Data.UploadedChunks)
		}
	}

	// 4.4 Repeated upload of chunk 0 -> 200 OK (idempotent fast path)
	{
		req, _ := newChunkUploadRequest(uploadID, 0, chunk0MD5, chunk0Data, authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for repeated chunk 0, got %d: %s", w.Code, w.Body.String())
		}
	}

	// 4.5 Viewer tries to upload chunk 1 to author's session -> 403 Forbidden
	{
		req, _ := newChunkUploadRequest(uploadID, 1, chunk1MD5, chunk1Data, viewerToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for unauthorized chunk upload, got %d: %s", w.Code, w.Body.String())
		}
	}

	// ==========================================
	// Phase 5: Premature Complete Check
	// ==========================================

	// Try to complete before chunks 1 and 2 are uploaded -> 400 Bad Request
	{
		req := newJSONRequest(http.MethodPost, "/api/v1/videos/chunk/complete", httpupload.CompleteUploadRequest{
			UploadID: uploadID,
		}, "")
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 when completing with missing chunks, got %d: %s", w.Code, w.Body.String())
		}
		var errResp response.Response
		parseJSONResponse(t, w, &errResp)
		if errResp.Error == nil || errResp.Error.Code != response.CodeInvalidParam {
			t.Fatalf("expected code %q, got %+v", response.CodeInvalidParam, errResp.Error)
		}
	}

	// ==========================================
	// Phase 6: Multi-chunk Concurrent Upload (Testing Bitmap atomic safety)
	// ==========================================
	{
		var wg sync.WaitGroup
		uploadErrors := make([]error, 2)

		chunks := []struct {
			index int
			hash  string
			data  []byte
		}{
			{index: 1, hash: chunk1MD5, data: chunk1Data},
			{index: 2, hash: chunk2MD5, data: chunk2Data},
		}

		for idx, c := range chunks {
			wg.Add(1)
			go func(i int, item struct {
				index int
				hash  string
				data  []byte
			}) {
				defer wg.Done()
				r, err := newChunkUploadRequest(uploadID, item.index, item.hash, item.data, authorToken)
				if err != nil {
					uploadErrors[i] = err
					return
				}
				rec := httptest.NewRecorder()
				engine.ServeHTTP(rec, r)
				if rec.Code != http.StatusOK {
					uploadErrors[i] = fmt.Errorf("chunk %d upload failed with code %d: %s", item.index, rec.Code, rec.Body.String())
				}
			}(idx, c)
		}
		wg.Wait()

		for _, err := range uploadErrors {
			if err != nil {
				t.Fatalf("concurrent chunk upload error: %v", err)
			}
		}

		// Verify status now shows all 3 chunks [0, 1, 2]
		req := httptest.NewRequest(http.MethodGet, "/api/v1/videos/chunk/status?upload_id="+uploadID, nil)
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for status, got %d", w.Code)
		}
		var statusResp struct {
			Data httpupload.ChunkStatusResponse `json:"data"`
		}
		parseJSONResponse(t, w, &statusResp)
		if len(statusResp.Data.UploadedChunks) != 3 ||
			statusResp.Data.UploadedChunks[0] != 0 ||
			statusResp.Data.UploadedChunks[1] != 1 ||
			statusResp.Data.UploadedChunks[2] != 2 {
			t.Fatalf("expected uploaded_chunks [0, 1, 2], got %+v", statusResp.Data.UploadedChunks)
		}
	}

	// ==========================================
	// Phase 7: Complete Chunk Upload & Idempotent Replay
	// ==========================================
	var playURL string
	{
		req := newJSONRequest(http.MethodPost, "/api/v1/videos/chunk/complete", httpupload.CompleteUploadRequest{
			UploadID: uploadID,
		}, "")
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for complete, got %d: %s", w.Code, w.Body.String())
		}
		var compResp struct {
			Data httpupload.CompleteUploadResponse `json:"data"`
		}
		parseJSONResponse(t, w, &compResp)
		playURL = compResp.Data.PlayURL
		if playURL == "" || !strings.HasPrefix(playURL, fmt.Sprintf("/media/videos/%d/", authorID)) {
			t.Fatalf("unexpected play_url: %s", playURL)
		}
		if compResp.Data.Size != totalSize {
			t.Fatalf("expected merged size %d, got %d", totalSize, compResp.Data.Size)
		}

		// Verify final video file exists on disk
		localVideoPath := filepath.Join(tempDir, strings.TrimPrefix(playURL, "/media/"))
		if _, err := os.Stat(localVideoPath); err != nil {
			t.Fatalf("merged video file not found on disk at %s: %v", localVideoPath, err)
		}

		// Verify temporary chunk directory is completely cleaned up
		tmpSessionDir := filepath.Join(tempDir, "tmp", uploadID)
		if _, err := os.Stat(tmpSessionDir); !os.IsNotExist(err) {
			t.Fatalf("expected tmp directory %s to be cleaned up, but it still exists", tmpSessionDir)
		}

		// Idempotent Replay: repeat POST /complete with same upload_id -> 200 OK with identical play_url
		req2 := newJSONRequest(http.MethodPost, "/api/v1/videos/chunk/complete", httpupload.CompleteUploadRequest{
			UploadID: uploadID,
		}, "")
		req2.Header.Set("Authorization", "Bearer "+authorToken)
		w2 := httptest.NewRecorder()
		engine.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Fatalf("expected 200 for idempotent replay complete, got %d: %s", w2.Code, w2.Body.String())
		}
		var compResp2 struct {
			Data httpupload.CompleteUploadResponse `json:"data"`
		}
		parseJSONResponse(t, w2, &compResp2)
		if compResp2.Data.PlayURL != playURL {
			t.Fatalf("idempotency error: expected play_url %s, got %s", playURL, compResp2.Data.PlayURL)
		}
	}

	// ==========================================
	// Phase 8: HTTP 206 Streaming of Merged Video
	// ==========================================
	{
		req := httptest.NewRequest(http.MethodGet, playURL, nil)
		req.Header.Set("Range", "bytes=0-1023")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content for merged video stream, got %d", w.Code)
		}
		expectedRange := fmt.Sprintf("bytes 0-1023/%d", totalSize)
		if w.Header().Get("Content-Range") != expectedRange {
			t.Fatalf("expected Content-Range %s, got %s", expectedRange, w.Header().Get("Content-Range"))
		}
		if w.Body.Len() != 1024 {
			t.Fatalf("expected 1024 bytes body, got %d", w.Body.Len())
		}
		if !bytes.Equal(w.Body.Bytes(), videoBytes[0:1024]) {
			t.Fatalf("stream content mismatch with original video bytes")
		}
	}

	// ==========================================
	// Phase 9: Corrupted / Non-MP4 Chunks Defense & Automatic Clean-on-failure
	// ==========================================
	{
		corruptFileHash := "corrupted_non_mp4_hash"
		corruptTextData := []byte("plain text line that is not an MP4 binary")

		// 9.1 Init corrupt session
		reqInit := newJSONRequest(http.MethodPost, "/api/v1/videos/chunk/init", httpupload.InitUploadRequest{
			FileName:    "fake.mp4",
			FileSize:    int64(len(corruptTextData)),
			ChunkSize:   len(corruptTextData),
			TotalChunks: 1,
			FileHash:    corruptFileHash,
		}, "")
		reqInit.Header.Set("Authorization", "Bearer "+authorToken)
		wInit := httptest.NewRecorder()
		engine.ServeHTTP(wInit, reqInit)
		if wInit.Code != http.StatusOK {
			t.Fatalf("failed to init corrupt session: %d", wInit.Code)
		}
		var initResp struct {
			Data httpupload.InitUploadResponse `json:"data"`
		}
		parseJSONResponse(t, wInit, &initResp)
		corruptUploadID := initResp.Data.UploadID

		// 9.2 Upload chunk with text content
		reqUp, _ := newChunkUploadRequest(corruptUploadID, 0, computeMD5(corruptTextData), corruptTextData, authorToken)
		wUp := httptest.NewRecorder()
		engine.ServeHTTP(wUp, reqUp)
		if wUp.Code != http.StatusOK {
			t.Fatalf("failed to upload chunk: %d", wUp.Code)
		}

		// 9.3 Complete should fail format sniffing (not a valid MP4) -> 400 Bad Request
		reqComp := newJSONRequest(http.MethodPost, "/api/v1/videos/chunk/complete", httpupload.CompleteUploadRequest{
			UploadID: corruptUploadID,
		}, "")
		reqComp.Header.Set("Authorization", "Bearer "+authorToken)
		wComp := httptest.NewRecorder()
		engine.ServeHTTP(wComp, reqComp)
		if wComp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for corrupted mp4 merge sniffing, got %d: %s", wComp.Code, wComp.Body.String())
		}
		var errResp response.Response
		parseJSONResponse(t, wComp, &errResp)
		if errResp.Error == nil || errResp.Error.Code != response.CodeInvalidParam {
			t.Fatalf("expected code %q, got %+v", response.CodeInvalidParam, errResp.Error)
		}
	}
}
