package creator

import (
	"context"
	"time"

	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
)

// DraftRepository defines persistence contracts for creator drafts.
type DraftRepository interface {
	Create(ctx context.Context, draft *Draft) error
	GetByID(ctx context.Context, id int64) (*Draft, error)
	Update(ctx context.Context, draft *Draft) error
	Delete(ctx context.Context, id int64, authorID int64) error
	ListByAuthor(ctx context.Context, authorID int64, cursor *DraftCursor, limit int) ([]*Draft, *DraftCursor, bool, error)
	PublishDraft(ctx context.Context, draftID int64, authorID int64, publishFn func(ctx context.Context, draft *Draft) (*domainvideo.Video, error)) (*domainvideo.Video, error)
}

// ScheduledTaskRepository defines persistence contracts for scheduled publish tasks.
type ScheduledTaskRepository interface {
	Create(ctx context.Context, task *ScheduledTask) error
	GetByTaskID(ctx context.Context, taskID string) (*ScheduledTask, error)
	ListByAuthor(ctx context.Context, authorID int64, limit int) ([]*ScheduledTask, error)
	Cancel(ctx context.Context, taskID string, authorID int64) error
	AcquireDueTask(ctx context.Context, taskID string) (bool, error)
	MarkPublished(ctx context.Context, taskID string, videoID int64) error
	MarkFailed(ctx context.Context, taskID string, errMsg string) error
	ListDueTasks(ctx context.Context, dueTime time.Time, limit int) ([]*ScheduledTask, error)
}

// SequenceGenerator defines the contract for generating globally monotonic publish sequences.
type SequenceGenerator interface {
	NextPublishSeq(ctx context.Context) (int64, error)
	GetCurrentSeq(ctx context.Context) (int64, error)
	InitializeSequence(ctx context.Context, minVal int64) error
}
