package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
)

type stubVideoHider struct {
	calls    int
	lastUID  int64
	err      error
	affected int64
}

func (s *stubVideoHider) HideAllByAuthor(ctx context.Context, authorID int64) (int64, error) {
	s.calls++
	s.lastUID = authorID
	if s.err != nil {
		return 0, s.err
	}
	return s.affected, nil
}

func setupServiceWithHider() (Service, *mockRepository, *mockTokenStore, *stubVideoHider) {
	cfg := config.JWTConfig{
		AccessSecret:  "test_secret_for_access_123456",
		RefreshSecret: "test_secret_for_refresh_123456",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    7 * 24 * time.Hour,
	}
	repo := newMockRepository()
	store := newMockTokenStore()
	hider := &stubVideoHider{affected: 2}
	svc := NewService(repo, store, jwt.NewTokenManager(cfg), cfg, WithVideoHider(hider))
	return svc, repo, store, hider
}

func assertAppCode(t *testing.T, err error, want string) {
	t.Helper()
	var appErr *response.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected an AppError with code %q, got %v", want, err)
	}
	if appErr.Code != want {
		t.Fatalf("expected error code %q, got %q (%s)", want, appErr.Code, appErr.Message)
	}
}

func TestService_ChangePassword(t *testing.T) {
	ctx := context.Background()
	svc, _, store, _ := setupServiceWithHider()

	user, err := svc.Register(ctx, "alice_user", "Password123")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 1. The current password must match before anything is rotated.
	err = svc.ChangePassword(ctx, user.ID, "WrongPassword123", "NewPassword456")
	assertAppCode(t, err, response.CodeUnauthorized)

	// 2. Reusing the same password defeats the point of the endpoint.
	err = svc.ChangePassword(ctx, user.ID, "Password123", "Password123")
	assertAppCode(t, err, response.CodeInvalidParam)

	// 3. The new password goes through the same complexity rules as registration.
	err = svc.ChangePassword(ctx, user.ID, "Password123", "short")
	assertAppCode(t, err, response.CodeInvalidParam)

	// 4. Nothing above should have touched the stored credential.
	if _, _, err := svc.Login(ctx, "alice_user", "Password123"); err != nil {
		t.Fatalf("original password should still work after rejected changes, got %v", err)
	}

	// 5. A valid change replaces the hash and retires previously issued tokens.
	if err := svc.ChangePassword(ctx, user.ID, "Password123", "NewPassword456"); err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}
	if _, _, err := svc.Login(ctx, "alice_user", "Password123"); err == nil {
		t.Fatalf("old password should stop working after the change")
	}
	if _, _, err := svc.Login(ctx, "alice_user", "NewPassword456"); err != nil {
		t.Fatalf("new password should work, got %v", err)
	}
	if _, ok := store.invalidated[user.ID]; !ok {
		t.Fatalf("expected issued tokens to be invalidated for user %d", user.ID)
	}

	// 6. A token minted before the cutoff is rejected; one minted after is accepted.
	cutoff := store.invalidated[user.ID]
	if stale, _ := store.IsUserTokenInvalidated(ctx, user.ID, cutoff.Add(-time.Minute)); !stale {
		t.Fatalf("token issued before the cutoff must be treated as invalidated")
	}
	if stale, _ := store.IsUserTokenInvalidated(ctx, user.ID, cutoff.Add(time.Minute)); stale {
		t.Fatalf("token issued after the cutoff must stay valid")
	}
}

func TestService_ChangePassword_UnknownUser(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := setupServiceWithHider()

	err := svc.ChangePassword(ctx, 999, "Password123", "NewPassword456")
	assertAppCode(t, err, response.CodeNotFound)
}

func TestService_DeleteAccount(t *testing.T) {
	ctx := context.Background()
	svc, repo, store, hider := setupServiceWithHider()

	user, err := svc.Register(ctx, "bob_delete", "Password123")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	// The mock repository hands back the same pointer it stores, so the original handle has to be
	// copied now: reading user.Username after the deletion would read the rewritten value.
	originalUsername := user.Username

	// 1. A wrong password must not delete anything, and must not hide content either.
	err = svc.DeleteAccount(ctx, user.ID, "WrongPassword123")
	assertAppCode(t, err, response.CodeUnauthorized)
	if hider.calls != 0 {
		t.Fatalf("content must not be touched when the password is wrong, got %d calls", hider.calls)
	}

	// 2. If hiding fails the account stays usable so the caller can retry; the reverse order
	//    would leave a deleted account whose videos are still live.
	hider.err = errors.New("storage unavailable")
	if err := svc.DeleteAccount(ctx, user.ID, "Password123"); err == nil {
		t.Fatalf("expected the hide failure to abort the deletion")
	}
	stored, _ := repo.FindByID(ctx, user.ID)
	if stored.Status != domainaccount.UserStatusActive {
		t.Fatalf("account should still be active after an aborted deletion, got %q", stored.Status)
	}

	// 3. Successful deletion hides content, soft-deletes the account, and cuts tokens.
	hider.err = nil
	if err := svc.DeleteAccount(ctx, user.ID, "Password123"); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}
	if hider.lastUID != user.ID {
		t.Fatalf("expected videos of user %d to be hidden, got %d", user.ID, hider.lastUID)
	}
	stored, _ = repo.FindByID(ctx, user.ID)
	if stored.Status != domainaccount.UserStatusDeleted {
		t.Fatalf("expected status %q, got %q", domainaccount.UserStatusDeleted, stored.Status)
	}
	if _, ok := store.invalidated[user.ID]; !ok {
		t.Fatalf("expected issued tokens to be invalidated for user %d", user.ID)
	}

	// 4. Personal identifiers must not survive: comment payloads carry the nickname and the
	//    handle, so leaving either in place would keep the author identifiable in every thread.
	if stored.Nickname != "已注销用户" {
		t.Fatalf("expected the nickname to be anonymised, got %q", stored.Nickname)
	}
	if stored.Username == originalUsername {
		t.Fatalf("expected the handle to be rewritten, still %q", stored.Username)
	}
	if !strings.HasPrefix(stored.Username, "deleted_") {
		t.Fatalf("expected a deleted_<id> handle, got %q", stored.Username)
	}
	if stored.AvatarURL != "" || stored.Bio != "" {
		t.Fatalf("expected avatar and bio to be cleared, got avatar=%q bio=%q", stored.AvatarURL, stored.Bio)
	}

	// 5. A deleted account cannot sign back in.
	_, _, err = svc.Login(ctx, "bob_delete", "Password123")
	if err == nil {
		t.Fatalf("deleted account must not be able to log in")
	}
}
