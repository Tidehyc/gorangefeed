package interaction

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domaininteraction "github.com/Tidehyc/gorangefeed/internal/domain/interaction"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
)

// Service coordinates interaction business use cases.
type Service struct {
	interactionRepo domaininteraction.Repository
	videoRepo       domainvideo.Repository
	accountRepo     domainaccount.Repository
}

// NewService creates a new interaction Service.
func NewService(
	interactionRepo domaininteraction.Repository,
	videoRepo domainvideo.Repository,
	accountRepo domainaccount.Repository,
) *Service {
	return &Service{
		interactionRepo: interactionRepo,
		videoRepo:       videoRepo,
		accountRepo:     accountRepo,
	}
}

// LikeVideo handles liking a video, generating outbox event.
func (s *Service) LikeVideo(ctx context.Context, userID, videoID int64) error {
	if userID <= 0 {
		return domaininteraction.ErrInvalidUserID
	}
	if videoID <= 0 {
		return domaininteraction.ErrInvalidVideoID
	}

	// Verify video exists and is published
	v, err := s.videoRepo.FindByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return domaininteraction.ErrVideoNotFound
		}
		return fmt.Errorf("find video error: %w", err)
	}
	if !v.IsPublished() {
		return domaininteraction.ErrVideoNotFound
	}

	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeInteractionLiked, map[string]any{
		"user_id":   userID,
		"video_id":  videoID,
		"author_id": v.AuthorID,
		"liked_at":  time.Now(),
	})
	if err != nil {
		return fmt.Errorf("create outbox event error: %w", err)
	}

	_, err = s.interactionRepo.LikeVideo(ctx, userID, videoID, outboxEvent)
	return err
}

// UnlikeVideo handles unliking a video.
func (s *Service) UnlikeVideo(ctx context.Context, userID, videoID int64) error {
	if userID <= 0 {
		return domaininteraction.ErrInvalidUserID
	}
	if videoID <= 0 {
		return domaininteraction.ErrInvalidVideoID
	}

	v, err := s.videoRepo.FindByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return domaininteraction.ErrVideoNotFound
		}
		return fmt.Errorf("find video error: %w", err)
	}

	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeInteractionUnliked, map[string]any{
		"user_id":    userID,
		"video_id":   videoID,
		"author_id":  v.AuthorID,
		"unliked_at": time.Now(),
	})
	if err != nil {
		return fmt.Errorf("create outbox event error: %w", err)
	}

	_, err = s.interactionRepo.UnlikeVideo(ctx, userID, videoID, outboxEvent)
	return err
}

// ListMyLikedVideos returns a cursor-paginated list of videos liked by the current user.
func (s *Service) ListMyLikedVideos(ctx context.Context, userID int64, cursor int64, limit int) ([]*domainvideo.VideoDetail, int64, bool, error) {
	if userID <= 0 {
		return nil, 0, false, domaininteraction.ErrInvalidUserID
	}
	if limit <= 0 {
		limit = 10
	}

	videoIDs, nextCursor, hasMore, err := s.interactionRepo.ListMyLikedVideoIDs(ctx, userID, cursor, limit)
	if err != nil {
		return nil, 0, false, err
	}

	var details []*domainvideo.VideoDetail
	for _, vid := range videoIDs {
		v, stats, err := s.videoRepo.FindDetailByID(ctx, vid)
		if err != nil {
			// Video may be deleted or hidden, skip
			continue
		}
		if !v.IsPublished() {
			continue
		}

		var authorProfile *domainvideo.AuthorProfile
		if s.accountRepo != nil {
			if u, err := s.accountRepo.FindByID(ctx, v.AuthorID); err == nil && u != nil {
				authorProfile = &domainvideo.AuthorProfile{
					ID:        u.ID,
					Username:  u.Username,
					Nickname:  u.Nickname,
					AvatarURL: u.AvatarURL,
				}
			}
		}

		details = append(details, &domainvideo.VideoDetail{
			Video:   v,
			Stats:   stats,
			Author:  authorProfile,
			IsLiked: true, // In my-likes list, it's liked by default
		})
	}

	return details, nextCursor, hasMore, nil
}

// CreateComment publishes a new comment on a video with idempotency guarantee.
func (s *Service) CreateComment(ctx context.Context, userID, videoID, parentID int64, content, idempotencyKey string) (*domaininteraction.CommentDetail, bool, error) {
	v, err := s.videoRepo.FindByID(ctx, videoID)
	if err != nil {
		if errors.Is(err, domainvideo.ErrVideoNotFound) {
			return nil, false, domaininteraction.ErrVideoNotFound
		}
		return nil, false, fmt.Errorf("find video error: %w", err)
	}
	if !v.IsPublished() {
		return nil, false, domaininteraction.ErrVideoNotFound
	}

	// Validate parent comment if specified
	if parentID > 0 {
		parent, err := s.interactionRepo.FindCommentByID(ctx, parentID)
		if err != nil {
			if errors.Is(err, domaininteraction.ErrCommentNotFound) {
				return nil, false, domaininteraction.ErrInvalidParentComment
			}
			return nil, false, err
		}
		if !parent.IsPublished() || parent.VideoID != videoID {
			return nil, false, domaininteraction.ErrInvalidParentComment
		}
	}

	comment := &domaininteraction.Comment{
		VideoID:        videoID,
		UserID:         userID,
		ParentID:       parentID,
		Content:        strings.TrimSpace(content),
		IdempotencyKey: strings.TrimSpace(idempotencyKey),
		Status:         domaininteraction.CommentStatusPublished,
	}
	if err := comment.Validate(); err != nil {
		return nil, false, err
	}

	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeInteractionCommented, map[string]any{
		"user_id":      userID,
		"video_id":     videoID,
		"parent_id":    parentID,
		"author_id":    v.AuthorID,
		"commented_at": time.Now(),
	})
	if err != nil {
		return nil, false, fmt.Errorf("create outbox event error: %w", err)
	}

	resComment, created, err := s.interactionRepo.CreateComment(ctx, comment, outboxEvent)
	if err != nil {
		return nil, false, err
	}

	detail := s.assembleCommentDetail(ctx, resComment, userID)
	return detail, created, nil
}

// DeleteComment removes a comment by its author.
func (s *Service) DeleteComment(ctx context.Context, userID, commentID int64) error {
	c, err := s.interactionRepo.FindCommentByID(ctx, commentID)
	if err != nil {
		return err
	}

	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeInteractionCommentDelete, map[string]any{
		"comment_id": commentID,
		"video_id":   c.VideoID,
		"user_id":    userID,
		"deleted_at": time.Now(),
	})
	if err != nil {
		return fmt.Errorf("create outbox event error: %w", err)
	}

	return s.interactionRepo.DeleteComment(ctx, commentID, userID, outboxEvent)
}

// ListComments retrieves comments sorted by time or hot rank.
func (s *Service) ListComments(ctx context.Context, viewerID, videoID int64, sort string, cursorTime int64, cursorLikeCount, cursorID int64, limit int) ([]*domaininteraction.CommentDetail, int64, int64, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	var comments []*domaininteraction.Comment
	var nextTimeCursor, nextLikeCount, nextID int64
	var hasMore bool
	var err error

	if sort == "hot" {
		comments, nextLikeCount, nextID, hasMore, err = s.interactionRepo.ListCommentsByHot(ctx, videoID, cursorLikeCount, cursorID, limit)
	} else {
		comments, nextTimeCursor, hasMore, err = s.interactionRepo.ListCommentsByTime(ctx, videoID, cursorTime, limit)
	}
	if err != nil {
		return nil, 0, 0, false, err
	}

	// Batch get like status if viewer is logged in
	var commentIDs []int64
	for _, c := range comments {
		commentIDs = append(commentIDs, c.ID)
	}

	likeMap := make(map[int64]bool)
	if viewerID > 0 && len(commentIDs) > 0 {
		likeMap, _ = s.interactionRepo.BatchIsCommentLiked(ctx, viewerID, commentIDs)
	}

	var details []*domaininteraction.CommentDetail
	for _, c := range comments {
		var author *domaininteraction.CommentAuthor
		if s.accountRepo != nil {
			if u, err := s.accountRepo.FindByID(ctx, c.UserID); err == nil && u != nil {
				author = &domaininteraction.CommentAuthor{
					ID:        u.ID,
					Username:  u.Username,
					Nickname:  u.Nickname,
					AvatarURL: u.AvatarURL,
				}
			}
		}

		details = append(details, &domaininteraction.CommentDetail{
			Comment: c,
			Author:  author,
			IsLiked: likeMap[c.ID],
		})
	}

	if sort == "hot" {
		return details, nextLikeCount, nextID, hasMore, nil
	}
	return details, nextTimeCursor, 0, hasMore, nil
}

// LikeComment handles liking a comment.
func (s *Service) LikeComment(ctx context.Context, userID, commentID int64) error {
	c, err := s.interactionRepo.FindCommentByID(ctx, commentID)
	if err != nil {
		return err
	}
	if !c.IsPublished() {
		return domaininteraction.ErrCommentNotFound
	}

	_, err = s.interactionRepo.LikeComment(ctx, userID, commentID)
	return err
}

// UnlikeComment handles unliking a comment.
func (s *Service) UnlikeComment(ctx context.Context, userID, commentID int64) error {
	_, err := s.interactionRepo.UnlikeComment(ctx, userID, commentID)
	return err
}

func (s *Service) assembleCommentDetail(ctx context.Context, c *domaininteraction.Comment, viewerID int64) *domaininteraction.CommentDetail {
	var author *domaininteraction.CommentAuthor
	if s.accountRepo != nil {
		if u, err := s.accountRepo.FindByID(ctx, c.UserID); err == nil && u != nil {
			author = &domaininteraction.CommentAuthor{
				ID:        u.ID,
				Username:  u.Username,
				Nickname:  u.Nickname,
				AvatarURL: u.AvatarURL,
			}
		}
	}

	isLiked := false
	if viewerID > 0 {
		likedMap, _ := s.interactionRepo.BatchIsCommentLiked(ctx, viewerID, []int64{c.ID})
		isLiked = likedMap[c.ID]
	}

	return &domaininteraction.CommentDetail{
		Comment: c,
		Author:  author,
		IsLiked: isLiked,
	}
}
