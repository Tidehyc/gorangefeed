package account

import (
	"context"
	"errors"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/config"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
)

type stubProfileStatsProvider struct {
	stats  domainaccount.ProfileStats
	err    error
	called bool
	lastID int64
}

func (s *stubProfileStatsProvider) GetProfileStats(ctx context.Context, userID int64) (domainaccount.ProfileStats, error) {
	s.called = true
	s.lastID = userID
	if s.err != nil {
		return domainaccount.ProfileStats{}, s.err
	}
	return s.stats, nil
}

func setupServiceWithStats(provider domainaccount.ProfileStatsProvider) Service {
	cfg := config.JWTConfig{
		AccessSecret:  "test_secret_for_access_123456",
		RefreshSecret: "test_secret_for_refresh_123456",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    7 * 24 * time.Hour,
	}
	return NewService(newMockRepository(), newMockTokenStore(), jwt.NewTokenManager(cfg), cfg, WithProfileStats(provider))
}

func TestService_GetProfileStats_ReturnsProviderNumbers(t *testing.T) {
	provider := &stubProfileStatsProvider{stats: domainaccount.ProfileStats{
		FollowingCount: 12,
		FollowerCount:  34,
		WorkCount:      5,
		TotalLikes:     678,
	}}
	svc := setupServiceWithStats(provider)

	got, err := svc.GetProfileStats(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetProfileStats failed: %v", err)
	}
	if !provider.called || provider.lastID != 42 {
		t.Fatalf("expected the provider to be asked for user 42, got called=%v id=%d", provider.called, provider.lastID)
	}
	want := domainaccount.ProfileStats{FollowingCount: 12, FollowerCount: 34, WorkCount: 5, TotalLikes: 678}
	if got != want {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestService_GetProfileStats_ProviderErrorDegradesToZero(t *testing.T) {
	// A profile page must still render when the stats read model is down: zeroes are a worse
	// number, not a broken page.
	svc := setupServiceWithStats(&stubProfileStatsProvider{err: errors.New("db down")})

	got, err := svc.GetProfileStats(context.Background(), 42)
	if err != nil {
		t.Fatalf("a stats failure must not surface as an error, got %v", err)
	}
	if got != (domainaccount.ProfileStats{}) {
		t.Fatalf("expected zeroed stats, got %+v", got)
	}
}

func TestService_GetProfileStats_WithoutProviderReturnsZero(t *testing.T) {
	svc := setupServiceWithStats(nil)

	got, err := svc.GetProfileStats(context.Background(), 42)
	if err != nil {
		t.Fatalf("a service without a stats provider must not fail, got %v", err)
	}
	if got != (domainaccount.ProfileStats{}) {
		t.Fatalf("expected zeroed stats, got %+v", got)
	}
}
