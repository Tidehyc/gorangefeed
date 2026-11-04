package moderation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
)

// Service defines the moderation operations interface.
type Service interface {
	SubmitVideoForReview(ctx context.Context, videoID int64, policy domainmoderation.ReviewPolicy, title, desc string) (*domainmoderation.ReviewTask, error)
	ReviewVideo(ctx context.Context, videoID int64, reviewerID int64, action string, note string) error
	TakedownVideo(ctx context.Context, videoID int64, reviewerID int64, reason string) error
	BanUser(ctx context.Context, userID int64, operatorID int64, reason string) error
	ListReviewTasks(ctx context.Context, status domainmoderation.ReviewStatus, cursor int64, limit int) ([]*domainmoderation.ReviewTask, int64, bool, error)
	ListAuditLogs(ctx context.Context, targetType string, targetID int64, limit int) ([]*domainmoderation.ModerationLog, error)
}

type service struct {
	repo         domainmoderation.ReviewRepository
	matcher      domainmoderation.SensitiveMatcher
	videoRepo    domainvideo.Repository
	timelineRepo domainfeed.TimelineRepository
	accountRepo  domainaccount.Repository
	tokenStore   domainaccount.TokenStore
	logger       *slog.Logger
}

// NewService constructs a content moderation Service.
func NewService(
	repo domainmoderation.ReviewRepository,
	matcher domainmoderation.SensitiveMatcher,
	videoRepo domainvideo.Repository,
	timelineRepo domainfeed.TimelineRepository,
	accountRepo domainaccount.Repository,
	tokenStore domainaccount.TokenStore,
	logger *slog.Logger,
) Service {
	if logger == nil {
		logger = slog.Default()
	}
	if matcher == nil {
		matcher = NewACMatcher()
	}
	return &service{
		repo:         repo,
		matcher:      matcher,
		videoRepo:    videoRepo,
		timelineRepo: timelineRepo,
		accountRepo:  accountRepo,
		tokenStore:   tokenStore,
		logger:       logger,
	}
}

func (s *service) SubmitVideoForReview(ctx context.Context, videoID int64, policy domainmoderation.ReviewPolicy, title, desc string) (*domainmoderation.ReviewTask, error) {
	task, err := domainmoderation.NewReviewTask(videoID, policy)
	if err != nil {
		return nil, err
	}

	// 1. Run AC automaton machine scan
	scanText := title + " " + desc
	matchRes := s.matcher.Match(scanText)

	// 2. Apply machine inspection result
	if err := task.ApplyMachineResult(matchRes.RiskScore, matchRes.MatchedWords); err != nil {
		return nil, fmt.Errorf("apply machine result: %w", err)
	}

	// 3. Persist review task
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, fmt.Errorf("create review task: %w", err)
	}

	// 4. Handle state consequences
	if task.IsApproved() {
		_ = s.repo.CreateLog(ctx, &domainmoderation.ModerationLog{
			TargetType: domainmoderation.TargetTypeVideo,
			TargetID:   videoID,
			Action:     domainmoderation.ActionMachineApprove,
			OperatorID: 0,
			Reason:     "machine scan clean",
			CreatedAt:  time.Now().UTC(),
		})
	} else if task.IsRejected() {
		// Auto reject: hide video & remove from timeline if present
		if s.videoRepo != nil {
			_ = s.videoRepo.UpdateStatus(ctx, videoID, 0, domainvideo.VideoStatusHidden)
		}
		if s.timelineRepo != nil {
			_ = s.timelineRepo.Remove(ctx, videoID)
		}
		_ = s.repo.CreateLog(ctx, &domainmoderation.ModerationLog{
			TargetType: domainmoderation.TargetTypeVideo,
			TargetID:   videoID,
			Action:     domainmoderation.ActionMachineReject,
			OperatorID: 0,
			Reason:     fmt.Sprintf("high risk words detected: %v", matchRes.MatchedWords),
			CreatedAt:  time.Now().UTC(),
		})
	} else {
		// Pending human review: if PreReview policy, ensure video is hidden from timeline
		if policy == domainmoderation.ReviewPolicyPreReview && s.videoRepo != nil {
			_ = s.videoRepo.UpdateStatus(ctx, videoID, 0, domainvideo.VideoStatusHidden)
		}
		_ = s.repo.CreateLog(ctx, &domainmoderation.ModerationLog{
			TargetType: domainmoderation.TargetTypeVideo,
			TargetID:   videoID,
			Action:     "machine_flagged",
			OperatorID: 0,
			Reason:     fmt.Sprintf("flagged words: %v", matchRes.MatchedWords),
			CreatedAt:  time.Now().UTC(),
		})
	}

	return task, nil
}

func (s *service) ReviewVideo(ctx context.Context, videoID int64, reviewerID int64, action string, note string) error {
	task, err := s.repo.GetTaskByVideoID(ctx, videoID)
	if err != nil {
		return err
	}

	switch action {
	case "approve":
		if err := task.ApproveByHuman(reviewerID, note); err != nil {
			return err
		}
		if err := s.repo.UpdateTask(ctx, task); err != nil {
			return err
		}
		if s.videoRepo != nil {
			_ = s.videoRepo.UpdateStatus(ctx, videoID, 0, domainvideo.VideoStatusPublished)
		}
		if s.timelineRepo != nil {
			_ = s.timelineRepo.Append(ctx, videoID)
		}
		_ = s.repo.CreateLog(ctx, &domainmoderation.ModerationLog{
			TargetType: domainmoderation.TargetTypeVideo,
			TargetID:   videoID,
			Action:     domainmoderation.ActionHumanApprove,
			OperatorID: reviewerID,
			Reason:     note,
			CreatedAt:  time.Now().UTC(),
		})
		return nil

	case "reject":
		if err := task.RejectByHuman(reviewerID, note); err != nil {
			return err
		}
		if err := s.repo.UpdateTask(ctx, task); err != nil {
			return err
		}
		if s.videoRepo != nil {
			_ = s.videoRepo.UpdateStatus(ctx, videoID, 0, domainvideo.VideoStatusHidden)
		}
		if s.timelineRepo != nil {
			_ = s.timelineRepo.Remove(ctx, videoID)
		}
		_ = s.repo.CreateLog(ctx, &domainmoderation.ModerationLog{
			TargetType: domainmoderation.TargetTypeVideo,
			TargetID:   videoID,
			Action:     domainmoderation.ActionHumanReject,
			OperatorID: reviewerID,
			Reason:     note,
			CreatedAt:  time.Now().UTC(),
		})
		return nil

	default:
		return domainmoderation.ErrInvalidAction
	}
}

func (s *service) TakedownVideo(ctx context.Context, videoID int64, reviewerID int64, reason string) error {
	task, err := s.repo.GetTaskByVideoID(ctx, videoID)
	if err != nil {
		// If task record doesn't exist, create an approved one first so it can transition to takedown
		task, _ = domainmoderation.NewReviewTask(videoID, domainmoderation.ReviewPolicyPostReview)
		_ = task.ApplyMachineResult(0, nil)
		_ = s.repo.CreateTask(ctx, task)
	}

	if err := task.Takedown(reviewerID, reason); err != nil {
		return err
	}
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}

	if s.videoRepo != nil {
		_ = s.videoRepo.UpdateStatus(ctx, videoID, 0, domainvideo.VideoStatusHidden)
	}
	if s.timelineRepo != nil {
		_ = s.timelineRepo.Remove(ctx, videoID)
	}

	_ = s.repo.CreateLog(ctx, &domainmoderation.ModerationLog{
		TargetType: domainmoderation.TargetTypeVideo,
		TargetID:   videoID,
		Action:     domainmoderation.ActionTakedown,
		OperatorID: reviewerID,
		Reason:     reason,
		CreatedAt:  time.Now().UTC(),
	})
	return nil
}

func (s *service) BanUser(ctx context.Context, userID int64, operatorID int64, reason string) error {
	if userID <= 0 {
		return domainmoderation.ErrInvalidTargetID
	}
	if operatorID <= 0 {
		return domainmoderation.ErrUnauthorizedReviewer
	}

	if s.accountRepo != nil {
		if err := s.accountRepo.UpdateStatus(ctx, userID, domainaccount.UserStatusSuspended); err != nil {
			return fmt.Errorf("update user status to suspended: %w", err)
		}
	}

	_ = s.repo.CreateLog(ctx, &domainmoderation.ModerationLog{
		TargetType: domainmoderation.TargetTypeUser,
		TargetID:   userID,
		Action:     domainmoderation.ActionBanUser,
		OperatorID: operatorID,
		Reason:     reason,
		CreatedAt:  time.Now().UTC(),
	})

	return nil
}

func (s *service) ListReviewTasks(ctx context.Context, status domainmoderation.ReviewStatus, cursor int64, limit int) ([]*domainmoderation.ReviewTask, int64, bool, error) {
	return s.repo.ListTasks(ctx, status, cursor, limit)
}

func (s *service) ListAuditLogs(ctx context.Context, targetType string, targetID int64, limit int) ([]*domainmoderation.ModerationLog, error) {
	return s.repo.ListLogs(ctx, targetType, targetID, limit)
}
