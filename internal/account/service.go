package account

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/google/uuid"
)

// TokenPair represents the access and refresh token returned to client.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// Service defines application-level use cases for accounts.
type Service interface {
	Register(ctx context.Context, username, password string) (*User, error)
	Login(ctx context.Context, username, password string) (*TokenPair, *User, error)
	RefreshToken(ctx context.Context, refreshTokenStr string) (*TokenPair, error)
	Logout(ctx context.Context, sessionID, accessTokenStr string) error
	GetProfile(ctx context.Context, userID int64) (*User, error)
	UpdateProfile(ctx context.Context, userID int64, nickname, bio string) (*User, error)
	UpdateAvatar(ctx context.Context, userID int64, avatarURL string) error
}

type service struct {
	repo       Repository
	tokenStore TokenStore
	jwtManager *jwt.TokenManager
	cfg        config.JWTConfig
}

// NewService creates a new account Service.
func NewService(repo Repository, tokenStore TokenStore, jwtManager *jwt.TokenManager, cfg config.JWTConfig) Service {
	return &service{
		repo:       repo,
		tokenStore: tokenStore,
		jwtManager: jwtManager,
		cfg:        cfg,
	}
}

func (s *service) Register(ctx context.Context, username, password string) (*User, error) {
	if err := ValidateUsername(username); err != nil {
		return nil, response.ErrInvalidParam(err.Error())
	}
	if err := ValidatePassword(password); err != nil {
		return nil, response.ErrInvalidParam(err.Error())
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password failed: %w", err)
	}

	user := &User{
		Username:     username,
		PasswordHash: hash,
		Nickname:     username,
		Status:       UserStatusActive,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		if errors.Is(err, ErrUsernameDuplicate) {
			return nil, response.ErrConflict("Username is already taken")
		}
		return nil, err
	}

	return user, nil
}

func (s *service) Login(ctx context.Context, username, password string) (*TokenPair, *User, error) {
	user, err := s.repo.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, nil, response.ErrUnauthorized("Invalid username or password")
		}
		return nil, nil, err
	}

	if user.Status != UserStatusActive {
		return nil, nil, response.ErrForbidden("User account is inactive or suspended")
	}

	if !CheckPassword(password, user.PasswordHash) {
		return nil, nil, response.ErrUnauthorized("Invalid username or password")
	}

	sessionID := uuid.NewString()
	accTokenID := uuid.NewString()
	refTokenID := uuid.NewString()

	accessToken, _, err := s.jwtManager.GenerateAccessToken(user.ID, accTokenID)
	if err != nil {
		return nil, nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, _, err := s.jwtManager.GenerateRefreshToken(user.ID, sessionID, refTokenID)
	if err != nil {
		return nil, nil, fmt.Errorf("generate refresh token: %w", err)
	}

	if err := s.tokenStore.CreateSession(ctx, sessionID, refTokenID, s.cfg.RefreshTTL); err != nil {
		return nil, nil, fmt.Errorf("create session in token store: %w", err)
	}

	pair := &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.cfg.AccessTTL.Seconds()),
		TokenType:    "Bearer",
	}

	return pair, user, nil
}

func (s *service) RefreshToken(ctx context.Context, refreshTokenStr string) (*TokenPair, error) {
	refClaims, err := s.jwtManager.VerifyRefreshToken(refreshTokenStr)
	if err != nil {
		return nil, response.ErrUnauthorized("Invalid or expired refresh token")
	}

	newTokenID := uuid.NewString()
	rotationResult, err := s.tokenStore.RotateSession(ctx, refClaims.SessionID, refClaims.TokenID, newTokenID, s.cfg.RefreshTTL)
	if err != nil {
		return nil, fmt.Errorf("rotate token session: %w", err)
	}

	switch rotationResult {
	case RotationSessionNotFound:
		return nil, response.ErrUnauthorized("Session expired or already logged out")
	case RotationReplayDetected:
		return nil, response.ErrUnauthorized("Security violation: Token replay detected. Session terminated.")
	case RotationOK:
		// Succeeded
	default:
		return nil, response.ErrUnauthorized("Unable to validate token rotation")
	}

	newAccTokenID := uuid.NewString()
	newAccessToken, _, err := s.jwtManager.GenerateAccessToken(refClaims.UserID, newAccTokenID)
	if err != nil {
		return nil, fmt.Errorf("generate new access token: %w", err)
	}

	newRefreshToken, _, err := s.jwtManager.GenerateRefreshToken(refClaims.UserID, refClaims.SessionID, newTokenID)
	if err != nil {
		return nil, fmt.Errorf("generate new refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int64(s.cfg.AccessTTL.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *service) Logout(ctx context.Context, sessionID, accessTokenStr string) error {
	if sessionID != "" {
		_ = s.tokenStore.RevokeSession(ctx, sessionID)
	}

	if accessTokenStr != "" {
		claims, err := s.jwtManager.VerifyAccessToken(accessTokenStr)
		if err == nil && claims != nil {
			remaining := time.Until(claims.ExpiresAt.Time)
			if remaining > 0 {
				_ = s.tokenStore.BlacklistAccessToken(ctx, claims.TokenID, remaining)
			}
		}
	}

	return nil
}

func (s *service) GetProfile(ctx context.Context, userID int64) (*User, error) {
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, response.ErrNotFound("User not found")
		}
		return nil, err
	}
	return user, nil
}

func (s *service) UpdateProfile(ctx context.Context, userID int64, nickname, bio string) (*User, error) {
	if len(nickname) > 64 {
		return nil, response.ErrInvalidParam("Nickname length cannot exceed 64 characters")
	}
	if len(bio) > 255 {
		return nil, response.ErrInvalidParam("Bio length cannot exceed 255 characters")
	}

	if err := s.repo.UpdateProfile(ctx, userID, nickname, bio); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, response.ErrNotFound("User not found")
		}
		return nil, err
	}

	return s.repo.FindByID(ctx, userID)
}

func (s *service) UpdateAvatar(ctx context.Context, userID int64, avatarURL string) error {
	if err := s.repo.UpdateAvatar(ctx, userID, avatarURL); err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return response.ErrNotFound("User not found")
		}
		return err
	}
	return nil
}
