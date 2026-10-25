package account

import (
	"context"
	"fmt"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/redis/go-redis/v9"
)

type redisTokenStore struct {
	client *redis.Client
}

// NewTokenStore constructs a Redis-backed TokenStore implementing domainaccount.TokenStore.
func NewTokenStore(client *redis.Client) domainaccount.TokenStore {
	return &redisTokenStore{client: client}
}

func sessionKey(sessionID string) string {
	return fmt.Sprintf("auth:session:%s", sessionID)
}

func blacklistKey(tokenID string) string {
	return fmt.Sprintf("auth:blacklist:%s", tokenID)
}

func (s *redisTokenStore) CreateSession(ctx context.Context, sessionID, tokenID string, ttl time.Duration) error {
	key := sessionKey(sessionID)
	return s.client.Set(ctx, key, tokenID, ttl).Err()
}

// rotateScript atomically checks the current token for a session.
// If it matches oldToken, updates to newToken and refreshes TTL.
// If session key is missing, returns 0.
// If current token != oldToken, detects replay attack, deletes the session and returns -1.
var rotateScript = redis.NewScript(`
local session_key = KEYS[1]
local old_token = ARGV[1]
local new_token = ARGV[2]
local ttl_seconds = tonumber(ARGV[3])

local current_token = redis.call('GET', session_key)
if not current_token then
    return 0
end

if current_token == old_token then
    redis.call('SET', session_key, new_token, 'EX', ttl_seconds)
    return 1
else
    redis.call('DEL', session_key)
    return -1
end
`)

func (s *redisTokenStore) RotateSession(ctx context.Context, sessionID, oldTokenID, newTokenID string, ttl time.Duration) (domainaccount.RotationResult, error) {
	key := sessionKey(sessionID)
	ttlSec := int(ttl.Seconds())
	if ttlSec <= 0 {
		ttlSec = 1
	}

	res, err := rotateScript.Run(ctx, s.client, []string{key}, oldTokenID, newTokenID, ttlSec).Int()
	if err != nil {
		return 0, fmt.Errorf("run rotate lua script error: %w", err)
	}

	return domainaccount.RotationResult(res), nil
}

func (s *redisTokenStore) RevokeSession(ctx context.Context, sessionID string) error {
	key := sessionKey(sessionID)
	return s.client.Del(ctx, key).Err()
}

func (s *redisTokenStore) BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	key := blacklistKey(tokenID)
	return s.client.Set(ctx, key, "1", ttl).Err()
}

func (s *redisTokenStore) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	key := blacklistKey(tokenID)
	val, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return val > 0, nil
}
