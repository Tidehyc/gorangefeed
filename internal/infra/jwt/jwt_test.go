package jwt

import (
	"testing"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
)

func TestTokenManager_AccessAndRefreshToken(t *testing.T) {
	cfg := config.JWTConfig{
		AccessSecret:  "test_access_secret_1234567890",
		RefreshSecret: "test_refresh_secret_1234567890",
		AccessTTL:     1 * time.Hour,
		RefreshTTL:    24 * time.Hour,
	}
	tm := NewTokenManager(cfg)

	userID := int64(1001)
	tokenID := "tok-1234"
	sessionID := "sess-5678"

	// 1. Access Token Generation & Verification
	accessToken, accExp, err := tm.GenerateAccessToken(userID, tokenID)
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}
	if accessToken == "" || accExp.Before(time.Now()) {
		t.Fatalf("invalid accessToken output: %v, exp: %v", accessToken, accExp)
	}

	accClaims, err := tm.VerifyAccessToken(accessToken)
	if err != nil {
		t.Fatalf("VerifyAccessToken failed: %v", err)
	}
	if accClaims.UserID != userID || accClaims.TokenID != tokenID {
		t.Fatalf("claims mismatch: got %+v", accClaims)
	}

	// 2. Refresh Token Generation & Verification
	refreshToken, refExp, err := tm.GenerateRefreshToken(userID, sessionID, tokenID)
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}
	if refreshToken == "" || refExp.Before(time.Now()) {
		t.Fatalf("invalid refreshToken output: %v, exp: %v", refreshToken, refExp)
	}

	refClaims, err := tm.VerifyRefreshToken(refreshToken)
	if err != nil {
		t.Fatalf("VerifyRefreshToken failed: %v", err)
	}
	if refClaims.UserID != userID || refClaims.SessionID != sessionID || refClaims.TokenID != tokenID {
		t.Fatalf("refresh claims mismatch: got %+v", refClaims)
	}

	// 3. Cross Verification Should Fail
	if _, err := tm.VerifyAccessToken(refreshToken); err == nil {
		t.Fatalf("expected error verifying refreshToken with access secret, got nil")
	}
	if _, err := tm.VerifyRefreshToken(accessToken); err == nil {
		t.Fatalf("expected error verifying accessToken with refresh secret, got nil")
	}

	// 4. Invalid token string
	if _, err := tm.VerifyAccessToken("invalid.jwt.token"); err == nil {
		t.Fatalf("expected error for invalid token string")
	}
}

func TestTokenManager_ExpiredToken(t *testing.T) {
	cfg := config.JWTConfig{
		AccessSecret:  "test_access_secret_1234567890",
		RefreshSecret: "test_refresh_secret_1234567890",
		AccessTTL:     -1 * time.Second, // already expired
		RefreshTTL:    -1 * time.Second, // already expired
	}
	tm := NewTokenManager(cfg)

	accToken, _, err := tm.GenerateAccessToken(1, "tok-exp")
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	_, err = tm.VerifyAccessToken(accToken)
	if err != ErrExpiredToken {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}

	refToken, _, err := tm.GenerateRefreshToken(1, "sess-1", "tok-exp")
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}

	_, err = tm.VerifyRefreshToken(refToken)
	if err != ErrExpiredToken {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}
