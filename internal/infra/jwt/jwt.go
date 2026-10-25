package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
)

// AccessClaims represents the claims inside an Access Token.
type AccessClaims struct {
	UserID  int64  `json:"user_id"`
	TokenID string `json:"token_id"`
	jwt.RegisteredClaims
}

// RefreshClaims represents the claims inside a Refresh Token.
type RefreshClaims struct {
	UserID    int64  `json:"user_id"`
	SessionID string `json:"session_id"`
	TokenID   string `json:"token_id"`
	jwt.RegisteredClaims
}

// TokenManager manages JWT token generation and verification.
type TokenManager struct {
	cfg config.JWTConfig
}

// NewTokenManager constructs a new TokenManager.
func NewTokenManager(cfg config.JWTConfig) *TokenManager {
	return &TokenManager{cfg: cfg}
}

// GenerateAccessToken signs a short-lived access token.
func (m *TokenManager) GenerateAccessToken(userID int64, tokenID string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.cfg.AccessTTL)

	claims := AccessClaims{
		UserID:  userID,
		TokenID: tokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "gorangefeed",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(m.cfg.AccessSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token error: %w", err)
	}

	return tokenStr, expiresAt, nil
}

// GenerateRefreshToken signs a long-lived refresh token.
func (m *TokenManager) GenerateRefreshToken(userID int64, sessionID, tokenID string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.cfg.RefreshTTL)

	claims := RefreshClaims{
		UserID:    userID,
		SessionID: sessionID,
		TokenID:   tokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "gorangefeed",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(m.cfg.RefreshSecret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign refresh token error: %w", err)
	}

	return tokenStr, expiresAt, nil
}

// VerifyAccessToken validates the signature and expiration of an access token.
func (m *TokenManager) VerifyAccessToken(tokenStr string) (*AccessClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(m.cfg.AccessSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// VerifyRefreshToken validates the signature and expiration of a refresh token.
func (m *TokenManager) VerifyRefreshToken(tokenStr string) (*RefreshClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &RefreshClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(m.cfg.RefreshSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*RefreshClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
