package creator

import (
	"context"
	"errors"
	"fmt"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"gorm.io/gorm"
)

type gormDraftRepository struct {
	db *gorm.DB
}

// NewDraftRepository creates a new GORM-backed creator Draft repository.
func NewDraftRepository(db *gorm.DB) domaincreator.DraftRepository {
	return &gormDraftRepository{db: db}
}

func (r *gormDraftRepository) Create(ctx context.Context, draft *domaincreator.Draft) error {
	if draft == nil {
		return errors.New("draft cannot be nil")
	}

	model := fromDomainDraft(draft)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("create draft error: %w", err)
	}

	draft.ID = model.ID
	draft.CreatedAt = model.CreatedAt
	draft.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *gormDraftRepository) GetByID(ctx context.Context, id int64) (*domaincreator.Draft, error) {
	if id <= 0 {
		return nil, domaincreator.ErrDraftNotFound
	}

	var model DraftModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND status != ?", id, int8(domaincreator.DraftStatusDeleted)).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domaincreator.ErrDraftNotFound
		}
		return nil, fmt.Errorf("find draft error: %w", err)
	}

	return toDomainDraft(&model), nil
}

func (r *gormDraftRepository) Update(ctx context.Context, draft *domaincreator.Draft) error {
	if draft == nil || draft.ID <= 0 {
		return domaincreator.ErrDraftNotFound
	}

	var existing DraftModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND status != ?", draft.ID, int8(domaincreator.DraftStatusDeleted)).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domaincreator.ErrDraftNotFound
		}
		return fmt.Errorf("find draft for update error: %w", err)
	}

	if existing.AuthorID != draft.AuthorID {
		return domaincreator.ErrDraftForbidden
	}

	updates := map[string]interface{}{
		"title":       draft.Title,
		"description": draft.Description,
		"play_url":    draft.PlayURL,
		"cover_url":   draft.CoverURL,
		"duration_ms": draft.DurationMs,
		"status":      int8(draft.Status),
	}

	result := r.db.WithContext(ctx).Model(&DraftModel{}).Where("id = ?", draft.ID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update draft error: %w", result.Error)
	}

	var refreshed DraftModel
	if err := r.db.WithContext(ctx).First(&refreshed, draft.ID).Error; err == nil {
		draft.UpdatedAt = refreshed.UpdatedAt
	}

	return nil
}

func (r *gormDraftRepository) Delete(ctx context.Context, id int64, authorID int64) error {
	if id <= 0 {
		return domaincreator.ErrDraftNotFound
	}

	var existing DraftModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND status != ?", id, int8(domaincreator.DraftStatusDeleted)).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domaincreator.ErrDraftNotFound
		}
		return fmt.Errorf("find draft for delete error: %w", err)
	}

	if existing.AuthorID != authorID {
		return domaincreator.ErrDraftForbidden
	}

	if err := r.db.WithContext(ctx).Model(&DraftModel{}).
		Where("id = ?", id).
		Update("status", int8(domaincreator.DraftStatusDeleted)).Error; err != nil {
		return fmt.Errorf("soft delete draft error: %w", err)
	}

	return nil
}

func (r *gormDraftRepository) ListByAuthor(
	ctx context.Context,
	authorID int64,
	cursor *domaincreator.DraftCursor,
	limit int,
) ([]*domaincreator.Draft, *domaincreator.DraftCursor, bool, error) {
	if authorID <= 0 {
		return nil, nil, false, domaincreator.ErrInvalidAuthorID
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	query := r.db.WithContext(ctx).
		Where("author_id = ? AND status != ?", authorID, int8(domaincreator.DraftStatusDeleted))

	if cursor != nil && cursor.ID > 0 {
		if !cursor.UpdatedAt.IsZero() {
			query = query.Where(
				"(updated_at < ?) OR (updated_at = ? AND id < ?)",
				cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID,
			)
		} else {
			query = query.Where("id < ?", cursor.ID)
		}
	}

	var models []DraftModel
	err := query.Order("updated_at DESC, id DESC").Limit(limit + 1).Find(&models).Error
	if err != nil {
		return nil, nil, false, fmt.Errorf("list drafts error: %w", err)
	}

	hasMore := false
	if len(models) > limit {
		hasMore = true
		models = models[:limit]
	}

	result := make([]*domaincreator.Draft, len(models))
	for i := range models {
		result[i] = toDomainDraft(&models[i])
	}

	var nextCursor *domaincreator.DraftCursor
	if hasMore && len(result) > 0 {
		last := result[len(result)-1]
		nextCursor = &domaincreator.DraftCursor{
			UpdatedAt: last.UpdatedAt,
			ID:        last.ID,
		}
	}

	return result, nextCursor, hasMore, nil
}

func (r *gormDraftRepository) PublishDraft(
	ctx context.Context,
	draftID int64,
	authorID int64,
	publishFn func(ctx context.Context, draft *domaincreator.Draft) (*domainvideo.Video, error),
) (*domainvideo.Video, error) {
	var publishedVideo *domainvideo.Video

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var model DraftModel
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND status != ?", draftID, int8(domaincreator.DraftStatusDeleted)).
			First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domaincreator.ErrDraftNotFound
			}
			return fmt.Errorf("lock draft error: %w", err)
		}

		if model.AuthorID != authorID {
			return domaincreator.ErrDraftForbidden
		}

		if model.Status == int8(domaincreator.DraftStatusPublished) {
			return domaincreator.ErrDraftAlreadyPublished
		}

		draft := toDomainDraft(&model)
		if err := draft.ValidateForPublish(); err != nil {
			return err
		}

		// Execute publisher function within the same transaction context if possible, or invoke
		v, err := publishFn(ctx, draft)
		if err != nil {
			return fmt.Errorf("publish video error: %w", err)
		}
		publishedVideo = v

		// Update draft status to published
		if err := tx.Model(&DraftModel{}).
			Where("id = ?", draftID).
			Updates(map[string]interface{}{
				"status": int8(domaincreator.DraftStatusPublished),
			}).Error; err != nil {
			return fmt.Errorf("update draft status to published error: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return publishedVideo, nil
}
