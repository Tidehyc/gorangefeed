package relation

import (
	"context"

	"github.com/Tidehyc/gorangefeed/internal/domain/outbox"
)

// Repository defines data persistence contracts for relation domain.
type Repository interface {
	// Follow atomically creates a follow relationship and updates following/follower counts using RowsAffected == 1.
	// Optionally writes an outboxEvent within the same transaction.
	Follow(ctx context.Context, followerID, vloggerID int64, outboxEvent *outbox.Event) (bool, error)

	// Unfollow atomically deletes a follow relationship and decrements counts using RowsAffected == 1.
	// GREATEST prevents counts from falling below 0. Optionally writes an outboxEvent.
	Unfollow(ctx context.Context, followerID, vloggerID int64, outboxEvent *outbox.Event) (bool, error)

	// GetRelation evaluates the bidirectional friendship state between current user and target user.
	GetRelation(ctx context.Context, userID, targetID int64) (*Friendship, error)

	// BatchIsFollowed returns a map of authorID -> isFollowed for the given follower.
	BatchIsFollowed(ctx context.Context, followerID int64, authorIDs []int64) (map[int64]bool, error)

	// ListFollowingIDs retrieves vlogger IDs that the user follows, ordered by follows.id DESC.
	// Returns: vloggerIDs, nextCursor, hasMore, error.
	ListFollowingIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error)

	// ListFollowerIDs retrieves follower IDs that follow the vlogger, ordered by follows.id DESC.
	// Returns: followerIDs, nextCursor, hasMore, error.
	ListFollowerIDs(ctx context.Context, userID int64, cursor int64, limit int) ([]int64, int64, bool, error)

	// GetStats retrieves relation statistics for a user.
	GetStats(ctx context.Context, userID int64) (*RelationStats, error)
}
