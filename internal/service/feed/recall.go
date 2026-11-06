package feed

import (
	"context"
	"math/rand"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
)

type multiSourceRecallEngine struct {
	hotStore     domainfeed.HotTimelineStore
	timelineRepo domainfeed.TimelineRepository
	dbRepo       domainfeed.FeedDBRepository
}

// NewMultiSourceRecallEngine constructs a recall engine aggregating hot, latest, and exploratory pools.
func NewMultiSourceRecallEngine(
	hotStore domainfeed.HotTimelineStore,
	timelineRepo domainfeed.TimelineRepository,
	dbRepo domainfeed.FeedDBRepository,
) domainfeed.RecallEngine {
	return &multiSourceRecallEngine{
		hotStore:     hotStore,
		timelineRepo: timelineRepo,
		dbRepo:       dbRepo,
	}
}

// Recall aggregates the hot, latest and exploratory pools. viewerID is accepted for the M11-01
// interest-vector recall but is deliberately unused today: none of the three pools are
// viewer-aware, and pretending otherwise would hide that the personalisation work is not done.
func (e *multiSourceRecallEngine) Recall(
	ctx context.Context,
	viewerID int64,
	targetCount int,
	seed int64,
) ([]int64, error) {
	_ = viewerID
	if targetCount <= 0 {
		targetCount = 30
	}

	// 1. Calculate target allocations (50% Hot, 30% Latest, 20% Explore)
	rule := domainfeed.DefaultRecommendationRule()
	hotTarget := int(float64(targetCount) * rule.HotRatio)
	latestTarget := int(float64(targetCount) * rule.LatestRatio)
	exploreTarget := targetCount - hotTarget - latestTarget
	if hotTarget <= 0 {
		hotTarget = 1
	}
	if latestTarget <= 0 {
		latestTarget = 1
	}
	if exploreTarget <= 0 {
		exploreTarget = 1
	}

	// 2. Fetch candidates from 3 recall channels
	// 2a. Hot high-quality pool (50%)
	hotCandidates := e.recallHot(ctx, hotTarget)

	// 2b. Latest timeliness pool (30%)
	latestCandidates := e.recallLatest(ctx, latestTarget)

	// 2c. Random exploration pool (20%)
	exploreCandidates := e.recallExplore(ctx, exploreTarget, seed)

	// 3. Merge & Deduplicate
	seen := make(map[int64]bool)
	candidates := make([]int64, 0, targetCount)

	addCandidate := func(id int64) {
		if id > 0 && !seen[id] {
			seen[id] = true
			candidates = append(candidates, id)
		}
	}

	// Interleave or append according to priority
	for _, id := range hotCandidates {
		addCandidate(id)
	}
	for _, id := range latestCandidates {
		addCandidate(id)
	}
	for _, id := range exploreCandidates {
		addCandidate(id)
	}

	// 4. Smooth Compensation (互补平滑降级)
	// If candidates count is less than targetCount, fetch additional cold videos to fulfill targetCount
	if len(candidates) < targetCount && e.dbRepo != nil {
		remainNeeded := targetCount - len(candidates)
		coldCards, _, _, _, err := e.dbRepo.ListColdVideos(ctx, 0, remainNeeded*2)
		if err == nil {
			for _, card := range coldCards {
				addCandidate(card.VideoID)
				if len(candidates) >= targetCount {
					break
				}
			}
		}
	}

	return candidates, nil
}

func (e *multiSourceRecallEngine) recallHot(ctx context.Context, target int) []int64 {
	var candidates []int64

	if e.hotStore != nil {
		asOf := time.Now().Truncate(time.Minute)
		_, _ = e.hotStore.MergeWindow(ctx, asOf, 60)
		ids, err := e.hotStore.ListBySnapshot(ctx, asOf, 0, target*2)
		if err == nil && len(ids) > 0 {
			candidates = append(candidates, ids...)
		}
	}

	// Cold start fallback: ListLikeRankVideos from DB if hot store returns insufficient candidates
	if len(candidates) < target && e.dbRepo != nil {
		likeVideos, err := e.dbRepo.ListLikeRankVideos(ctx, 0, 0, target)
		if err == nil {
			for _, v := range likeVideos {
				candidates = append(candidates, v.VideoID)
			}
		}
	}

	return candidates
}

func (e *multiSourceRecallEngine) recallLatest(ctx context.Context, target int) []int64 {
	var candidates []int64

	if e.timelineRepo != nil {
		ids, err := e.timelineRepo.ListByCursor(ctx, 0, target*2)
		if err == nil && len(ids) > 0 {
			candidates = append(candidates, ids...)
		}
	}

	if len(candidates) < target && e.dbRepo != nil {
		coldCards, _, _, _, err := e.dbRepo.ListColdVideos(ctx, 0, target)
		if err == nil {
			for _, card := range coldCards {
				candidates = append(candidates, card.VideoID)
			}
		}
	}

	return candidates
}

func (e *multiSourceRecallEngine) recallExplore(ctx context.Context, target int, seed int64) []int64 {
	if e.dbRepo == nil {
		return nil
	}

	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	r := rand.New(rand.NewSource(seed))

	// Fetch a pool of candidates and randomly sample
	fetchCount := target * 3
	if fetchCount < 20 {
		fetchCount = 20
	}
	coldCards, _, _, _, err := e.dbRepo.ListColdVideos(ctx, 0, fetchCount)
	if err != nil || len(coldCards) == 0 {
		return nil
	}

	// Fisher-Yates shuffle
	indices := r.Perm(len(coldCards))
	candidates := make([]int64, 0, target)
	for _, idx := range indices {
		candidates = append(candidates, coldCards[idx].VideoID)
		if len(candidates) >= target {
			break
		}
	}

	return candidates
}
