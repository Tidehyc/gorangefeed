package relation

import "time"

// Follow represents a user following relationship.
type Follow struct {
	ID         int64     `json:"id"`
	FollowerID int64     `json:"follower_id"`
	VloggerID  int64     `json:"vlogger_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// RelationStats represents following and follower counts for a user.
type RelationStats struct {
	UserID         int64     `json:"user_id"`
	FollowingCount int64     `json:"following_count"`
	FollowerCount  int64     `json:"follower_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Friendship represents the bidirectional relationship state between two users.
type Friendship struct {
	IsFollowing bool `json:"is_following"` // Current user is following target user
	IsFollower  bool `json:"is_follower"`  // Target user is following current user
	IsFriend    bool `json:"is_friend"`    // Mutual following (IsFollowing && IsFollower)
}

// UserProfile represents a public summary of a user in following/follower lists.
type UserProfile struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
	Bio       string `json:"bio"`
	IsFriend  bool   `json:"is_friend"`
}
