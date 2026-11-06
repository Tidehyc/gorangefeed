package account

import "context"

// VideoHider is the narrow view of the video domain that account deletion needs.
//
// It exists because account must not import video: the dependency would be circular-adjacent
// (video already depends on account for authorship) and would drag video's whole repository
// surface into this package. main wires the video repository in as the implementation.
type VideoHider interface {
	// HideAllByAuthor hides every published video authored by authorID and reports how many
	// rows changed. It must be idempotent: calling it twice hides nothing extra.
	HideAllByAuthor(ctx context.Context, authorID int64) (int64, error)
}

// ProfileStats is the counter block rendered on a profile page.
//
// The numbers live in three different tables (relation_stats, videos, video_stats) owned by
// three different domains, so aggregation happens in the persistence layer and only the
// finished numbers reach the account service.
type ProfileStats struct {
	FollowingCount int64
	FollowerCount  int64
	WorkCount      int64
	TotalLikes     int64
}

// ProfileStatsProvider aggregates the counters for a single profile.
//
// Like VideoHider this is deliberately narrow: account owns the profile response but not the
// tables behind these numbers. A missing provider yields zeroed stats instead of an error —
// a profile page that renders with "0 关注" is better than one that fails to render.
type ProfileStatsProvider interface {
	GetProfileStats(ctx context.Context, userID int64) (ProfileStats, error)
}
