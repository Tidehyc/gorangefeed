package feed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
)

// EngagementPayload models generic video engagement payloads from outbox events.
type EngagementPayload struct {
	VideoID    int64     `json:"video_id"`
	UserID     int64     `json:"user_id,omitempty"`
	AuthorID   int64     `json:"author_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	Timestamp  time.Time `json:"timestamp,omitempty"`
}

// HotScoreConsumer coordinates real-time engagement score accumulation for sliding hot feed buckets.
type HotScoreConsumer struct {
	hotStore     domainfeed.HotTimelineStore
	deduplicator rabbitmq.Deduplicator
}

// NewHotScoreConsumer creates a new HotScoreConsumer instance.
func NewHotScoreConsumer(
	hotStore domainfeed.HotTimelineStore,
	deduplicator rabbitmq.Deduplicator,
) (*HotScoreConsumer, error) {
	if hotStore == nil {
		return nil, errors.New("hot timeline store cannot be nil")
	}
	if deduplicator == nil {
		return nil, errors.New("deduplicator cannot be nil")
	}

	return &HotScoreConsumer{
		hotStore:     hotStore,
		deduplicator: deduplicator,
	}, nil
}

// HandleEngagementEvent processes an interaction or playback event, applying the corresponding
// engagement score delta to the target video's 1-minute bucket.
//
// The score is applied before the event is recorded as consumed, and the apply itself is guarded
// per event ID (ApplyEngagementOnce). Ordering matters: marking first meant a failure after the
// mark turned into a silent drop on redelivery, because the redelivery saw the marker and skipped
// the work. Applying first means a redelivery is at worst a no-op, never a lost event.
func (c *HotScoreConsumer) HandleEngagementEvent(ctx context.Context, eventID string, routingKey string, payload []byte) error {
	if eventID == "" {
		return errors.New("event ID cannot be empty")
	}

	// 1. Only recognized engagement events carry a score delta.
	delta, ok := engagementDelta(routingKey)
	if !ok {
		// Not one of ours: record it so the broker stops redelivering, and stop.
		return c.markConsumed(ctx, eventID)
	}

	// 2. Parse payload
	var p EngagementPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return rabbitmq.Permanent(fmt.Errorf("failed to unmarshal engagement payload: %w", err))
	}
	if p.VideoID <= 0 {
		return rabbitmq.Permanent(fmt.Errorf("invalid engagement payload: video_id=%d", p.VideoID))
	}

	// 3. Determine event timestamp
	occurredAt := p.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = p.CreatedAt
	}
	if occurredAt.IsZero() {
		occurredAt = p.Timestamp
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	// 4. Apply the delta at most once per event, atomically (guard + ZINCRBY in one script).
	if _, err := c.hotStore.ApplyEngagementOnce(ctx, eventID, p.VideoID, delta, occurredAt); err != nil {
		return fmt.Errorf("failed to apply hot score for video %d: %w", p.VideoID, err)
	}

	// 5. Record consumption only now that the score is in place.
	return c.markConsumed(ctx, eventID)
}

// markConsumed records the event in consumed_events. A failure here is retryable: the redelivery
// re-runs an idempotent apply and tries to mark again.
func (c *HotScoreConsumer) markConsumed(ctx context.Context, eventID string) error {
	if _, err := c.deduplicator.MarkConsumed(ctx, eventID); err != nil {
		return fmt.Errorf("failed to mark event %s as consumed: %w", eventID, err)
	}
	return nil
}

// engagementDelta maps a routing key to its score delta.
func engagementDelta(routingKey string) (float64, bool) {
	switch routingKey {
	case rabbitmq.RoutingKeyInteractionLiked:
		return domainfeed.ScoreLikeDelta, true
	case rabbitmq.RoutingKeyInteractionUnliked:
		return domainfeed.ScoreUnlikeDelta, true
	case rabbitmq.RoutingKeyInteractionCommented:
		return domainfeed.ScoreCommentDelta, true
	case rabbitmq.RoutingKeyInteractionCommentDelete:
		return domainfeed.ScoreCommentDeleteDelta, true
	case rabbitmq.RoutingKeyVideoViewed:
		return domainfeed.ScorePlayDelta, true
	case rabbitmq.RoutingKeyVideoCompleted:
		return domainfeed.ScoreCompleteDelta, true
	case rabbitmq.RoutingKeyInteractionFavorited:
		return domainfeed.ScoreFavoriteDelta, true
	case rabbitmq.RoutingKeyInteractionUnfavorited:
		return domainfeed.ScoreUnfavoriteDelta, true
	default:
		return 0, false
	}
}
