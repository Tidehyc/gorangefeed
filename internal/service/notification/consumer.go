package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	"github.com/Tidehyc/gorangefeed/internal/infra/rabbitmq"
)

// LikeEventPayload models the payload from interaction.liked outbox events.
type LikeEventPayload struct {
	UserID   int64     `json:"user_id"`
	VideoID  int64     `json:"video_id"`
	AuthorID int64     `json:"author_id"`
	LikedAt  time.Time `json:"liked_at"`
}

// CommentEventPayload models the payload from interaction.commented outbox events.
type CommentEventPayload struct {
	UserID      int64     `json:"user_id"`
	VideoID     int64     `json:"video_id"`
	ParentID    int64     `json:"parent_id"`
	AuthorID    int64     `json:"author_id"`
	Content     string    `json:"content"`
	CommentedAt time.Time `json:"commented_at"`
}

// FollowEventPayload models the payload from relation.followed outbox events.
type FollowEventPayload struct {
	FollowerID int64     `json:"follower_id"`
	VloggerID  int64     `json:"vlogger_id"`
	FollowedAt time.Time `json:"followed_at"`
}

// Consumer processes RabbitMQ interaction and relation events, generates system
// notifications with double-layer deduplication, and broadcasts them across nodes.
type Consumer struct {
	notifRepo     domainnotification.Repository
	broadcaster   domainnotification.Broadcaster
	deduplicator  rabbitmq.Deduplicator
	accountFinder AccountFinder
	mentionParser *MentionParser
}

// NewConsumer creates a new notification Consumer.
func NewConsumer(
	notifRepo domainnotification.Repository,
	broadcaster domainnotification.Broadcaster,
	deduplicator rabbitmq.Deduplicator,
	accountFinder AccountFinder,
) (*Consumer, error) {
	if notifRepo == nil {
		return nil, errors.New("notification repository cannot be nil")
	}
	if deduplicator == nil {
		return nil, errors.New("deduplicator cannot be nil")
	}

	return &Consumer{
		notifRepo:     notifRepo,
		broadcaster:   broadcaster,
		deduplicator:  deduplicator,
		accountFinder: accountFinder,
		mentionParser: NewMentionParser(accountFinder),
	}, nil
}

// HandleEvent dispatches an incoming RabbitMQ event by routing key with deduplication.
//
// The notification rows are written before the event is recorded as consumed. Marking first meant a
// persistence failure left the marker behind, so the redelivery skipped the work and the recipient
// never saw the notification. The insert itself is idempotent (UNIQUE(recipient_id, event_id) with
// ON CONFLICT DO NOTHING), so repeating it on redelivery is safe.
func (c *Consumer) HandleEvent(ctx context.Context, eventID string, routingKey string, payload []byte) error {
	if eventID == "" {
		return errors.New("event ID cannot be empty")
	}

	// 1. Cheap skip for replays that already completed.
	consumed, err := c.deduplicator.IsConsumed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("deduplication check failed: %w", err)
	}
	if consumed {
		// Event was already consumed previously -> idempotent skip
		return nil
	}

	// 2. Run the business handler
	switch routingKey {
	case rabbitmq.RoutingKeyInteractionLiked:
		if err := c.handleLike(ctx, eventID, payload); err != nil {
			return err
		}
	case rabbitmq.RoutingKeyInteractionCommented:
		if err := c.handleComment(ctx, eventID, payload); err != nil {
			return err
		}
	case rabbitmq.RoutingKeyRelationFollowed:
		if err := c.handleFollow(ctx, eventID, payload); err != nil {
			return err
		}
	default:
		// Unrecognized routing key: nothing to persist, but still record it so the broker
		// stops redelivering.
	}

	// 3. Record consumption only now that the notifications are persisted
	if _, err := c.deduplicator.MarkConsumed(ctx, eventID); err != nil {
		return fmt.Errorf("failed to mark event %s as consumed: %w", eventID, err)
	}
	return nil
}

func (c *Consumer) handleLike(ctx context.Context, eventID string, payload []byte) error {
	var p LikeEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return rabbitmq.Permanent(fmt.Errorf("failed to unmarshal like payload: %w", err))
	}

	// Self-action defense: liking own video never notifies oneself
	if p.UserID <= 0 || p.AuthorID <= 0 || p.UserID == p.AuthorID {
		return nil
	}

	occurredAt := p.LikedAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	notif := &domainnotification.Notification{
		RecipientID: p.AuthorID,
		ActorID:     p.UserID,
		Type:        domainnotification.TypeLike,
		TargetType:  domainnotification.TargetVideo,
		TargetID:    p.VideoID,
		Title:       "点赞通知",
		Content:     "赞了你的视频",
		EventID:     eventID,
		CreatedAt:   occurredAt,
	}

	if err := c.notifRepo.Create(ctx, notif); err != nil {
		return fmt.Errorf("failed to persist like notification: %w", err)
	}

	c.enrichAndBroadcast(ctx, notif)
	return nil
}

func (c *Consumer) handleComment(ctx context.Context, eventID string, payload []byte) error {
	var p CommentEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return rabbitmq.Permanent(fmt.Errorf("failed to unmarshal comment payload: %w", err))
	}

	occurredAt := p.CommentedAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	snippet := truncateRunes(p.Content, 80)

	// 1. Notify video author if not a self-comment
	if p.UserID > 0 && p.AuthorID > 0 && p.UserID != p.AuthorID {
		content := "评论了你的视频"
		if snippet != "" {
			content = "评论了你的视频: " + snippet
		}
		authorNotif := &domainnotification.Notification{
			RecipientID: p.AuthorID,
			ActorID:     p.UserID,
			Type:        domainnotification.TypeComment,
			TargetType:  domainnotification.TargetVideo,
			TargetID:    p.VideoID,
			Title:       "评论通知",
			Content:     content,
			EventID:     eventID,
			CreatedAt:   occurredAt,
		}

		if err := c.notifRepo.Create(ctx, authorNotif); err != nil {
			return fmt.Errorf("failed to persist comment notification for author: %w", err)
		}
		c.enrichAndBroadcast(ctx, authorNotif)
	}

	// 2. Parse and dispatch @mention notifications
	if p.Content != "" && c.mentionParser != nil {
		mentionUIDs, err := c.mentionParser.ParseMentions(ctx, p.Content, p.UserID, p.AuthorID)
		if err != nil {
			slog.Warn("failed to parse mentions in comment", "event_id", eventID, "error", err)
		} else {
			for _, mUID := range mentionUIDs {
				mContent := "在评论中提及了你"
				if snippet != "" {
					mContent = "在评论中提及了你: " + snippet
				}
				mNotif := &domainnotification.Notification{
					RecipientID: mUID,
					ActorID:     p.UserID,
					Type:        domainnotification.TypeMention,
					TargetType:  domainnotification.TargetVideo,
					TargetID:    p.VideoID,
					Title:       "提及通知",
					Content:     mContent,
					EventID:     fmt.Sprintf("%s:mention:%d", eventID, mUID),
					CreatedAt:   occurredAt,
				}
				if err := c.notifRepo.Create(ctx, mNotif); err != nil {
					slog.Warn("failed to persist mention notification", "event_id", eventID, "recipient_id", mUID, "error", err)
					continue
				}
				c.enrichAndBroadcast(ctx, mNotif)
			}
		}
	}

	return nil
}

func (c *Consumer) handleFollow(ctx context.Context, eventID string, payload []byte) error {
	var p FollowEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return rabbitmq.Permanent(fmt.Errorf("failed to unmarshal follow payload: %w", err))
	}

	// Self-action defense: following oneself should never happen, but filter defensively
	if p.FollowerID <= 0 || p.VloggerID <= 0 || p.FollowerID == p.VloggerID {
		return nil
	}

	occurredAt := p.FollowedAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	notif := &domainnotification.Notification{
		RecipientID: p.VloggerID,
		ActorID:     p.FollowerID,
		Type:        domainnotification.TypeFollow,
		TargetType:  domainnotification.TargetUser,
		TargetID:    p.FollowerID,
		Title:       "关注通知",
		Content:     "关注了你",
		EventID:     eventID,
		CreatedAt:   occurredAt,
	}

	if err := c.notifRepo.Create(ctx, notif); err != nil {
		return fmt.Errorf("failed to persist follow notification: %w", err)
	}

	c.enrichAndBroadcast(ctx, notif)
	return nil
}

func (c *Consumer) enrichAndBroadcast(ctx context.Context, notif *domainnotification.Notification) {
	if c.broadcaster == nil {
		return
	}

	// Hydrate ActorInfo if missing
	if notif.Actor == nil && c.accountFinder != nil && notif.ActorID > 0 {
		if u, err := c.accountFinder.FindByID(ctx, notif.ActorID); err == nil && u != nil {
			notif.Actor = &domainnotification.ActorInfo{
				ID:        u.ID,
				Username:  u.Username,
				Nickname:  u.Nickname,
				AvatarURL: u.AvatarURL,
			}
		}
	}

	if err := c.broadcaster.Publish(ctx, notif); err != nil {
		slog.Warn("failed to broadcast notification", "recipient_id", notif.RecipientID, "id", notif.ID, "error", err)
	}
}

func truncateRunes(s string, maxRunes int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxRunes {
		return string(r)
	}
	return string(r[:maxRunes]) + "..."
}
