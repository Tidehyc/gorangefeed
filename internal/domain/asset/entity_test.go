package asset_test

import (
	"strings"
	"testing"
	"time"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
)

func TestFavoriteFolder_Validation(t *testing.T) {
	tests := []struct {
		name    string
		folder  domainasset.FavoriteFolder
		wantErr error
	}{
		{
			name: "valid custom folder",
			folder: domainasset.FavoriteFolder{
				UserID:      100,
				Name:        "My Favorite Music",
				Description: "Hand-picked background songs",
				IsPublic:    true,
			},
			wantErr: nil,
		},
		{
			name: "invalid user id",
			folder: domainasset.FavoriteFolder{
				UserID: 0,
				Name:   "Tech Videos",
			},
			wantErr: domainasset.ErrInvalidUserID,
		},
		{
			name: "empty name",
			folder: domainasset.FavoriteFolder{
				UserID: 100,
				Name:   "   ",
			},
			wantErr: domainasset.ErrEmptyFolderName,
		},
		{
			name: "name exceeds max length",
			folder: domainasset.FavoriteFolder{
				UserID: 100,
				Name:   strings.Repeat("a", domainasset.MaxFolderNameLength+1),
			},
			wantErr: domainasset.ErrFolderNameTooLong,
		},
		{
			name: "description exceeds max length",
			folder: domainasset.FavoriteFolder{
				UserID:      100,
				Name:        "Valid Name",
				Description: strings.Repeat("d", domainasset.MaxFolderDescriptionLength+1),
			},
			wantErr: domainasset.ErrFolderDescTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.folder.Validate()
			if err != tt.wantErr {
				t.Fatalf("expected err %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestFavoriteFolder_OwnershipAndDeletion(t *testing.T) {
	defaultFolder := domainasset.FavoriteFolder{
		ID:        1,
		UserID:    100,
		Name:      domainasset.DefaultFolderName,
		IsDefault: true,
	}

	customFolder := domainasset.FavoriteFolder{
		ID:        2,
		UserID:    100,
		Name:      "Coding Tutorials",
		IsDefault: false,
	}

	if !defaultFolder.IsOwnedBy(100) {
		t.Fatalf("expected ownership check to pass for owner")
	}
	if defaultFolder.IsOwnedBy(101) {
		t.Fatalf("expected ownership check to fail for different user")
	}

	if err := defaultFolder.CanDelete(); err != domainasset.ErrCannotDeleteDefaultFolder {
		t.Fatalf("expected ErrCannotDeleteDefaultFolder, got %v", err)
	}

	if err := customFolder.CanDelete(); err != nil {
		t.Fatalf("expected custom folder deletion to be allowed, got %v", err)
	}
}

func TestFavoriteFolder_ItemCount(t *testing.T) {
	folder := domainasset.FavoriteFolder{
		ID:        1,
		UserID:    100,
		ItemCount: 0,
	}

	folder.IncrItemCount()
	if folder.ItemCount != 1 {
		t.Fatalf("expected itemCount 1, got %d", folder.ItemCount)
	}

	folder.DecrItemCount()
	if folder.ItemCount != 0 {
		t.Fatalf("expected itemCount 0, got %d", folder.ItemCount)
	}

	// Should not drop below 0
	folder.DecrItemCount()
	if folder.ItemCount != 0 {
		t.Fatalf("expected itemCount to remain 0, got %d", folder.ItemCount)
	}
}

func TestFavoriteItem_Validation(t *testing.T) {
	tests := []struct {
		name    string
		item    domainasset.FavoriteItem
		wantErr error
	}{
		{
			name: "valid item",
			item: domainasset.FavoriteItem{
				FavoriteID: 1,
				VideoID:    10,
				UserID:     100,
			},
			wantErr: nil,
		},
		{
			name: "invalid folder id",
			item: domainasset.FavoriteItem{
				FavoriteID: 0,
				VideoID:    10,
				UserID:     100,
			},
			wantErr: domainasset.ErrInvalidFolderID,
		},
		{
			name: "invalid video id",
			item: domainasset.FavoriteItem{
				FavoriteID: 1,
				VideoID:    0,
				UserID:     100,
			},
			wantErr: domainasset.ErrInvalidVideoID,
		},
		{
			name: "invalid user id",
			item: domainasset.FavoriteItem{
				FavoriteID: 1,
				VideoID:    10,
				UserID:     0,
			},
			wantErr: domainasset.ErrInvalidUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.item.Validate()
			if err != tt.wantErr {
				t.Fatalf("expected err %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestWatchHistory_ValidationAndProgress(t *testing.T) {
	tests := []struct {
		name    string
		history domainasset.WatchHistory
		wantErr error
	}{
		{
			name: "valid watch history",
			history: domainasset.WatchHistory{
				UserID:         100,
				VideoID:        200,
				LastPositionMs: 5000,
				DurationMs:     10000,
			},
			wantErr: nil,
		},
		{
			name: "invalid user id",
			history: domainasset.WatchHistory{
				UserID:  0,
				VideoID: 200,
			},
			wantErr: domainasset.ErrInvalidUserID,
		},
		{
			name: "invalid video id",
			history: domainasset.WatchHistory{
				UserID:  100,
				VideoID: 0,
			},
			wantErr: domainasset.ErrInvalidVideoID,
		},
		{
			name: "negative position",
			history: domainasset.WatchHistory{
				UserID:         100,
				VideoID:        200,
				LastPositionMs: -10,
				DurationMs:     10000,
			},
			wantErr: domainasset.ErrInvalidPlaybackPosition,
		},
		{
			name: "negative duration",
			history: domainasset.WatchHistory{
				UserID:         100,
				VideoID:        200,
				LastPositionMs: 100,
				DurationMs:     -5,
			},
			wantErr: domainasset.ErrInvalidDuration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.history.Validate()
			if err != tt.wantErr {
				t.Fatalf("expected err %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestWatchHistory_ProgressUpdateAndCompletion(t *testing.T) {
	h := domainasset.WatchHistory{
		UserID:     100,
		VideoID:    200,
		DurationMs: 60000, // 60s
	}

	// 1. Partial progress (30s out of 60s -> 50%)
	h.UpdateProgress(30000, 60000)
	if h.IsFinished {
		t.Fatalf("expected not finished at 50%%")
	}
	if ratio := h.ProgressRatio(); ratio < 0.49 || ratio > 0.51 {
		t.Fatalf("expected ratio ~0.5, got %f", ratio)
	}

	// 2. High progress reaching 95% threshold (57s out of 60s -> 95%)
	h.UpdateProgress(57000, 60000)
	if !h.IsFinished {
		t.Fatalf("expected finished when reaching 95%% threshold")
	}

	// 3. MarkFinished sets position to duration
	h2 := domainasset.WatchHistory{
		UserID:     100,
		VideoID:    300,
		DurationMs: 45000,
	}
	h2.MarkFinished()
	if !h2.IsFinished {
		t.Fatalf("expected finished after MarkFinished()")
	}
	if h2.LastPositionMs != 45000 {
		t.Fatalf("expected position equal to duration, got %d", h2.LastPositionMs)
	}
	if h2.ProgressRatio() != 1.0 {
		t.Fatalf("expected ratio 1.0, got %f", h2.ProgressRatio())
	}
}

func TestHistoryRetentionPolicy(t *testing.T) {
	p := domainasset.DefaultRetentionPolicy()
	if p.MaxEntries != 100 {
		t.Fatalf("expected default max entries 100, got %d", p.MaxEntries)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("expected valid default policy, got %v", err)
	}

	invalidPolicy := domainasset.HistoryRetentionPolicy{MaxEntries: 0}
	if err := invalidPolicy.Validate(); err != domainasset.ErrInvalidRetentionLimit {
		t.Fatalf("expected ErrInvalidRetentionLimit, got %v", err)
	}
}

func TestWatchHistory_EdgeCases(t *testing.T) {
	h := domainasset.WatchHistory{
		UserID:     1,
		VideoID:    2,
		WatchedAt:  time.Now(),
		DurationMs: 0,
	}
	// Zero duration should return 0.0 ratio and not panic
	if ratio := h.ProgressRatio(); ratio != 0.0 {
		t.Fatalf("expected 0.0 ratio for 0 duration, got %f", ratio)
	}

	// Negative values clamped to 0
	h.UpdateProgress(-100, -200)
	if h.LastPositionMs != 0 || h.DurationMs != 0 {
		t.Fatalf("expected clamped values, got pos=%d dur=%d", h.LastPositionMs, h.DurationMs)
	}
}
