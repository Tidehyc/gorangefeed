package video

import (
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
)

// PublishVideoRequest represents the payload for creating and publishing a video.
type PublishVideoRequest struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	CoverURL       string `json:"cover_url"`
	PlayURL        string `json:"play_url"`
	DurationMs     int    `json:"duration_ms"`
	IdempotencyKey string `json:"idempotency_key"`
}

// UploadVideoResponse returns uploaded video media details.
type UploadVideoResponse struct {
	PlayURL string `json:"play_url"`
	Size    int64  `json:"size"`
}

// UploadCoverResponse returns uploaded video cover image details.
type UploadCoverResponse struct {
	CoverURL string `json:"cover_url"`
}

// ListAuthorVideosResponse represents paginated author works.
type ListAuthorVideosResponse struct {
	Videos     []*domainvideo.VideoDetail `json:"videos"`
	NextCursor int64                      `json:"next_cursor"`
	HasMore    bool                       `json:"has_more"`
}
