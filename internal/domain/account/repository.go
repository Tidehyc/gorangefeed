package account

import "context"

// Repository defines the account data persistence contract.
type Repository interface {
	Create(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id int64) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	UpdateProfile(ctx context.Context, id int64, nickname, bio string) error
	UpdateAvatar(ctx context.Context, id int64, avatarURL string) error
}
