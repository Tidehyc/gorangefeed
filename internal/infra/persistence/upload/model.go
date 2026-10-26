package upload

import (
	"time"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
)

// UploadSessionModel represents the database schema for the upload_sessions table.
type UploadSessionModel struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	UploadID    string    `gorm:"column:upload_id;type:varchar(64);uniqueIndex;not null"`
	UserID      int64     `gorm:"column:user_id;index:idx_user_file_hash;index:idx_user_id;not null"`
	FileHash    string    `gorm:"column:file_hash;type:char(64);index:idx_user_file_hash;not null"`
	FileName    string    `gorm:"column:file_name;type:varchar(255);not null"`
	FileSize    int64     `gorm:"column:file_size;not null"`
	ChunkSize   int       `gorm:"column:chunk_size;not null"`
	TotalChunks int       `gorm:"column:total_chunks;not null"`
	Status      int8      `gorm:"column:status;index:idx_user_file_hash;default:1;not null"`
	PlayURL     *string   `gorm:"column:play_url;type:varchar(512)"`
	ExpireAt    time.Time `gorm:"column:expire_at;index:idx_expire_at;not null"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName overrides the GORM table name.
func (UploadSessionModel) TableName() string {
	return "upload_sessions"
}

// ToDomain converts persistent model to domain aggregate root.
func (m *UploadSessionModel) ToDomain() *domainupload.UploadSession {
	var playURL string
	if m.PlayURL != nil {
		playURL = *m.PlayURL
	}
	return &domainupload.UploadSession{
		ID:          m.ID,
		UploadID:    m.UploadID,
		UserID:      m.UserID,
		FileHash:    m.FileHash,
		FileName:    m.FileName,
		FileSize:    m.FileSize,
		ChunkSize:   m.ChunkSize,
		TotalChunks: m.TotalChunks,
		Status:      domainupload.UploadStatus(m.Status),
		PlayURL:     playURL,
		ExpireAt:    m.ExpireAt,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// FromDomain creates a model instance from domain aggregate.
func FromDomain(s *domainupload.UploadSession) *UploadSessionModel {
	var playURL *string
	if s.PlayURL != "" {
		p := s.PlayURL
		playURL = &p
	}
	return &UploadSessionModel{
		ID:          s.ID,
		UploadID:    s.UploadID,
		UserID:      s.UserID,
		FileHash:    s.FileHash,
		FileName:    s.FileName,
		FileSize:    s.FileSize,
		ChunkSize:   s.ChunkSize,
		TotalChunks: s.TotalChunks,
		Status:      int8(s.Status),
		PlayURL:     playURL,
		ExpireAt:    s.ExpireAt,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}
