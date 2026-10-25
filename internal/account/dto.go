package account

import (
	"time"
)

// RegisterRequest represents the JSON payload for registration.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest represents the JSON payload for login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RefreshRequest represents the JSON payload for token refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutRequest represents the optional JSON payload for logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// UpdateProfileRequest represents the JSON payload for updating nickname and bio.
type UpdateProfileRequest struct {
	Nickname string `json:"nickname"`
	Bio      string `json:"bio"`
}

// UserStats contains aggregated social and content metrics.
// Pre-allocated in M1 with default 0s, to be populated in M2/M4.
type UserStats struct {
	FollowingCount int64 `json:"following_count"`
	FollowerCount  int64 `json:"follower_count"`
	WorkCount      int64 `json:"work_count"`
	TotalLikes     int64 `json:"total_likes"`
}

// UserProfileResponse represents the private, full profile for the authenticated user (/users/me).
type UserProfileResponse struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	Nickname  string     `json:"nickname"`
	AvatarURL string     `json:"avatar_url"`
	Bio       string     `json:"bio"`
	Status    UserStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	Stats     UserStats  `json:"stats"`
}

// PublicUserProfileResponse represents public profile visible to any user (/users/:userId).
// Sensitive attributes like username and account status are omitted.
type PublicUserProfileResponse struct {
	ID        int64     `json:"id"`
	Nickname  string    `json:"nickname"`
	AvatarURL string    `json:"avatar_url"`
	Bio       string    `json:"bio"`
	Stats     UserStats `json:"stats"`
}

// LoginResponse represents the login payload containing tokens and user profile.
type LoginResponse struct {
	Tokens TokenPair           `json:"tokens"`
	User   UserProfileResponse `json:"user"`
}

// ToUserProfileResponse converts an internal User entity to UserProfileResponse.
func ToUserProfileResponse(u *User) UserProfileResponse {
	return UserProfileResponse{
		ID:        u.ID,
		Username:  u.Username,
		Nickname:  u.Nickname,
		AvatarURL: u.AvatarURL,
		Bio:       u.Bio,
		Status:    u.Status,
		CreatedAt: u.CreatedAt,
		Stats:     UserStats{},
	}
}

// ToPublicUserProfileResponse converts an internal User entity to PublicUserProfileResponse.
func ToPublicUserProfileResponse(u *User) PublicUserProfileResponse {
	return PublicUserProfileResponse{
		ID:        u.ID,
		Nickname:  u.Nickname,
		AvatarURL: u.AvatarURL,
		Bio:       u.Bio,
		Stats:     UserStats{},
	}
}
