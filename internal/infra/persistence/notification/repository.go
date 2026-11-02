package notification

import (
	"context"
	"fmt"
	"time"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a GORM-based Notification repository.
func NewRepository(db *gorm.DB) domainnotification.Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, n *domainnotification.Notification) error {
	if n == nil {
		return nil
	}
	model := fromDomain(n)
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "recipient_id"}, {Name: "event_id"}},
		DoNothing: true,
	}).Create(model)

	if err := result.Error; err != nil {
		return fmt.Errorf("create notification error: %w", err)
	}

	if result.RowsAffected == 0 && model.EventID != "" {
		var existing NotificationModel
		if err := r.db.WithContext(ctx).
			Where("recipient_id = ? AND event_id = ?", model.RecipientID, model.EventID).
			First(&existing).Error; err == nil {
			n.ID = existing.ID
			n.CreatedAt = existing.CreatedAt
			n.IsRead = existing.IsRead
			n.ReadAt = existing.ReadAt
			return nil
		}
	}

	n.ID = model.ID
	n.CreatedAt = model.CreatedAt
	return nil
}

func (r *gormRepository) BatchCreate(ctx context.Context, ns []*domainnotification.Notification) error {
	if len(ns) == 0 {
		return nil
	}
	models := make([]*NotificationModel, len(ns))
	for i, n := range ns {
		models[i] = fromDomain(n)
	}

	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "recipient_id"}, {Name: "event_id"}},
		DoNothing: true,
	}).Create(&models).Error; err != nil {
		return fmt.Errorf("batch create notifications error: %w", err)
	}

	for i, m := range models {
		ns[i].ID = m.ID
		ns[i].CreatedAt = m.CreatedAt
	}
	return nil
}

func (r *gormRepository) ListByRecipient(
	ctx context.Context,
	recipientID int64,
	cursor *domainnotification.NotificationCursor,
	limit int,
	filterType string,
) ([]*domainnotification.Notification, *domainnotification.NotificationCursor, bool, error) {
	if limit <= 0 {
		limit = domainnotification.DefaultLimit
	} else if limit > domainnotification.MaxLimit {
		limit = domainnotification.MaxLimit
	}

	query := r.db.WithContext(ctx).Model(&NotificationModel{}).Where("recipient_id = ?", recipientID)
	if filterType != "" {
		query = query.Where("type = ?", filterType)
	}

	if cursor != nil && !cursor.CreatedAt.IsZero() && cursor.ID > 0 {
		query = query.Where("(created_at < ? OR (created_at = ? AND id < ?))", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}

	var models []NotificationModel
	if err := query.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&models).Error; err != nil {
		return nil, nil, false, fmt.Errorf("list notifications error: %w", err)
	}

	hasMore := len(models) > limit
	if hasMore {
		models = models[:limit]
	}

	items := make([]*domainnotification.Notification, len(models))
	actorIDs := make([]int64, 0, len(models))
	for i := range models {
		items[i] = toDomain(&models[i])
		if models[i].ActorID > 0 {
			actorIDs = append(actorIDs, models[i].ActorID)
		}
	}

	r.hydrateActors(ctx, items, actorIDs)

	var nextCursor *domainnotification.NotificationCursor
	if len(items) > 0 {
		nextCursor = items[len(items)-1].Cursor()
	}

	return items, nextCursor, hasMore, nil
}

func (r *gormRepository) CountUnread(ctx context.Context, recipientID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&NotificationModel{}).
		Where("recipient_id = ? AND is_read = ?", recipientID, false).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count unread notifications error: %w", err)
	}
	return count, nil
}

func (r *gormRepository) MarkAsRead(ctx context.Context, recipientID int64, notificationIDs []int64) (int64, error) {
	if len(notificationIDs) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&NotificationModel{}).
		Where("recipient_id = ? AND id IN (?) AND is_read = ?", recipientID, notificationIDs, false).
		Updates(map[string]any{
			"is_read": true,
			"read_at": now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("mark notifications read error: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *gormRepository) MarkAllAsRead(ctx context.Context, recipientID int64) (int64, error) {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&NotificationModel{}).
		Where("recipient_id = ? AND is_read = ?", recipientID, false).
		Updates(map[string]any{
			"is_read": true,
			"read_at": now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("mark all notifications read error: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *gormRepository) ListAfter(
	ctx context.Context,
	recipientID int64,
	after time.Time,
	minID int64,
	limit int,
) ([]*domainnotification.Notification, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.db.WithContext(ctx).Model(&NotificationModel{}).
		Where("recipient_id = ? AND is_read = ?", recipientID, false)

	if !after.IsZero() && minID > 0 {
		query = query.Where("created_at >= ? AND id > ?", after, minID)
	} else if !after.IsZero() {
		query = query.Where("created_at >= ?", after)
	} else if minID > 0 {
		query = query.Where("id > ?", minID)
	}

	var models []NotificationModel
	if err := query.Order("created_at ASC, id ASC").Limit(limit).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list notifications after cursor error: %w", err)
	}

	items := make([]*domainnotification.Notification, len(models))
	actorIDs := make([]int64, 0, len(models))
	for i := range models {
		items[i] = toDomain(&models[i])
		if models[i].ActorID > 0 {
			actorIDs = append(actorIDs, models[i].ActorID)
		}
	}

	r.hydrateActors(ctx, items, actorIDs)
	return items, nil
}

func (r *gormRepository) hydrateActors(ctx context.Context, items []*domainnotification.Notification, actorIDs []int64) {
	if len(actorIDs) == 0 {
		return
	}
	var users []struct {
		ID        int64  `gorm:"column:id"`
		Username  string `gorm:"column:username"`
		Nickname  string `gorm:"column:nickname"`
		AvatarURL string `gorm:"column:avatar_url"`
	}
	if err := r.db.WithContext(ctx).Table("users").
		Select("id, username, nickname, avatar_url").
		Where("id IN (?)", actorIDs).
		Find(&users).Error; err != nil {
		return
	}

	actorMap := make(map[int64]*domainnotification.ActorInfo, len(users))
	for _, u := range users {
		actorMap[u.ID] = &domainnotification.ActorInfo{
			ID:        u.ID,
			Username:  u.Username,
			Nickname:  u.Nickname,
			AvatarURL: u.AvatarURL,
		}
	}

	for _, item := range items {
		if actor, ok := actorMap[item.ActorID]; ok {
			item.WithActor(actor)
		}
	}
}
