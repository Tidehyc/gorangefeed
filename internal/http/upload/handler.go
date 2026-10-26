package upload

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	serviceupload "github.com/Tidehyc/gorangefeed/internal/service/upload"
	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for chunked uploads and resumable transfers.
type Handler struct {
	svc serviceupload.Service
}

// NewHandler creates a new chunked upload Handler instance.
func NewHandler(svc serviceupload.Service) *Handler {
	return &Handler{svc: svc}
}

func mapUploadError(err error) error {
	switch {
	case errors.Is(err, domainupload.ErrSessionNotFound):
		return response.ErrNotFound("Upload session not found")
	case errors.Is(err, domainupload.ErrSessionForbidden):
		return response.ErrForbidden("Access denied: session belongs to another user")
	case errors.Is(err, domainupload.ErrSessionExpired):
		return response.ErrConflict("Upload session expired")
	case errors.Is(err, domainupload.ErrSessionNotActive):
		return response.ErrConflict("Upload session is not in uploading status")
	case errors.Is(err, domainupload.ErrMergeConflict):
		return response.ErrConflict("Concurrent merge conflict: session is already merging or completed")
	case errors.Is(err, domainupload.ErrMissingChunks):
		return response.ErrInvalidParam("Cannot complete: not all chunks have been uploaded")
	case errors.Is(err, domainupload.ErrCorruptedMergedFile):
		return response.ErrInvalidParam("Merged video format validation failed: invalid MP4")
	case errors.Is(err, domainupload.ErrChunkHashMismatch):
		return response.ErrInvalidParam("Chunk hash mismatch")
	case errors.Is(err, domainupload.ErrInvalidChunkIndex):
		return response.ErrInvalidParam("Invalid chunk index")
	case errors.Is(err, domainupload.ErrFileSizeExceeded):
		return response.ErrInvalidParam("File size exceeds 200MB limit")
	case errors.Is(err, domainupload.ErrInvalidFileSize):
		return response.ErrInvalidParam("File size must be positive")
	case errors.Is(err, domainupload.ErrInvalidTotalChunks):
		return response.ErrInvalidParam("Invalid total chunks")
	case errors.Is(err, domainupload.ErrInvalidChunkSize):
		return response.ErrInvalidParam("Invalid chunk size")
	case errors.Is(err, domainupload.ErrInvalidFileHash):
		return response.ErrInvalidParam("File hash cannot be empty")
	case errors.Is(err, domainupload.ErrEmptyFileName):
		return response.ErrInvalidParam("File name cannot be empty")
	default:
		return err
	}
}

// InitSession handles POST /api/v1/videos/chunk/init.
func (h *Handler) InitSession(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	var req InitUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request body: "+err.Error()))
		return
	}

	res, err := h.svc.InitSession(
		c.Request.Context(),
		userID,
		req.FileName,
		req.FileSize,
		req.ChunkSize,
		req.TotalChunks,
		req.FileHash,
	)
	if err != nil {
		response.Fail(c, mapUploadError(err))
		return
	}

	response.Success(c, http.StatusOK, InitUploadResponse{
		UploadID:       res.UploadID,
		UploadedChunks: res.UploadedChunks,
		TotalChunks:    res.TotalChunks,
		IsResumed:      res.IsResumed,
	})
}

// UploadChunk handles POST /api/v1/videos/chunk/upload.
func (h *Handler) UploadChunk(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	uploadID := strings.TrimSpace(c.PostForm("upload_id"))
	if uploadID == "" {
		response.Fail(c, response.ErrInvalidParam("Missing upload_id parameter"))
		return
	}

	chunkIndexStr := strings.TrimSpace(c.PostForm("chunk_index"))
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid chunk_index parameter"))
		return
	}

	chunkHash := strings.TrimSpace(c.PostForm("chunk_hash"))
	if chunkHash == "" {
		response.Fail(c, response.ErrInvalidParam("Missing chunk_hash parameter"))
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, response.ErrInvalidParam("Missing or invalid chunk file: "+err.Error()))
		return
	}

	src, err := file.Open()
	if err != nil {
		response.Fail(c, response.ErrInternal("Failed to read chunk file"))
		return
	}
	defer src.Close()

	if err := h.svc.UploadChunk(c.Request.Context(), userID, uploadID, chunkIndex, chunkHash, src); err != nil {
		response.Fail(c, mapUploadError(err))
		return
	}

	response.OK(c, UploadChunkResponse{ChunkIndex: chunkIndex})
}

// GetStatus handles GET /api/v1/videos/chunk/status.
func (h *Handler) GetStatus(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	uploadID := strings.TrimSpace(c.Query("upload_id"))
	if uploadID == "" {
		response.Fail(c, response.ErrInvalidParam("Missing upload_id query parameter"))
		return
	}

	res, err := h.svc.GetStatus(c.Request.Context(), userID, uploadID)
	if err != nil {
		response.Fail(c, mapUploadError(err))
		return
	}

	response.OK(c, ChunkStatusResponse{
		UploadID:       res.UploadID,
		TotalChunks:    res.TotalChunks,
		UploadedChunks: res.UploadedChunks,
		Status:         res.Status,
		PlayURL:        res.PlayURL,
	})
}

// Complete handles POST /api/v1/videos/chunk/complete.
func (h *Handler) Complete(c *gin.Context) {
	userID := c.GetInt64(middleware.ContextKeyUserID)
	if userID <= 0 {
		response.Fail(c, response.ErrUnauthorized("Authentication required"))
		return
	}

	var req CompleteUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request body: "+err.Error()))
		return
	}

	res, err := h.svc.Complete(c.Request.Context(), userID, req.UploadID)
	if err != nil {
		response.Fail(c, mapUploadError(err))
		return
	}

	response.OK(c, CompleteUploadResponse{
		UploadID: res.UploadID,
		PlayURL:  res.PlayURL,
		Size:     res.Size,
	})
}
