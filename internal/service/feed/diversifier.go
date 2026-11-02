package feed

import (
	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
)

type slidingWindowDiversifier struct{}

// NewAuthorDiversifier creates an instance of AuthorDiversifier implementing sliding window separation.
func NewAuthorDiversifier() domainfeed.AuthorDiversifier {
	return &slidingWindowDiversifier{}
}

func (d *slidingWindowDiversifier) Diversify(
	videoIDs []int64,
	authorMap map[int64]int64,
	limit int,
	interval int,
) []int64 {
	if len(videoIDs) == 0 {
		return []int64{}
	}
	if interval <= 0 {
		interval = 3
	}
	if limit <= 0 || limit > len(videoIDs) {
		limit = len(videoIDs)
	}

	result := make([]int64, 0, limit)
	buffer := make([]int64, 0)
	window := domainfeed.NewAuthorWindow(interval)

	for _, id := range videoIDs {
		authorID := authorMap[id]

		if authorID > 0 && window.Contains(authorID) {
			// Author collision within sliding window: buffer candidate for fallback
			buffer = append(buffer, id)
		} else {
			// Accept candidate and update sliding window
			result = append(result, id)
			if authorID > 0 {
				window.Add(authorID)
			}
			if len(result) >= limit {
				break
			}
		}
	}

	// Fallback: If result count is less than limit, fill remaining slots from buffer
	if len(result) < limit && len(buffer) > 0 {
		for _, id := range buffer {
			result = append(result, id)
			if len(result) >= limit {
				break
			}
		}
	}

	return result
}
