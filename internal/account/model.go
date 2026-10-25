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

// User represents a user account record in MySQL.
type User struct {
	ID           int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string     `gorm:"column:username;uniqueIndex;size:32;not null" json:"username"`
	PasswordHash string     `gorm:"column:password_hash;size:255;not null" json:"-"`
	Nickname     string     `gorm:"column:nickname;size:64;not null;default:''" json:"nickname"`
	AvatarURL    string     `gorm:"column:avatar_url;size:255;not null;default:''" json:"avatar_url"`
	Bio          string     `gorm:"column:bio;size:255;not null;default:''" json:"bio"`
	Status       UserStatus `gorm:"column:status;type:enum('active','suspended','deleted');not null;default:'active'" json:"status"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime:milli" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime:milli" json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}
