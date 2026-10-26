package video

import (
	"context"
	"errors"
	"strings"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
)

// Service defines application use cases for video publication and management.
type Service interface {
	// Publish creates and publishes a new video with initial stats, supporting idempotency.
	Publish(ctx context.Context, authorID int64, title, description, playURL, coverURL string, durationMs int, idempotencyKey string) (*domainvideo.VideoDetail, error)

	// GetDetail retrieves the public video details, engagement counts, and author profile.
	GetDetail(ctx context.Context, videoID int64, currentUserID int64) (*domainvideo.VideoDetail, error)

	// ListAuthorVideos retrieves an author's public videos using cursor pagination.
	ListAuthorVideos(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error)

	// DeleteVideo takes down a video. Only the author is permitted to perform this.
	DeleteVideo(ctx context.Context, videoID int64, currentUserID int64) error
}

type service struct {
	repo        domainvideo.Repository
	accountRepo domainaccount.Repository
}

// NewService creates a new video domain service instance.
func NewService(repo domainvideo.Repository, accountRepo domainaccount.Repository) Service {
	return &service{
		repo:        repo,
		accountRepo: accountRepo,
	}
}

func (s *service) Publish(ctx context.Context, authorID int64, title, description, playURL, coverURL string, durationMs int, idempotencyKey string) (*domainvideo.VideoDetail, error) {
	if authorID <= 0 {
		return nil, response.ErrUnauthorized("Invalid user credentials")
	}

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	playURL = strings.TrimSpace(playURL)
	coverURL = strings.TrimSpace(coverURL)
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	var idemKeyPtr *string
	if idempotencyKey != "" {
		if len(idempotencyKey) > domainvideo.MaxIdempotencyKeyLength {
			return nil, response.ErrInvalidParam("idempotency_key exceeds maximum length")
		}
		idemKeyPtr = &idempotencyKey

		// Fast-path: Check if request is an idempotent replay
		existingV, existingStats, err := s.repo.FindByAuthorAndIdempotencyKey(ctx, authorID, idempotencyKey)
		if err == nil && existingV != nil {
			return s.assembleDetail(ctx, existingV, existingStats), nil
		}
	}

	video := &domainvideo.Video{
		AuthorID:       authorID,
		Title:          title,
		Description:    description,
		PlayURL:        playURL,
		CoverURL:       coverURL,
		DurationMs:     durationMs,
		Status:         domainvideo.VideoStatusPublished,
		IdempotencyKey: idemKeyPtr,
	}

	if err := video.Validate(); err != nil {
		return nil, response.ErrInvalidParam(err.Error())
	}

	stats := &domainvideo.VideoStats{
		LikeCount:    0,
		CommentCount: 0,
		ViewCount:    0,
	}

	if err := s.repo.CreateWithStats(ctx, video, stats); err != nil {
		if errors.Is(err, domainvideo.ErrDuplicateIdempotencyKey) && idempotencyKey != "" {
			// Concurrent race: Another request with same idempotency key committed first
			existingV, existingStats, loadErr := s.repo.FindByAuthorAndIdempotencyKey(ctx, authorID, idempotencyKey)
			if loadErr == nil && existingV != nil {
				return s.assembleDetail(ctx, existingV, existingStats), nil
			}
		}
		return nil, err
	}

	return s.assembleDetail(ctx, video, stats), nil
}

func (s *service) GetDetail(ctx context.Context, videoID int64, currentUserID int64) (*domainvideo.VideoDetail, error) {
	if videoID <= 0 {
		return nil, response.ErrInvalidParam("Invalid video ID")
	}

	video, stats, err := s.repo.FindDetailByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return nil, response.ErrNotFound("Video not found or has been taken down")
		}
		return nil, err
	}

	return s.assembleDetail(ctx, video, stats), nil
}

func (s *service) ListAuthorVideos(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error) {
	if authorID <= 0 {
		return nil, 0, false, response.ErrInvalidParam("Invalid author ID")
	}
	if cursor < 0 {
		return nil, 0, false, response.ErrInvalidParam("Invalid cursor")
	}

	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	videos, stats, nextCursor, hasMore, err := s.repo.ListByAuthorCursor(ctx, authorID, cursor, limit)
	if err != nil {
		return nil, 0, false, err
	}

	authorProfile := s.getAuthorProfile(ctx, authorID)

	details := make([]*domainvideo.VideoDetail, len(videos))
	for i := range videos {
		details[i] = &domainvideo.VideoDetail{
			Video:   videos[i],
			Stats:   stats[i],
			Author:  authorProfile,
			IsLiked: false,
		}
	}

	return details, nextCursor, hasMore, nil
}

func (s *service) DeleteVideo(ctx context.Context, videoID int64, currentUserID int64) error {
	if videoID <= 0 {
		return response.ErrInvalidParam("Invalid video ID")
	}
	if currentUserID <= 0 {
		return response.ErrUnauthorized("Authentication required")
	}

	err := s.repo.UpdateStatus(ctx, videoID, currentUserID, domainvideo.VideoStatusHidden)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoForbidden) {
			return response.ErrForbidden("You are not authorized to delete this video")
		}
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return response.ErrNotFound("Video not found")
		}
		return err
	}

	return nil
}

func (s *service) getAuthorProfile(ctx context.Context, authorID int64) *domainvideo.AuthorProfile {
	profile := &domainvideo.AuthorProfile{
		ID: authorID,
	}

	if s.accountRepo != nil {
		user, err := s.accountRepo.FindByID(ctx, authorID)
		if err == nil && user != nil {
			profile.Username = user.Username
			profile.Nickname = user.Nickname
			profile.AvatarURL = user.AvatarURL
		}
	}

	return profile
}

func (s *service) assembleDetail(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) *domainvideo.VideoDetail {
	authorProfile := s.getAuthorProfile(ctx, video.AuthorID)
	return &domainvideo.VideoDetail{
		Video:   video,
		Stats:   stats,
		Author:  authorProfile,
		IsLiked: false,
	}
}
