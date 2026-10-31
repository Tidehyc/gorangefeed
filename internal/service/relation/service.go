package relation

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
)

// Service coordinates user relationship business use cases.
type Service struct {
	relationRepo domainrelation.Repository
	accountRepo  domainaccount.Repository
}

// NewService creates a new relation Service.
func NewService(
	relationRepo domainrelation.Repository,
	accountRepo domainaccount.Repository,
) *Service {
	return &Service{
		relationRepo: relationRepo,
		accountRepo:  accountRepo,
	}
}

// Follow establishes a follow relationship between follower and vlogger.
func (s *Service) Follow(ctx context.Context, followerID, vloggerID int64) error {
	if followerID <= 0 || vloggerID <= 0 {
		return domainrelation.ErrInvalidFollowerID
	}
	if followerID == vloggerID {
		return domainrelation.ErrSelfFollow
	}

	// Verify vlogger exists
	if s.accountRepo != nil {
		target, err := s.accountRepo.FindByID(ctx, vloggerID)
		if err != nil {
			if errors.Is(err, domainaccount.ErrUserNotFound) {
				return domainrelation.ErrTargetUserNotFound
			}
			return fmt.Errorf("find target user error: %w", err)
		}
		if target == nil {
			return domainrelation.ErrTargetUserNotFound
		}
	}

	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeRelationFollowed, map[string]any{
		"follower_id": followerID,
		"vlogger_id":  vloggerID,
		"followed_at": time.Now(),
	})
	if err != nil {
		return fmt.Errorf("create outbox event error: %w", err)
	}

	_, err = s.relationRepo.Follow(ctx, followerID, vloggerID, outboxEvent)
	return err
}

// Unfollow removes a follow relationship.
func (s *Service) Unfollow(ctx context.Context, followerID, vloggerID int64) error {
	if followerID <= 0 || vloggerID <= 0 {
		return domainrelation.ErrInvalidFollowerID
	}
	if followerID == vloggerID {
		return domainrelation.ErrSelfUnfollow
	}

	outboxEvent, err := domainoutbox.NewEvent(domainoutbox.EventTypeRelationUnfollowed, map[string]any{
		"follower_id":   followerID,
		"vlogger_id":    vloggerID,
		"unfollowed_at": time.Now(),
	})
	if err != nil {
		return fmt.Errorf("create outbox event error: %w", err)
	}

	_, err = s.relationRepo.Unfollow(ctx, followerID, vloggerID, outboxEvent)
	return err
}

// GetRelation checks relationship and mutual friendship between current user and target user.
func (s *Service) GetRelation(ctx context.Context, userID, targetID int64) (*domainrelation.Friendship, error) {
	return s.relationRepo.GetRelation(ctx, userID, targetID)
}

// ListFollowings retrieves a paginated list of users followed by userID.
func (s *Service) ListFollowings(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error) {
	if userID <= 0 {
		return nil, 0, false, domainrelation.ErrInvalidFollowerID
	}
	if limit <= 0 {
		limit = 10
	}

	vloggerIDs, nextCursor, hasMore, err := s.relationRepo.ListFollowingIDs(ctx, userID, cursor, limit)
	if err != nil {
		return nil, 0, false, err
	}

	profiles := s.assembleUserProfiles(ctx, viewerID, vloggerIDs)
	return profiles, nextCursor, hasMore, nil
}

// ListFollowers retrieves a paginated list of users following userID.
func (s *Service) ListFollowers(ctx context.Context, userID, viewerID int64, cursor int64, limit int) ([]*domainrelation.UserProfile, int64, bool, error) {
	if userID <= 0 {
		return nil, 0, false, domainrelation.ErrInvalidVloggerID
	}
	if limit <= 0 {
		limit = 10
	}

	followerIDs, nextCursor, hasMore, err := s.relationRepo.ListFollowerIDs(ctx, userID, cursor, limit)
	if err != nil {
		return nil, 0, false, err
	}

	profiles := s.assembleUserProfiles(ctx, viewerID, followerIDs)
	return profiles, nextCursor, hasMore, nil
}

// GetStats returns following and follower counts.
func (s *Service) GetStats(ctx context.Context, userID int64) (*domainrelation.RelationStats, error) {
	return s.relationRepo.GetStats(ctx, userID)
}

func (s *Service) assembleUserProfiles(ctx context.Context, viewerID int64, userIDs []int64) []*domainrelation.UserProfile {
	var profiles []*domainrelation.UserProfile
	if len(userIDs) == 0 {
		return profiles
	}

	for _, uid := range userIDs {
		p := &domainrelation.UserProfile{ID: uid}
		if s.accountRepo != nil {
			if u, err := s.accountRepo.FindByID(ctx, uid); err == nil && u != nil {
				p.Username = u.Username
				p.Nickname = u.Nickname
				p.AvatarURL = u.AvatarURL
				p.Bio = u.Bio
			}
		}

		if viewerID > 0 {
			rel, _ := s.relationRepo.GetRelation(ctx, viewerID, uid)
			if rel != nil {
				p.IsFriend = rel.IsFriend
			}
		}

		profiles = append(profiles, p)
	}
	return profiles
}
