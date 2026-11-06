package account

import "context"

// Repository defines the account data persistence contract.
type Repository interface {
	Create(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id int64) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	UpdateProfile(ctx context.Context, id int64, nickname, bio string) error
	UpdateAvatar(ctx context.Context, id int64, avatarURL string) error
	UpdateStatus(ctx context.Context, id int64, status UserStatus) error
	// UpdatePassword replaces the stored bcrypt hash. It is separate from UpdateProfile so a
	// profile edit can never accidentally clear the credential.
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	// Anonymize strips every personal identifier from the row: the username is rewritten to
	// deleted_<id> so the original handle stops appearing in comment payloads, and nickname,
	// avatar and bio are replaced with placeholders. One statement, because a partial
	// anonymisation is worse than none: it looks private while still leaking a field.
	Anonymize(ctx context.Context, id int64) error
}
