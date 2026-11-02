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

func TestHotCursor_EncodeAndDecode(t *testing.T) {
	// 1. Zero/Empty cursor
	emptyCursor := EncodeHotCursor(0, 0)
	if emptyCursor != "" {
		t.Fatalf("expected empty cursor string, got %s", emptyCursor)
	}

	asOf, offset, err := DecodeHotCursor("")
	if err != nil || asOf != 0 || offset != 0 {
		t.Fatalf("expected (0, 0, nil), got (%d, %d, %v)", asOf, offset, err)
	}

	asOf, offset, err = DecodeHotCursor("0")
	if err != nil || asOf != 0 || offset != 0 {
		t.Fatalf("expected (0, 0, nil), got (%d, %d, %v)", asOf, offset, err)
	}

	// 2. Normal roundtrip
	expectedAsOf := int64(1730000000)
	expectedOffset := 20
	encoded := EncodeHotCursor(expectedAsOf, expectedOffset)
	if encoded == "" {
		t.Fatalf("expected non-empty encoded cursor")
	}

	asOf, offset, err = DecodeHotCursor(encoded)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if asOf != expectedAsOf || offset != expectedOffset {
		t.Fatalf("roundtrip mismatch: got (%d, %d), want (%d, %d)", asOf, offset, expectedAsOf, expectedOffset)
	}

	// 3. Invalid inputs
	_, _, err = DecodeHotCursor("invalid-base64%%%")
	if err != ErrInvalidHotCursor {
		t.Fatalf("expected ErrInvalidHotCursor for bad base64, got %v", err)
	}

	_, _, err = DecodeHotCursor("e30=") // "{}"
	if err != nil {
		// asOf=0, offset=0 is valid
	}
}

func TestRecommendCursor_EncodeAndDecode(t *testing.T) {
	// 1. Initial page
	emptyCursor := EncodeRecommendCursor(1, 0)
	if emptyCursor != "" {
		t.Fatalf("expected empty string for initial page, got %s", emptyCursor)
	}

	page, seed, err := DecodeRecommendCursor("")
	if err != nil || page != 1 || seed != 0 {
		t.Fatalf("expected (1, 0, nil), got (%d, %d, %v)", page, seed, err)
	}

	// 2. Normal roundtrip
	expectedPage := 3
	expectedSeed := int64(987654321)
	encoded := EncodeRecommendCursor(expectedPage, expectedSeed)
	if encoded == "" {
		t.Fatalf("expected non-empty encoded cursor")
	}

	page, seed, err = DecodeRecommendCursor(encoded)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if page != expectedPage || seed != expectedSeed {
		t.Fatalf("roundtrip mismatch: got (%d, %d), want (%d, %d)", page, seed, expectedPage, expectedSeed)
	}

	// 3. Invalid inputs
	_, _, err = DecodeRecommendCursor("not-valid-base64!!!")
	if err != ErrInvalidRecommendCursor {
		t.Fatalf("expected ErrInvalidRecommendCursor for bad base64, got %v", err)
	}
}

func TestAuthorWindow(t *testing.T) {
	w := NewAuthorWindow(3)
	if w.Contains(101) {
		t.Fatalf("expected window to be empty")
	}

	w.Add(101)
	w.Add(102)
	w.Add(103)

	if !w.Contains(101) || !w.Contains(102) || !w.Contains(103) {
		t.Fatalf("expected 101, 102, 103 to be present")
	}

	// Adding 4th element should slide out 101
	w.Add(104)
	if w.Contains(101) {
		t.Fatalf("expected 101 to have slid out")
	}
	if !w.Contains(104) {
		t.Fatalf("expected 104 to be present")
	}

	// Non-positive IDs are ignored
	w.Add(0)
	w.Add(-5)
	if w.Contains(0) {
		t.Fatalf("expected non-positive IDs to not be stored")
	}

	// Reset
	w.Reset()
	if w.Contains(102) || w.Contains(103) || w.Contains(104) {
		t.Fatalf("expected window to be empty after reset")
	}
}

func TestRecommendationRule(t *testing.T) {
	r := DefaultRecommendationRule()
	if r.HotRatio != 0.5 || r.LatestRatio != 0.3 || r.ExploreRatio != 0.2 {
		t.Fatalf("unexpected default recommendation ratios: %+v", r)
	}
	if r.WindowSize != 3 {
		t.Fatalf("unexpected default window size: %d", r.WindowSize)
	}
}

