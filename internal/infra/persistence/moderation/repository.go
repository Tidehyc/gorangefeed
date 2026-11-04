package moderation

import (
	"context"
	"errors"
	"fmt"

	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
	"gorm.io/gorm"
)

type repository struct {
	db *gorm.DB
}

// NewRepository creates a new GORM-backed implementation of moderation.ReviewRepository.
func NewRepository(db *gorm.DB) domainmoderation.ReviewRepository {
	return &repository{db: db}
}

func (r *repository) CreateTask(ctx context.Context, task *domainmoderation.ReviewTask) error {
	if task == nil {
		return errors.New("cannot create nil task")
	}
	m := toTaskModel(task)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("create review task: %w", err)
	}
	task.ID = m.ID
	return nil
}

func (r *repository) GetTaskByVideoID(ctx context.Context, videoID int64) (*domainmoderation.ReviewTask, error) {
	var m ReviewTaskModel
	err := r.db.WithContext(ctx).Where("video_id = ?", videoID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainmoderation.ErrTaskNotFound
		}
		return nil, fmt.Errorf("get review task by video_id: %w", err)
	}
	return toDomainTask(&m), nil
}

func (r *repository) GetTaskByID(ctx context.Context, id int64) (*domainmoderation.ReviewTask, error) {
	var m ReviewTaskModel
	err := r.db.WithContext(ctx).First(&m, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainmoderation.ErrTaskNotFound
		}
		return nil, fmt.Errorf("get review task by id: %w", err)
	}
	return toDomainTask(&m), nil
}

func (r *repository) UpdateTask(ctx context.Context, task *domainmoderation.ReviewTask) error {
	if task == nil {
		return errors.New("cannot update nil task")
	}
	m := toTaskModel(task)
	updates := map[string]interface{}{
		"status":        m.Status,
		"risk_score":    m.RiskScore,
		"matched_words": m.MatchedWords,
		"reviewer_id":   m.ReviewerID,
		"review_note":   m.ReviewNote,
		"updated_at":    m.UpdatedAt,
	}
	res := r.db.WithContext(ctx).Model(&ReviewTaskModel{}).Where("id = ?", task.ID).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("update review task: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domainmoderation.ErrTaskNotFound
	}
	return nil
}

func (r *repository) ListTasks(ctx context.Context, status domainmoderation.ReviewStatus, cursor int64, limit int) ([]*domainmoderation.ReviewTask, int64, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	fetchLimit := limit + 1

	query := r.db.WithContext(ctx).Model(&ReviewTaskModel{})
	if status > 0 {
		query = query.Where("status = ?", int8(status))
	}
	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}
	query = query.Order("id DESC").Limit(fetchLimit)

	var models []*ReviewTaskModel
	if err := query.Find(&models).Error; err != nil {
		return nil, 0, false, fmt.Errorf("list review tasks: %w", err)
	}

	hasMore := len(models) > limit
	if hasMore {
		models = models[:limit]
	}

	tasks := make([]*domainmoderation.ReviewTask, 0, len(models))
	var nextCursor int64
	for _, m := range models {
		tasks = append(tasks, toDomainTask(m))
		nextCursor = m.ID
	}

	return tasks, nextCursor, hasMore, nil
}

func (r *repository) CreateLog(ctx context.Context, log *domainmoderation.ModerationLog) error {
	if log == nil {
		return errors.New("cannot create nil log")
	}
	m := toLogModel(log)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("create moderation log: %w", err)
	}
	log.ID = m.ID
	return nil
}

func (r *repository) ListLogs(ctx context.Context, targetType string, targetID int64, limit int) ([]*domainmoderation.ModerationLog, error) {
	if limit <= 0 {
		limit = 20
	}
	var models []*ModerationLogModel
	query := r.db.WithContext(ctx).Model(&ModerationLogModel{})
	if targetType != "" {
		query = query.Where("target_type = ?", targetType)
	}
	if targetID > 0 {
		query = query.Where("target_id = ?", targetID)
	}
	query = query.Order("id DESC").Limit(limit)

	if err := query.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list moderation logs: %w", err)
	}

	logs := make([]*domainmoderation.ModerationLog, 0, len(models))
	for _, m := range models {
		logs = append(logs, toDomainLog(m))
	}
	return logs, nil
}
