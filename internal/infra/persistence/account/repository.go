package account

import (
	"context"
	"errors"
	"fmt"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type gormRepository struct {
	db *gorm.DB
}

// NewRepository creates a GORM-based Account repository.
func NewRepository(db *gorm.DB) domainaccount.Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, user *domainaccount.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return domainaccount.ErrUsernameTaken
		}
		return fmt.Errorf("create user error: %w", err)
	}
	return nil
}

func (r *gormRepository) FindByID(ctx context.Context, id int64) (*domainaccount.User, error) {
	var user domainaccount.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainaccount.ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by id error: %w", err)
	}
	return &user, nil
}

func (r *gormRepository) FindByUsername(ctx context.Context, username string) (*domainaccount.User, error) {
	var user domainaccount.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainaccount.ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by username error: %w", err)
	}
	return &user, nil
}

func (r *gormRepository) UpdateProfile(ctx context.Context, id int64, nickname, bio string) error {
	res := r.db.WithContext(ctx).Model(&domainaccount.User{}).Where("id = ?", id).Updates(map[string]interface{}{
		"nickname": nickname,
		"bio":      bio,
	})
	if res.Error != nil {
		return fmt.Errorf("update profile error: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domainaccount.ErrUserNotFound
	}
	return nil
}

func (r *gormRepository) UpdateAvatar(ctx context.Context, id int64, avatarURL string) error {
	res := r.db.WithContext(ctx).Model(&domainaccount.User{}).Where("id = ?", id).Update("avatar_url", avatarURL)
	if res.Error != nil {
		return fmt.Errorf("update avatar error: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domainaccount.ErrUserNotFound
	}
	return nil
}
