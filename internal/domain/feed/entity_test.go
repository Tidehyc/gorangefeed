package feed

import (
	"testing"
	"time"
)

func TestNormalizeScene(t *testing.T) {
	tests := []struct {
		input    string
		expected Scene
	}{
		{"", SceneGlobal},
		{"   ", SceneGlobal},
		{"GLOBAL", SceneGlobal},
		{"global", SceneGlobal},
		{"following", SceneFollowing},
		{"FOLLOWING ", SceneFollowing},
		{"hot", SceneHot},
		{"recommend", SceneRecommend},
		{"unknown", SceneGlobal},
	}

	for _, tt := range tests {
		got := NormalizeScene(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeScene(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCachedPage_ExpirationAndStale(t *testing.T) {
	now := time.Now()
	page := &CachedPage{
		VideoIDs:   []int64{101, 102},
		NextCursor: 101,
		HasMore:    true,
		ExpireAt:   now.Add(10 * time.Second),
	}

	// 1. Fresh: now is before ExpireAt
	if page.IsExpired(now) {
		t.Errorf("expected page not to be expired before ExpireAt")
	}
	if page.CanServeStale(now) {
		t.Errorf("expected fresh page not to require serving stale")
	}

	// 2. Soft expired, within stale tolerance (e.g. 5s past ExpireAt, tolerance is 30s)
	softExpiredTime := page.ExpireAt.Add(5 * time.Second)
	if !page.IsExpired(softExpiredTime) {
		t.Errorf("expected page to be expired at softExpiredTime")
	}
	if !page.CanServeStale(softExpiredTime) {
		t.Errorf("expected page to be serviceable as stale within 30s tolerance")
	}

	// 3. Hard expired: past StaleCacheTolerance (e.g. 35s past ExpireAt)
	hardExpiredTime := page.ExpireAt.Add(35 * time.Second)
	if !page.IsExpired(hardExpiredTime) {
		t.Errorf("expected page to be expired at hardExpiredTime")
	}
	if page.CanServeStale(hardExpiredTime) {
		t.Errorf("expected page NOT to be serviceable as stale beyond 30s tolerance")
	}
}

func TestViewEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		event   ViewEvent
		wantErr error
	}{
		{
			name: "valid play event",
			event: ViewEvent{
				VideoID:   100,
				ViewerID:  200,
				EventType: ViewEventPlay,
			},
			wantErr: nil,
		},
		{
			name: "valid complete event",
			event: ViewEvent{
				VideoID:         100,
				ViewerID:        200,
				EventType:       ViewEventComplete,
				WatchDurationMs: 5000,
			},
			wantErr: nil,
		},
		{
			name: "invalid video ID <= 0",
			event: ViewEvent{
				VideoID:   0,
				ViewerID:  200,
				EventType: ViewEventPlay,
			},
			wantErr: ErrInvalidVideoID,
		},
		{
			name: "invalid event type",
			event: ViewEvent{
				VideoID:   100,
				ViewerID:  200,
				EventType: "pause",
			},
			wantErr: ErrInvalidEventType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
