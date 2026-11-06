package account

import (
	"context"
	"time"
)

type RotationResult int

const (
	RotationOK              RotationResult = 1
	RotationSessionNotFound RotationResult = 0
	RotationReplayDetected  RotationResult = -1
)

// TokenStore defines the storage contract for JWT sessions, rotations, blacklists, and
// account-level bans. Bans live here rather than in the session namespace because access
// tokens carry no session identifier: revoking a session cannot revoke the access tokens
// already minted from it, so a user-scoped flag is the only way to cut them off immediately.
type TokenStore interface {
	CreateSession(ctx context.Context, sessionID, tokenID string, ttl time.Duration) error
	RotateSession(ctx context.Context, sessionID, oldTokenID, newTokenID string, ttl time.Duration) (RotationResult, error)
	RevokeSession(ctx context.Context, sessionID string) error
	BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error
	IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error)
	MarkUserBanned(ctx context.Context, userID int64, ttl time.Duration) error
	ClearUserBanned(ctx context.Context, userID int64) error
	IsUserBanned(ctx context.Context, userID int64) (bool, error)

	// InvalidateUserTokens invalidates every access token issued at or before invalidBefore for
	// the given user. Password changes and account deletion need a user-scoped cutoff rather than
	// a session or token blacklist: sessions cannot be enumerated per user, and a blacklist needs
	// the token ID of a device the caller is not holding. The marker self-expires once every token
	// it covers has expired on its own.
	InvalidateUserTokens(ctx context.Context, userID int64, invalidBefore time.Time, ttl time.Duration) error
	// IsUserTokenInvalidated reports whether a token issued at issuedAt predates the user's
	// current credential cutoff. A missing marker means "not invalidated".
	IsUserTokenInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error)
}
