package feed_test

import (
	"reflect"
	"testing"

	servicefeed "github.com/Tidehyc/gorangefeed/internal/service/feed"
)

func TestAuthorDiversifier_Diversify(t *testing.T) {
	diversifier := servicefeed.NewAuthorDiversifier()

	tests := []struct {
		name      string
		videoIDs  []int64
		authorMap map[int64]int64
		limit     int
		interval  int
		want      []int64
	}{
		{
			name:      "empty input",
			videoIDs:  []int64{},
			authorMap: map[int64]int64{},
			limit:     10,
			interval:  3,
			want:      []int64{},
		},
		{
			name:     "consecutive same author videos diversified with window interval 3",
			videoIDs: []int64{1, 2, 3, 4, 5, 6},
			authorMap: map[int64]int64{
				1: 100, // A
				2: 100, // A (conflict -> buffer)
				3: 100, // A (conflict -> buffer)
				4: 200, // B (accept)
				5: 300, // C (accept)
				6: 400, // D (accept, window now [B, C, D])
			},
			limit:    4,
			interval: 3,
			// result: 1 (A), 4 (B), 5 (C), 6 (D) -> length reached limit 4
			want: []int64{1, 4, 5, 6},
		},
		{
			name:     "consecutive author fallback when candidates insufficient",
			videoIDs: []int64{1, 2, 3, 4},
			authorMap: map[int64]int64{
				1: 100, // A
				2: 100, // A (conflict -> buffer)
				3: 100, // A (conflict -> buffer)
				4: 200, // B
			},
			limit:    4,
			interval: 3,
			// result first accepts 1 (A), 4 (B). Then buffers 2, 3 are used to fill up to limit 4.
			want: []int64{1, 4, 2, 3},
		},
		{
			name:     "all videos from single author graceful fallback",
			videoIDs: []int64{10, 20, 30},
			authorMap: map[int64]int64{
				10: 999,
				20: 999,
				30: 999,
			},
			limit:    3,
			interval: 3,
			// accepts 10, buffers 20, 30. Then fallbacks and fills 20, 30.
			want: []int64{10, 20, 30},
		},
		{
			name:     "unknown author (0) does not conflict",
			videoIDs: []int64{1, 2, 3},
			authorMap: map[int64]int64{
				1: 0,
				2: 0,
				3: 0,
			},
			limit:    3,
			interval: 3,
			want:     []int64{1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diversifier.Diversify(tt.videoIDs, tt.authorMap, tt.limit, tt.interval)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Diversify() = %v, want %v", got, tt.want)
			}
		})
	}
}
