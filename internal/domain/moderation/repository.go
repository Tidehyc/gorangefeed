package moderation

import "context"

// ReviewRepository defines the persistence contract for content moderation tasks and audit logs.
type ReviewRepository interface {
	// CreateTask persists a new moderation task.
	CreateTask(ctx context.Context, task *ReviewTask) error

	// GetTaskByVideoID retrieves the review task associated with a specific video.
	GetTaskByVideoID(ctx context.Context, videoID int64) (*ReviewTask, error)

	// GetTaskByID retrieves the review task by primary key ID.
	GetTaskByID(ctx context.Context, id int64) (*ReviewTask, error)

	// UpdateTask persists changes to an existing review task.
	UpdateTask(ctx context.Context, task *ReviewTask) error

	// ListTasks retrieves a paginated list of tasks filtered by status.
	// Returns tasks, nextCursor, hasMore, error.
	ListTasks(ctx context.Context, status ReviewStatus, cursor int64, limit int) ([]*ReviewTask, int64, bool, error)

	// CreateLog records an audit action in moderation_logs.
	CreateLog(ctx context.Context, log *ModerationLog) error

	// ListLogs retrieves audit logs for a given target entity.
	ListLogs(ctx context.Context, targetType string, targetID int64, limit int) ([]*ModerationLog, error)
}
