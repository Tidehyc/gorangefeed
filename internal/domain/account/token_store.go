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

// TokenStore defines the storage contract for JWT sessions, rotations, and blacklists.
type TokenStore interface {
	CreateSession(ctx context.Context, sessionID, tokenID string, ttl time.Duration) error
	RotateSession(ctx context.Context, sessionID, oldTokenID, newTokenID string, ttl time.Duration) (RotationResult, error)
	RevokeSession(ctx context.Context, sessionID string) error
	BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error
	IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error)
}
