package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domainrelation "github.com/Tidehyc/gorangefeed/internal/domain/relation"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
)

const (
	// BigVThreshold defines the minimum follower count for an author to transition from Push to Pull model.
	BigVThreshold = 5000

	// MaxInboxCapacity defines the maximum number of items retained in a user's following inbox ZSet.
	MaxInboxCapacity = 800
)

// VideoPublishedPayload defines the event payload for video publication.
type VideoPublishedPayload struct {
	VideoID     int64     `json:"video_id"`
	AuthorID    int64     `json:"author_id"`
	PublishedAt time.Time `json:"published_at,omitempty"`
}

// FanoutService coordinates write fanout for following feeds when videos are published.
type FanoutService struct {
	relationRepo  domainrelation.Repository
	followingRepo domainfeed.FollowingFeedRepository
	deduplicator  rabbitmq.Deduplicator
}

// NewFanoutService creates a new FanoutService instance.
func NewFanoutService(
	relationRepo domainrelation.Repository,
	followingRepo domainfeed.FollowingFeedRepository,
	deduplicator rabbitmq.Deduplicator,
) (*FanoutService, error) {
	if relationRepo == nil {
		return nil, errors.New("relation repository cannot be nil")
	}
	if followingRepo == nil {
		return nil, errors.New("following feed repository cannot be nil")
	}
	if deduplicator == nil {
		return nil, errors.New("deduplicator cannot be nil")
	}

	return &FanoutService{
		relationRepo:  relationRepo,
		followingRepo: followingRepo,
		deduplicator:  deduplicator,
	}, nil
}

// HandleVideoPublished processes the video.published event, writing to followers' inboxes (push)
// or the author's outbox (pull) depending on whether the author has >= 5000 followers.
func (s *FanoutService) HandleVideoPublished(ctx context.Context, eventID string, payload []byte) error {
	if eventID == "" {
		return errors.New("event ID cannot be empty")
	}

	// 1. Transactional deduplication via consumed_events
	isNew, err := s.deduplicator.MarkConsumed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("deduplication check failed: %w", err)
	}
	if !isNew {
		// Event was already consumed previously -> idempotent success
		return nil
	}

	// 2. Parse event payload
	var p VideoPublishedPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal video published payload: %w", err)
	}
	if p.VideoID <= 0 || p.AuthorID <= 0 {
		return fmt.Errorf("invalid video published payload: video_id=%d, author_id=%d", p.VideoID, p.AuthorID)
	}

	// 3. Query author follower count to decide push vs pull
	stats, err := s.relationRepo.GetStats(ctx, p.AuthorID)
	if err != nil {
		return fmt.Errorf("failed to get relation stats for author %d: %w", p.AuthorID, err)
	}

	var followerCount int64
	if stats != nil {
		followerCount = stats.FollowerCount
	}

	// 4. Branch: Big V (Pull) vs Normal Author (Push)
	if followerCount >= BigVThreshold {
		// Big V Author -> write to author's dedicated outbox ZSet
		if err := s.followingRepo.PushToAuthorOutbox(ctx, p.AuthorID, p.VideoID, MaxInboxCapacity); err != nil {
			return fmt.Errorf("failed to push to author outbox for big V %d: %w", p.AuthorID, err)
		}
		return nil
	}

	// Normal Author -> write fanout to all followers' inbox ZSets
	var cursor int64
	for {
		followerIDs, nextCursor, hasMore, err := s.relationRepo.ListFollowerIDs(ctx, p.AuthorID, cursor, 500)
		if err != nil {
			return fmt.Errorf("failed to list followers for author %d: %w", p.AuthorID, err)
		}

		if len(followerIDs) > 0 {
			if err := s.followingRepo.PushToInboxes(ctx, followerIDs, p.VideoID, MaxInboxCapacity); err != nil {
				return fmt.Errorf("failed to push video %d to follower inboxes: %w", p.VideoID, err)
			}
		}

		if !hasMore || nextCursor <= 0 {
			break
		}
		cursor = nextCursor
	}

	return nil
}
