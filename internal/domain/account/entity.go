package account

import (
	"time"
)

type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusDeleted   UserStatus = "deleted"
)

// User represents the domain user entity.
type User struct {
	ID           int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string     `gorm:"column:username;type:varchar(32);uniqueIndex;not null" json:"username"`
	PasswordHash string     `gorm:"column:password_hash;type:varchar(255);not null" json:"-"`
	Nickname     string     `gorm:"column:nickname;type:varchar(64);not null;default:''" json:"nickname"`
	AvatarURL    string     `gorm:"column:avatar_url;type:varchar(255);not null;default:''" json:"avatar_url"`
	Bio          string     `gorm:"column:bio;type:varchar(255);not null;default:''" json:"bio"`
	Status       UserStatus `gorm:"column:status;type:enum('active','suspended','deleted');not null;default:'active'" json:"status"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}
