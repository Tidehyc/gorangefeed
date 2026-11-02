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
func (c *HotScoreConsumer) HandleEngagementEvent(ctx context.Context, eventID string, routingKey string, payload []byte) error {
	if eventID == "" {
		return errors.New("event ID cannot be empty")
	}

	// 1. Transactional deduplication via consumed_events
	isNew, err := c.deduplicator.MarkConsumed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("deduplication check failed: %w", err)
	}
	if !isNew {
		// Event was already consumed previously -> idempotent skip
		return nil
	}

	// 2. Parse payload
	var p EngagementPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal engagement payload: %w", err)
	}
	if p.VideoID <= 0 {
		// Non-video event or malformed payload, ignore safely
		return nil
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

	// 4. Determine score delta based on routing key
	var delta float64
	switch routingKey {
	case rabbitmq.RoutingKeyInteractionLiked:
		delta = domainfeed.ScoreLikeDelta
	case rabbitmq.RoutingKeyInteractionUnliked:
		delta = domainfeed.ScoreUnlikeDelta
	case rabbitmq.RoutingKeyInteractionCommented:
		delta = domainfeed.ScoreCommentDelta
	case rabbitmq.RoutingKeyInteractionCommentDelete:
		delta = domainfeed.ScoreCommentDeleteDelta
	case rabbitmq.RoutingKeyVideoViewed:
		delta = domainfeed.ScorePlayDelta
	case rabbitmq.RoutingKeyVideoCompleted:
		delta = domainfeed.ScoreCompleteDelta
	default:
		// Unrecognized engagement event, skip
		return nil
	}

	// 5. Apply score change to the target minute bucket in Redis
	if err := c.hotStore.IncrScore(ctx, p.VideoID, delta, occurredAt); err != nil {
		return fmt.Errorf("failed to increment hot score for video %d: %w", p.VideoID, err)
	}

	return nil
}
