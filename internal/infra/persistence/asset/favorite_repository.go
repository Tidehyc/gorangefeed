package asset

import (
	"context"
	"errors"
	"fmt"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	persistenceoutbox "github.com/Tidehyc/gorangefeed/internal/infra/persistence/outbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type favoriteRepository struct {
	db *gorm.DB
}

// NewFavoriteRepository creates a GORM-backed FavoriteRepository implementation.
func NewFavoriteRepository(db *gorm.DB) domainasset.FavoriteRepository {
	return &favoriteRepository{db: db}
}

func (r *favoriteRepository) GetOrCreateDefaultFolder(ctx context.Context, userID int64) (*domainasset.FavoriteFolder, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}

	var m FavoriteModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND is_default = ?", userID, 1).
		First(&m).Error

	if err == nil {
		return toDomainFolder(&m), nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("query default folder error: %w", err)
	}

	// Default folder does not exist; initialize it
	isDefaultVal := int8(1)
	defaultFolder := FavoriteModel{
		UserID:      userID,
		Name:        domainasset.DefaultFolderName,
		Description: domainasset.DefaultFolderDesc,
		IsPublic:    1,
		IsDefault:   &isDefaultVal,
		ItemCount:   0,
	}

	if createErr := r.db.WithContext(ctx).Create(&defaultFolder).Error; createErr != nil {
		// Handle potential race condition on concurrent initialization
		var existing FavoriteModel
		if queryErr := r.db.WithContext(ctx).
			Where("user_id = ? AND is_default = ?", userID, 1).
			First(&existing).Error; queryErr == nil {
			return toDomainFolder(&existing), nil
		}
		return nil, fmt.Errorf("create default folder error: %w", createErr)
	}

	return toDomainFolder(&defaultFolder), nil
}

func (r *favoriteRepository) CreateFolder(ctx context.Context, folder *domainasset.FavoriteFolder) error {
	if folder == nil {
		return errors.New("folder cannot be nil")
	}
	if err := folder.Validate(); err != nil {
		return err
	}
	// Custom folders created via CreateFolder cannot be default
	folder.IsDefault = false

	m := fromDomainFolder(folder)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return fmt.Errorf("create favorite folder error: %w", err)
	}

	folder.ID = m.ID
	folder.CreatedAt = m.CreatedAt
	folder.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *favoriteRepository) FindFolderByID(ctx context.Context, folderID int64) (*domainasset.FavoriteFolder, error) {
	if folderID <= 0 {
		return nil, domainasset.ErrInvalidFolderID
	}

	var m FavoriteModel
	err := r.db.WithContext(ctx).Where("id = ?", folderID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainasset.ErrFolderNotFound
		}
		return nil, fmt.Errorf("find favorite folder error: %w", err)
	}

	return toDomainFolder(&m), nil
}

func (r *favoriteRepository) ListFolders(ctx context.Context, userID int64) ([]*domainasset.FavoriteFolder, error) {
	if userID <= 0 {
		return nil, domainasset.ErrInvalidUserID
	}

	var models []FavoriteModel
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("is_default DESC, id ASC").
		Find(&models).Error

	if err != nil {
		return nil, fmt.Errorf("list favorite folders error: %w", err)
	}

	folders := make([]*domainasset.FavoriteFolder, len(models))
	for i := range models {
		folders[i] = toDomainFolder(&models[i])
	}
	return folders, nil
}

func (r *favoriteRepository) DeleteFolder(ctx context.Context, userID, folderID int64) error {
	if userID <= 0 {
		return domainasset.ErrInvalidUserID
	}
	if folderID <= 0 {
		return domainasset.ErrInvalidFolderID
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m FavoriteModel
		if err := tx.Where("id = ?", folderID).First(&m).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domainasset.ErrFolderNotFound
			}
			return fmt.Errorf("query folder before deletion error: %w", err)
		}

		if m.UserID != userID {
			return domainasset.ErrFolderAccessDenied
		}

		if m.IsDefault != nil && *m.IsDefault == 1 {
			return domainasset.ErrCannotDeleteDefaultFolder
		}

		// Retrieve all video IDs contained in this folder to decrement stats
		var videoIDs []int64
		if err := tx.Model(&FavoriteItemModel{}).
			Where("favorite_id = ?", folderID).
			Pluck("video_id", &videoIDs).Error; err != nil {
			return fmt.Errorf("pluck folder video IDs error: %w", err)
		}

		if len(videoIDs) > 0 {
			if err := tx.Exec("UPDATE video_stats SET favorite_count = GREATEST(favorite_count - 1, 0) WHERE video_id IN (?)", videoIDs).Error; err != nil {
				return fmt.Errorf("decrement video favorite_counts error: %w", err)
			}
			if err := tx.Where("favorite_id = ?", folderID).Delete(&FavoriteItemModel{}).Error; err != nil {
				return fmt.Errorf("delete folder items error: %w", err)
			}
		}

		if err := tx.Where("id = ?", folderID).Delete(&FavoriteModel{}).Error; err != nil {
			return fmt.Errorf("delete folder error: %w", err)
		}

		return nil
	})
}

func (r *favoriteRepository) AddItem(ctx context.Context, item *domainasset.FavoriteItem, outboxEvent *domainoutbox.Event) (bool, error) {
	if item == nil {
		return false, errors.New("favorite item cannot be nil")
	}
	if err := item.Validate(); err != nil {
		return false, err
	}

	var inserted bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var folder FavoriteModel
		if err := tx.Where("id = ?", item.FavoriteID).First(&folder).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domainasset.ErrFolderNotFound
			}
			return fmt.Errorf("query favorite folder error: %w", err)
		}

		if folder.UserID != item.UserID {
			return domainasset.ErrFolderAccessDenied
		}

		itemModel := fromDomainItem(item)
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(itemModel)
		if result.Error != nil {
			return fmt.Errorf("insert favorite item error: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			// Already favorited in this folder
			inserted = false
			return nil
		}

		inserted = true
		item.ID = itemModel.ID
		item.CreatedAt = itemModel.CreatedAt

		// Increment folder item_count
		if err := tx.Exec("UPDATE favorites SET item_count = item_count + 1 WHERE id = ?", item.FavoriteID).Error; err != nil {
			return fmt.Errorf("increment folder item_count error: %w", err)
		}

		// Atomically increment video_stats.favorite_count
		if err := tx.Exec("UPDATE video_stats SET favorite_count = favorite_count + 1 WHERE video_id = ?", item.VideoID).Error; err != nil {
			return fmt.Errorf("increment video favorite_count error: %w", err)
		}

		// Persist outbox event if provided
		if outboxEvent != nil {
			if err := persistenceoutbox.SaveEventInTx(ctx, tx, outboxEvent); err != nil {
				return err
			}
		}

		return nil
	})

	return inserted, err
}

func (r *favoriteRepository) RemoveItem(ctx context.Context, favoriteID, videoID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	if favoriteID <= 0 {
		return false, domainasset.ErrInvalidFolderID
	}
	if videoID <= 0 {
		return false, domainasset.ErrInvalidVideoID
	}

	var removed bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("favorite_id = ? AND video_id = ?", favoriteID, videoID).
			Delete(&FavoriteItemModel{})
		if result.Error != nil {
			return fmt.Errorf("delete favorite item error: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			removed = false
			return nil
		}

		removed = true

		// Decrement folder item_count
		if err := tx.Exec("UPDATE favorites SET item_count = GREATEST(item_count - 1, 0) WHERE id = ?", favoriteID).Error; err != nil {
			return fmt.Errorf("decrement folder item_count error: %w", err)
		}

		// Atomically decrement video_stats.favorite_count
		if err := tx.Exec("UPDATE video_stats SET favorite_count = GREATEST(favorite_count - 1, 0) WHERE video_id = ?", videoID).Error; err != nil {
			return fmt.Errorf("decrement video favorite_count error: %w", err)
		}

		// Persist outbox event if provided
		if outboxEvent != nil {
			if err := persistenceoutbox.SaveEventInTx(ctx, tx, outboxEvent); err != nil {
				return err
			}
		}

		return nil
	})

	return removed, err
}

func (r *favoriteRepository) IsFavorited(ctx context.Context, userID, videoID int64) (bool, error) {
	if userID <= 0 || videoID <= 0 {
		return false, nil
	}

	var count int64
	err := r.db.WithContext(ctx).
		Model(&FavoriteItemModel{}).
		Where("user_id = ? AND video_id = ?", userID, videoID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check is favorited error: %w", err)
	}

	return count > 0, nil
}

func (r *favoriteRepository) BatchIsFavorited(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error) {
	res := make(map[int64]bool, len(videoIDs))
	for _, id := range videoIDs {
		res[id] = false
	}
	if userID <= 0 || len(videoIDs) == 0 {
		return res, nil
	}

	var favoritedVideoIDs []int64
	err := r.db.WithContext(ctx).
		Model(&FavoriteItemModel{}).
		Where("user_id = ? AND video_id IN (?)", userID, videoIDs).
		Distinct().
		Pluck("video_id", &favoritedVideoIDs).Error
	if err != nil {
		return nil, fmt.Errorf("batch check is favorited error: %w", err)
	}

	for _, vid := range favoritedVideoIDs {
		res[vid] = true
	}
	return res, nil
}

func (r *favoriteRepository) ListItems(ctx context.Context, favoriteID int64, cursor int64, limit int) ([]*domainasset.FavoriteItem, int64, bool, error) {
	if favoriteID <= 0 {
		return nil, 0, false, domainasset.ErrInvalidFolderID
	}
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	query := r.db.WithContext(ctx).
		Where("favorite_id = ?", favoriteID)

	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}

	var models []FavoriteItemModel
	if err := query.Order("id DESC").Limit(limit + 1).Find(&models).Error; err != nil {
		return nil, 0, false, fmt.Errorf("list favorite items error: %w", err)
	}

	hasMore := len(models) > limit
	if hasMore {
		models = models[:limit]
	}

	items := make([]*domainasset.FavoriteItem, len(models))
	for i := range models {
		items[i] = toDomainItem(&models[i])
	}

	var nextCursor int64 = 0
	if len(items) > 0 && hasMore {
		nextCursor = items[len(items)-1].ID
	}

	return items, nextCursor, hasMore, nil
}
