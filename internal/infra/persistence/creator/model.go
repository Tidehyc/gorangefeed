package creator

import (
	"time"

	domaincreator "github.com/Tidehyc/gorangefeed/internal/domain/creator"
)

// DraftModel maps to the `drafts` database table.
type DraftModel struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	AuthorID    int64     `gorm:"column:author_id;not null"`
	Title       string    `gorm:"column:title;type:varchar(128);not null;default:''"`
	Description string    `gorm:"column:description;type:varchar(1000);not null;default:''"`
	PlayURL     string    `gorm:"column:play_url;type:varchar(512);not null;default:''"`
	CoverURL    string    `gorm:"column:cover_url;type:varchar(512);not null;default:''"`
	DurationMs  int       `gorm:"column:duration_ms;not null;default:0"`
	Status      int8      `gorm:"column:status;not null;default:1"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the table name for DraftModel.
func (DraftModel) TableName() string {
	return "drafts"
}

// ScheduledTaskModel maps to the `scheduled_tasks` database table.
type ScheduledTaskModel struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	TaskID      string    `gorm:"column:task_id;type:char(36);not null;uniqueIndex"`
	AuthorID    int64     `gorm:"column:author_id;not null"`
	DraftID     *int64    `gorm:"column:draft_id;default:null"`
	Title       string    `gorm:"column:title;type:varchar(128);not null;default:''"`
	Description string    `gorm:"column:description;type:varchar(1000);not null;default:''"`
	PlayURL     string    `gorm:"column:play_url;type:varchar(512);not null;default:''"`
	CoverURL    string    `gorm:"column:cover_url;type:varchar(512);not null;default:''"`
	DurationMs  int       `gorm:"column:duration_ms;not null;default:0"`
	ScheduledAt time.Time `gorm:"column:scheduled_at;not null"`
	Status      int8      `gorm:"column:status;not null;default:1"`
	VideoID     *int64    `gorm:"column:video_id;default:null"`
	ErrorMsg    *string   `gorm:"column:error_msg;type:text;default:null"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the table name for ScheduledTaskModel.
func (ScheduledTaskModel) TableName() string {
	return "scheduled_tasks"
}

func toDomainDraft(m *DraftModel) *domaincreator.Draft {
	if m == nil {
		return nil
	}
	return &domaincreator.Draft{
		ID:          m.ID,
		AuthorID:    m.AuthorID,
		Title:       m.Title,
		Description: m.Description,
		PlayURL:     m.PlayURL,
		CoverURL:    m.CoverURL,
		DurationMs:  m.DurationMs,
		Status:      domaincreator.DraftStatus(m.Status),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func fromDomainDraft(d *domaincreator.Draft) *DraftModel {
	if d == nil {
		return nil
	}
	return &DraftModel{
		ID:          d.ID,
		AuthorID:    d.AuthorID,
		Title:       d.Title,
		Description: d.Description,
		PlayURL:     d.PlayURL,
		CoverURL:    d.CoverURL,
		DurationMs:  d.DurationMs,
		Status:      int8(d.Status),
	}
}

func toDomainScheduledTask(m *ScheduledTaskModel) *domaincreator.ScheduledTask {
	if m == nil {
		return nil
	}
	return &domaincreator.ScheduledTask{
		ID:          m.ID,
		TaskID:      m.TaskID,
		AuthorID:    m.AuthorID,
		DraftID:     m.DraftID,
		Title:       m.Title,
		Description: m.Description,
		PlayURL:     m.PlayURL,
		CoverURL:    m.CoverURL,
		DurationMs:  m.DurationMs,
		ScheduledAt: m.ScheduledAt,
		Status:      domaincreator.ScheduledStatus(m.Status),
		VideoID:     m.VideoID,
		ErrorMsg:    m.ErrorMsg,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func fromDomainScheduledTask(t *domaincreator.ScheduledTask) *ScheduledTaskModel {
	if t == nil {
		return nil
	}
	return &ScheduledTaskModel{
		ID:          t.ID,
		TaskID:      t.TaskID,
		AuthorID:    t.AuthorID,
		DraftID:     t.DraftID,
		Title:       t.Title,
		Description: t.Description,
		PlayURL:     t.PlayURL,
		CoverURL:    t.CoverURL,
		DurationMs:  t.DurationMs,
		ScheduledAt: t.ScheduledAt,
		Status:      int8(t.Status),
		VideoID:     t.VideoID,
		ErrorMsg:    t.ErrorMsg,
	}
}
