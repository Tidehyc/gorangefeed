package upload

import (
	"context"
	"errors"
	"time"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	"gorm.io/gorm"
)

// SessionRepository implements domainupload.SessionRepository via GORM.
type SessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository creates a new GORM-backed upload session repository.
func NewSessionRepository(db *gorm.DB) domainupload.SessionRepository {
	return &SessionRepository{db: db}
}

// Create persists a new upload session record.
func (r *SessionRepository) Create(ctx context.Context, session *domainupload.UploadSession) error {
	m := FromDomain(session)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	session.ID = m.ID
	session.CreatedAt = m.CreatedAt
	session.UpdatedAt = m.UpdatedAt
	return nil
}

// FindByUploadID retrieves an upload session by uploadID.
func (r *SessionRepository) FindByUploadID(ctx context.Context, uploadID string) (*domainupload.UploadSession, error) {
	var m UploadSessionModel
	if err := r.db.WithContext(ctx).Where("upload_id = ?", uploadID).Take(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainupload.ErrSessionNotFound
		}
		return nil, err
	}
	return m.ToDomain(), nil
}

// FindActiveByUserAndFileHash finds an active (non-expired, uploading) session for the user and file hash.
func (r *SessionRepository) FindActiveByUserAndFileHash(ctx context.Context, userID int64, fileHash string) (*domainupload.UploadSession, error) {
	var m UploadSessionModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND file_hash = ? AND status = ? AND expire_at > ?",
			userID, fileHash, int8(domainupload.UploadStatusUploading), time.Now()).
		Order("id DESC").
		Take(&m).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainupload.ErrSessionNotFound
		}
		return nil, err
	}
	return m.ToDomain(), nil
}

// TransitionStatus atomically transitions session status from expectedStatus to newStatus via CAS.
// Returns true if a row was updated (status matched).
func (r *SessionRepository) TransitionStatus(ctx context.Context, uploadID string, expectedStatus, newStatus domainupload.UploadStatus, playURL string) (bool, error) {
	updates := map[string]interface{}{
		"status": int8(newStatus),
	}
	if playURL != "" {
		updates["play_url"] = playURL
	}

	result := r.db.WithContext(ctx).
		Model(&UploadSessionModel{}).
		Where("upload_id = ? AND status = ?", uploadID, int8(expectedStatus)).
		Updates(updates)

	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
