package account

import (
	"context"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
)

type mockRepository struct {
	usersByID       map[int64]*domainaccount.User
	usersByUsername map[string]*domainaccount.User
	nextID          int64
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		usersByID:       make(map[int64]*domainaccount.User),
		usersByUsername: make(map[string]*domainaccount.User),
		nextID:          1,
	}
}

func (m *mockRepository) Create(ctx context.Context, u *domainaccount.User) error {
	if _, exists := m.usersByUsername[u.Username]; exists {
		return domainaccount.ErrUsernameTaken
	}
	u.ID = m.nextID
	m.nextID++
	m.usersByID[u.ID] = u
	m.usersByUsername[u.Username] = u
	return nil
}

func (m *mockRepository) FindByID(ctx context.Context, id int64) (*domainaccount.User, error) {
	u, ok := m.usersByID[id]
	if !ok {
		return nil, domainaccount.ErrUserNotFound
	}
	return u, nil
}

func (m *mockRepository) FindByUsername(ctx context.Context, username string) (*domainaccount.User, error) {
	u, ok := m.usersByUsername[username]
	if !ok {
		return nil, domainaccount.ErrUserNotFound
	}
	return u, nil
}

func (m *mockRepository) UpdateProfile(ctx context.Context, id int64, nickname, bio string) error {
	u, ok := m.usersByID[id]
	if !ok {
		return domainaccount.ErrUserNotFound
	}
	u.Nickname = nickname
	u.Bio = bio
	return nil
}

func (m *mockRepository) UpdateAvatar(ctx context.Context, id int64, avatarURL string) error {
	u, ok := m.usersByID[id]
	if !ok {
		return domainaccount.ErrUserNotFound
	}
	u.AvatarURL = avatarURL
	return nil
}

type mockTokenStore struct {
	sessions    map[string]string // sessionID -> currentTokenID
	blacklisted map[string]bool   // tokenID -> true
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{
		sessions:    make(map[string]string),
		blacklisted: make(map[string]bool),
	}
}

func (m *mockTokenStore) CreateSession(ctx context.Context, sessionID, tokenID string, ttl time.Duration) error {
	m.sessions[sessionID] = tokenID
	return nil
}

func (m *mockTokenStore) RotateSession(ctx context.Context, sessionID, oldTokenID, newTokenID string, ttl time.Duration) (domainaccount.RotationResult, error) {
	cur, ok := m.sessions[sessionID]
	if !ok {
		return domainaccount.RotationSessionNotFound, nil
	}
	if cur != oldTokenID {
		delete(m.sessions, sessionID) // replay attack detected
		return domainaccount.RotationReplayDetected, nil
	}
	m.sessions[sessionID] = newTokenID
	return domainaccount.RotationOK, nil
}

func (m *mockTokenStore) RevokeSession(ctx context.Context, sessionID string) error {
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockTokenStore) BlacklistAccessToken(ctx context.Context, tokenID string, ttl time.Duration) error {
	m.blacklisted[tokenID] = true
	return nil
}

func (m *mockTokenStore) IsAccessTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	return m.blacklisted[tokenID], nil
}

func setupTestService() (Service, *mockRepository, *mockTokenStore) {
	cfg := config.JWTConfig{
		AccessSecret:  "test_secret_for_access_123456",
		RefreshSecret: "test_secret_for_refresh_123456",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    7 * 24 * time.Hour,
	}
	jwtMgr := jwt.NewTokenManager(cfg)
	repo := newMockRepository()
	store := newMockTokenStore()
	svc := NewService(repo, store, jwtMgr, cfg)
	return svc, repo, store
}

func TestService_RegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setupTestService()

	// 1. Invalid username
	if _, err := svc.Register(ctx, "a", "Password123"); err == nil {
		t.Fatalf("expected error for short username")
	}

	// 2. Invalid password
	if _, err := svc.Register(ctx, "validuser", "short"); err == nil {
		t.Fatalf("expected error for short password")
	}

	// 3. Successful registration
	user, err := svc.Register(ctx, "validuser", "Password123")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if user.ID != 1 || user.Username != "validuser" {
		t.Fatalf("unexpected user: %+v", user)
	}

	// 4. Duplicate registration
	if _, err := svc.Register(ctx, "validuser", "Password123"); err == nil {
		t.Fatalf("expected conflict error for duplicate username")
	}

	// 5. Login wrong password
	if _, _, err := svc.Login(ctx, "validuser", "WrongPassword123"); err == nil {
		t.Fatalf("expected unauthorized error for wrong password")
	}

	// 6. Login non-existent user
	if _, _, err := svc.Login(ctx, "nonexistent", "Password123"); err == nil {
		t.Fatalf("expected unauthorized error for non-existent user")
	}

	// 7. Successful login
	pair, loggedUser, err := svc.Login(ctx, "validuser", "Password123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected valid tokens, got: %+v", pair)
	}
	if loggedUser.ID != user.ID {
		t.Fatalf("expected user ID %d, got %d", user.ID, loggedUser.ID)
	}
}

func TestService_RefreshTokenRotationAndReplay(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setupTestService()

	// Register & Login
	_, err := svc.Register(ctx, "bob_user", "Password123")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	pair, _, err := svc.Login(ctx, "bob_user", "Password123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	oldRefreshToken := pair.RefreshToken

	// 1. Normal rotation
	newPair, err := svc.RefreshToken(ctx, oldRefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if newPair.RefreshToken == oldRefreshToken {
		t.Fatalf("expected new refresh token to be rotated")
	}

	// 2. Replay attack: try using oldRefreshToken again!
	_, err = svc.RefreshToken(ctx, oldRefreshToken)
	if err == nil {
		t.Fatalf("expected replay detection error, got nil")
	}

	// 3. Verify session was revoked by replay detection: even newPair.RefreshToken is now invalid!
	_, err = svc.RefreshToken(ctx, newPair.RefreshToken)
	if err == nil {
		t.Fatalf("expected session to be revoked after replay attack, got nil")
	}
}

func TestService_Logout(t *testing.T) {
	ctx := context.Background()
	svc, _, store := setupTestService()

	_, _ = svc.Register(ctx, "charlie_user", "Password123")
	pair, _, _ := svc.Login(ctx, "charlie_user", "Password123")

	// Parse refresh token to obtain session ID
	cfg := config.JWTConfig{
		AccessSecret:  "test_secret_for_access_123456",
		RefreshSecret: "test_secret_for_refresh_123456",
	}
	jwtMgr := jwt.NewTokenManager(cfg)
	claims, err := jwtMgr.VerifyRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("verify refresh token failed: %v", err)
	}

	// Logout
	err = svc.Logout(ctx, claims.SessionID, pair.AccessToken)
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	// Attempting refresh should fail because session is revoked
	_, err = svc.RefreshToken(ctx, pair.RefreshToken)
	if err == nil {
		t.Fatalf("expected refresh to fail after logout")
	}

	// Check access token was blacklisted
	accClaims, err := jwtMgr.VerifyAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	blacklisted, _ := store.IsAccessTokenBlacklisted(ctx, accClaims.TokenID)
	if !blacklisted {
		t.Fatalf("expected access token to be blacklisted")
	}
}

func TestService_Profile(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setupTestService()

	user, _ := svc.Register(ctx, "david_user", "Password123")

	// GetProfile
	p, err := svc.GetProfile(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}
	if p.Username != "david_user" {
		t.Fatalf("username mismatch: %s", p.Username)
	}

	// UpdateProfile
	updated, err := svc.UpdateProfile(ctx, user.ID, "SuperDavid", "Go developer")
	if err != nil {
		t.Fatalf("UpdateProfile failed: %v", err)
	}
	if updated.Nickname != "SuperDavid" || updated.Bio != "Go developer" {
		t.Fatalf("update profile values mismatch: %+v", updated)
	}

	// UpdateAvatar
	err = svc.UpdateAvatar(ctx, user.ID, "https://example.com/avatar.png")
	if err != nil {
		t.Fatalf("UpdateAvatar failed: %v", err)
	}
	pAfter, _ := svc.GetProfile(ctx, user.ID)
	if pAfter.AvatarURL != "https://example.com/avatar.png" {
		t.Fatalf("avatar update failed: %s", pAfter.AvatarURL)
	}

	// Non-existent user
	if _, err := svc.GetProfile(ctx, 9999); err == nil {
		t.Fatalf("expected ErrNotFound for non-existent user")
	}
	if _, err := svc.UpdateProfile(ctx, 9999, "nick", "bio"); err == nil {
		t.Fatalf("expected ErrNotFound for non-existent user")
	}
}
