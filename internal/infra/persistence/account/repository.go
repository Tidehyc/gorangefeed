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

func (r *gormRepository) UpdateStatus(ctx context.Context, id int64, status domainaccount.UserStatus) error {
	res := r.db.WithContext(ctx).Model(&domainaccount.User{}).Where("id = ?", id).Update("status", status)
	if res.Error != nil {
		return fmt.Errorf("update status error: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domainaccount.ErrUserNotFound
	}
	return nil
}

func (r *gormRepository) UpdatePassword(ctx context.Context, id int64, passwordHash string) error {
	res := r.db.WithContext(ctx).Model(&domainaccount.User{}).Where("id = ?", id).Update("password_hash", passwordHash)
	if res.Error != nil {
		return fmt.Errorf("update password error: %w", res.Error)
	}
	// MySQL reports 0 rows affected when the new hash equals the stored one. bcrypt salts every
	// hash, so an unchanged hash cannot happen here; a 0 count therefore means "no such user".
	if res.RowsAffected == 0 {
		return domainaccount.ErrUserNotFound
	}
	return nil
}

// anonymizedNickname is what every surviving reference (comment threads, mention records) will
// render once the account is gone.
const anonymizedNickname = "已注销用户"

func (r *gormRepository) Anonymize(ctx context.Context, id int64) error {
	// The username has to move as well: comment payloads carry it, so leaving it in place would
	// keep the original handle visible across every thread the account ever commented in.
	// deleted_<id> is unique by construction and fits the 32-character column.
	res := r.db.WithContext(ctx).Model(&domainaccount.User{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"username":   gorm.Expr("CONCAT('deleted_', id)"),
			"nickname":   anonymizedNickname,
			"avatar_url": "",
			"bio":        "",
		})
	if res.Error != nil {
		return fmt.Errorf("anonymize user error: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domainaccount.ErrUserNotFound
	}
	return nil
}
