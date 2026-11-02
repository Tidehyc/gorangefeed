package creator

import (
	"context"
	"errors"
	"fmt"
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	"gorm.io/gorm"
)

type gormScheduledTaskRepository struct {
	db *gorm.DB
}

// NewScheduledTaskRepository creates a new GORM-backed ScheduledTask repository.
func NewScheduledTaskRepository(db *gorm.DB) domaincreator.ScheduledTaskRepository {
	return &gormScheduledTaskRepository{db: db}
}

func (r *gormScheduledTaskRepository) Create(ctx context.Context, task *domaincreator.ScheduledTask) error {
	if task == nil {
		return errors.New("scheduled task cannot be nil")
	}

	model := fromDomainScheduledTask(task)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("create scheduled task error: %w", err)
	}

	task.ID = model.ID
	task.CreatedAt = model.CreatedAt
	task.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *gormScheduledTaskRepository) GetByTaskID(ctx context.Context, taskID string) (*domaincreator.ScheduledTask, error) {
	if taskID == "" {
		return nil, domaincreator.ErrTaskNotFound
	}

	var model ScheduledTaskModel
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domaincreator.ErrTaskNotFound
		}
		return nil, fmt.Errorf("find scheduled task error: %w", err)
	}

	return toDomainScheduledTask(&model), nil
}

func (r *gormScheduledTaskRepository) ListByAuthor(ctx context.Context, authorID int64, limit int) ([]*domaincreator.ScheduledTask, error) {
	if authorID <= 0 {
		return nil, domaincreator.ErrInvalidAuthorID
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	var models []ScheduledTaskModel
	err := r.db.WithContext(ctx).
		Where("author_id = ?", authorID).
		Order("scheduled_at DESC, id DESC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list scheduled tasks error: %w", err)
	}

	result := make([]*domaincreator.ScheduledTask, len(models))
	for i := range models {
		result[i] = toDomainScheduledTask(&models[i])
	}

	return result, nil
}

func (r *gormScheduledTaskRepository) Cancel(ctx context.Context, taskID string, authorID int64) error {
	if taskID == "" {
		return domaincreator.ErrTaskNotFound
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model ScheduledTaskModel
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("task_id = ?", taskID).
			First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domaincreator.ErrTaskNotFound
			}
			return fmt.Errorf("lock scheduled task error: %w", err)
		}

		if model.AuthorID != authorID {
			return domaincreator.ErrTaskForbidden
		}

		if model.Status != int8(domaincreator.ScheduledStatusScheduled) {
			return domaincreator.ErrTaskNotScheduled
		}

		// Update task status to canceled
		if err := tx.Model(&ScheduledTaskModel{}).
			Where("task_id = ?", taskID).
			Update("status", int8(domaincreator.ScheduledStatusCanceled)).Error; err != nil {
			return fmt.Errorf("cancel task error: %w", err)
		}

		// If linked to a draft, revert draft status back to draft
		if model.DraftID != nil && *model.DraftID > 0 {
			if err := tx.Model(&DraftModel{}).
				Where("id = ?", *model.DraftID).
				Update("status", int8(domaincreator.DraftStatusDraft)).Error; err != nil {
				return fmt.Errorf("revert draft to draft status error: %w", err)
			}
		}

		return nil
	})
}

func (r *gormScheduledTaskRepository) AcquireDueTask(ctx context.Context, taskID string) (bool, error) {
	if taskID == "" {
		return false, domaincreator.ErrTaskNotFound
	}

	result := r.db.WithContext(ctx).Model(&ScheduledTaskModel{}).
		Where("task_id = ? AND status = ?", taskID, int8(domaincreator.ScheduledStatusScheduled)).
		Update("status", int8(domaincreator.ScheduledStatusExecuting))

	if result.Error != nil {
		return false, fmt.Errorf("acquire due task error: %w", result.Error)
	}

	return result.RowsAffected > 0, nil
}

func (r *gormScheduledTaskRepository) MarkPublished(ctx context.Context, taskID string, videoID int64) error {
	if taskID == "" {
		return domaincreator.ErrTaskNotFound
	}

	result := r.db.WithContext(ctx).Model(&ScheduledTaskModel{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":   int8(domaincreator.ScheduledStatusPublished),
			"video_id": videoID,
		})

	if result.Error != nil {
		return fmt.Errorf("mark task published error: %w", result.Error)
	}

	return nil
}

func (r *gormScheduledTaskRepository) MarkFailed(ctx context.Context, taskID string, errMsg string) error {
	if taskID == "" {
		return domaincreator.ErrTaskNotFound
	}

	result := r.db.WithContext(ctx).Model(&ScheduledTaskModel{}).
		Where("task_id = ?", taskID).
		Updates(map[string]interface{}{
			"status":    int8(domaincreator.ScheduledStatusFailed),
			"error_msg": errMsg,
		})

	if result.Error != nil {
		return fmt.Errorf("mark task failed error: %w", result.Error)
	}

	return nil
}

func (r *gormScheduledTaskRepository) ListDueTasks(ctx context.Context, dueTime time.Time, limit int) ([]*domaincreator.ScheduledTask, error) {
	if limit <= 0 {
		limit = 50
	}

	var models []ScheduledTaskModel
	err := r.db.WithContext(ctx).
		Where("status = ? AND scheduled_at <= ?", int8(domaincreator.ScheduledStatusScheduled), dueTime).
		Order("scheduled_at ASC, id ASC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list due tasks error: %w", err)
	}

	result := make([]*domaincreator.ScheduledTask, len(models))
	for i := range models {
		result[i] = toDomainScheduledTask(&models[i])
	}

	return result, nil
}
