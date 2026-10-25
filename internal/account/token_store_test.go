package account

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func getTestRedisClient(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6380",
		DB:   1, // use DB 1 for testing
	})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("skipping Redis token store test: redis not reachable on 127.0.0.1:6380 (%v)", err)
		return nil
	}
	return client
}

func TestRedisTokenStore_SessionLifecycleAndReplay(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()
	store := NewTokenStore(client)

	sessionID := "test-sess-" + time.Now().Format("20060102150405.000")
	token1 := "tok-version-1"
	token2 := "tok-version-2"
	token3 := "tok-version-3"

	// Cleanup at the end
	defer func() {
		_ = store.RevokeSession(ctx, sessionID)
	}()

	// 1. Create Session
	err := store.CreateSession(ctx, sessionID, token1, 5*time.Second)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// 2. Normal rotation: token1 -> token2
	res, err := store.RotateSession(ctx, sessionID, token1, token2, 5*time.Second)
	if err != nil {
		t.Fatalf("RotateSession failed: %v", err)
	}
	if res != RotationOK {
		t.Fatalf("expected RotationOK, got %v", res)
	}

	// 3. Replay attack: try to rotate again using old token1!
	res, err = store.RotateSession(ctx, sessionID, token1, token3, 5*time.Second)
	if err != nil {
		t.Fatalf("RotateSession failed on replay attempt: %v", err)
	}
	if res != RotationReplayDetected {
		t.Fatalf("expected RotationReplayDetected, got %v", res)
	}

	// 4. Session should be deleted by the Lua script upon replay
	res, err = store.RotateSession(ctx, sessionID, token2, token3, 5*time.Second)
	if err != nil {
		t.Fatalf("RotateSession failed after replay: %v", err)
	}
	if res != RotationSessionNotFound {
		t.Fatalf("expected RotationSessionNotFound after replay revocation, got %v", res)
	}
}

func TestRedisTokenStore_Blacklist(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()
	store := NewTokenStore(client)

	tokenID := "tok-blacklist-" + time.Now().Format("20060102150405.000")

	// Initially not blacklisted
	isBlacklisted, err := store.IsAccessTokenBlacklisted(ctx, tokenID)
	if err != nil {
		t.Fatalf("check blacklist failed: %v", err)
	}
	if isBlacklisted {
		t.Fatalf("expected token not to be blacklisted initially")
	}

	// Blacklist it for 2 seconds
	err = store.BlacklistAccessToken(ctx, tokenID, 2*time.Second)
	if err != nil {
		t.Fatalf("BlacklistAccessToken failed: %v", err)
	}

	// Now should be blacklisted
	isBlacklisted, err = store.IsAccessTokenBlacklisted(ctx, tokenID)
	if err != nil {
		t.Fatalf("check blacklist failed: %v", err)
	}
	if !isBlacklisted {
		t.Fatalf("expected token to be blacklisted")
	}
}
