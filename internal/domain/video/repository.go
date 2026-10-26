package video

import "context"

// Repository defines the data persistence contracts for the video domain.
type Repository interface {
	// CreateWithStats atomically inserts video metadata and initializes its stats row.
	CreateWithStats(ctx context.Context, video *Video, stats *VideoStats) error

	// FindByID retrieves a published video by its ID. Returns ErrVideoNotFound if not found or hidden.
	FindByID(ctx context.Context, id int64) (*Video, error)

	// FindDetailByID retrieves a published video and its engagement statistics.
	FindDetailByID(ctx context.Context, id int64) (*Video, *VideoStats, error)

	// FindByAuthorAndIdempotencyKey retrieves an existing video by author ID and idempotency key.
	FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*Video, *VideoStats, error)

	// ListByAuthorCursor returns a paginated list of published videos and their stats by author ID,
	// ordered by id DESC using cursor pagination.
	// Returns: videos, stats, nextCursor (0 if end), hasMore, error.
	ListByAuthorCursor(ctx context.Context, authorID int64, cursor int64, limit int) ([]*Video, []*VideoStats, int64, bool, error)

	// UpdateStatus updates the video status. Enforces author authorization.
	UpdateStatus(ctx context.Context, id int64, authorID int64, status VideoStatus) error
}
