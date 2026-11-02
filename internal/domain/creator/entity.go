package creator

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// DraftStatus represents lifecycle states of a creator draft.
type DraftStatus int8

const (
	DraftStatusDraft     DraftStatus = 1
	DraftStatusScheduled DraftStatus = 2
	DraftStatusPublished DraftStatus = 3
	DraftStatusDeleted   DraftStatus = 4
)

func (s DraftStatus) String() string {
	switch s {
	case DraftStatusDraft:
		return "draft"
	case DraftStatusScheduled:
		return "scheduled"
	case DraftStatusPublished:
		return "published"
	case DraftStatusDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

// ScheduledStatus represents execution states of a scheduled task.
type ScheduledStatus int8

const (
	ScheduledStatusScheduled ScheduledStatus = 1
	ScheduledStatusExecuting ScheduledStatus = 2
	ScheduledStatusPublished ScheduledStatus = 3
	ScheduledStatusCanceled  ScheduledStatus = 4
	ScheduledStatusFailed    ScheduledStatus = 5
)

func (s ScheduledStatus) String() string {
	switch s {
	case ScheduledStatusScheduled:
		return "scheduled"
	case ScheduledStatusExecuting:
		return "executing"
	case ScheduledStatusPublished:
		return "published"
	case ScheduledStatusCanceled:
		return "canceled"
	case ScheduledStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

const (
	MaxDraftTitleLength       = 128
	MaxDraftDescriptionLength = 1000
	MaxDraftPlayURLLength     = 512
	MaxDraftCoverURLLength    = 512

	MinScheduledLeadTime = 5 * time.Minute
	MaxScheduledLeadTime = 30 * 24 * time.Hour
)

// Draft represents a saved video draft in the Creator Center.
type Draft struct {
	ID          int64       `json:"id"`
	AuthorID    int64       `json:"author_id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	PlayURL     string      `json:"play_url"`
	CoverURL    string      `json:"cover_url"`
	DurationMs  int         `json:"duration_ms"`
	Status      DraftStatus `json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// StatusName returns a human-readable string of the draft status.
func (d *Draft) StatusName() string {
	return d.Status.String()
}

// ValidateForSave checks basic field length limits for saving or updating drafts.
func (d *Draft) ValidateForSave() error {
	if d.AuthorID <= 0 {
		return ErrInvalidAuthorID
	}
	if len(d.Title) > MaxDraftTitleLength {
		return ErrDraftTitleTooLong
	}
	if len(d.Description) > MaxDraftDescriptionLength {
		return ErrDraftDescTooLong
	}
	if len(d.PlayURL) > MaxDraftPlayURLLength {
		return ErrDraftPlayURLTooLong
	}
	if len(d.CoverURL) > MaxDraftCoverURLLength {
		return ErrDraftCoverURLTooLong
	}
	return nil
}

// ValidateForPublish ensures all required fields are present when transitioning from draft to published video.
func (d *Draft) ValidateForPublish() error {
	if err := d.ValidateForSave(); err != nil {
		return err
	}
	if strings.TrimSpace(d.Title) == "" {
		return ErrEmptyDraftTitle
	}
	if strings.TrimSpace(d.PlayURL) == "" {
		return ErrEmptyDraftPlayURL
	}
	if d.Status == DraftStatusPublished {
		return ErrDraftAlreadyPublished
	}
	return nil
}

func (d *Draft) MarkScheduled() {
	d.Status = DraftStatusScheduled
}

func (d *Draft) MarkPublished() {
	d.Status = DraftStatusPublished
}

func (d *Draft) MarkDraft() {
	d.Status = DraftStatusDraft
}

func (d *Draft) MarkDeleted() {
	d.Status = DraftStatusDeleted
}

// DraftCursor represents a keyset cursor for composite (updated_at DESC, id DESC) pagination.
type DraftCursor struct {
	UpdatedAt time.Time `json:"updated_at"`
	ID        int64     `json:"id"`
}

type draftCursorPayload struct {
	UpdatedAt string `json:"updated_at"`
	ID        int64  `json:"id"`
}

// Encode serializes the cursor into a URL-safe Base64 string.
func (c *DraftCursor) Encode() string {
	if c == nil || c.ID <= 0 {
		return ""
	}
	payload := draftCursorPayload{
		UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339Nano),
		ID:        c.ID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeDraftCursor decodes an opaque Base64 string or numeric ID into a DraftCursor.
func DecodeDraftCursor(raw string) (*DraftCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	// Support pure numeric cursor for backward compatibility
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
		return &DraftCursor{ID: id}, nil
	}

	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		data, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, ErrInvalidCursor
		}
	}

	var payload draftCursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, ErrInvalidCursor
	}
	if payload.ID <= 0 {
		return nil, ErrInvalidCursor
	}

	t, err := time.Parse(time.RFC3339Nano, payload.UpdatedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, payload.UpdatedAt)
		if err != nil {
			return nil, ErrInvalidCursor
		}
	}

	return &DraftCursor{
		UpdatedAt: t.UTC(),
		ID:        payload.ID,
	}, nil
}

// ScheduledTask represents an asynchronous scheduled video release.
type ScheduledTask struct {
	ID          int64           `json:"id"`
	TaskID      string          `json:"task_id"`
	AuthorID    int64           `json:"author_id"`
	DraftID     *int64          `json:"draft_id,omitempty"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	PlayURL     string          `json:"play_url"`
	CoverURL    string          `json:"cover_url"`
	DurationMs  int             `json:"duration_ms"`
	ScheduledAt time.Time       `json:"scheduled_at"`
	Status      ScheduledStatus `json:"status"`
	VideoID     *int64          `json:"video_id,omitempty"`
	ErrorMsg    *string         `json:"error_msg,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// StatusName returns a human-readable string of the scheduled task status.
func (t *ScheduledTask) StatusName() string {
	return t.Status.String()
}

// Validate checks scheduled task integrity and scheduled time window constraints.
func (t *ScheduledTask) Validate(now time.Time) error {
	if t.AuthorID <= 0 {
		return ErrInvalidAuthorID
	}
	if strings.TrimSpace(t.TaskID) == "" {
		return ErrInvalidTaskID
	}
	if strings.TrimSpace(t.Title) == "" {
		return ErrEmptyDraftTitle
	}
	if len(t.Title) > MaxDraftTitleLength {
		return ErrDraftTitleTooLong
	}
	if len(t.Description) > MaxDraftDescriptionLength {
		return ErrDraftDescTooLong
	}
	if strings.TrimSpace(t.PlayURL) == "" {
		return ErrEmptyDraftPlayURL
	}
	if len(t.PlayURL) > MaxDraftPlayURLLength {
		return ErrDraftPlayURLTooLong
	}
	if len(t.CoverURL) > MaxDraftCoverURLLength {
		return ErrDraftCoverURLTooLong
	}

	// Must be between now + 5 minutes and now + 30 days
	earliest := now.Add(MinScheduledLeadTime)
	latest := now.Add(MaxScheduledLeadTime)
	if t.ScheduledAt.Before(earliest) || t.ScheduledAt.After(latest) {
		return ErrInvalidScheduledTime
	}

	return nil
}

// CanCancel returns true if task is in cancelable state.
func (t *ScheduledTask) CanCancel() bool {
	return t.Status == ScheduledStatusScheduled
}

func (t *ScheduledTask) MarkCanceled() {
	t.Status = ScheduledStatusCanceled
}

func (t *ScheduledTask) MarkExecuting() {
	t.Status = ScheduledStatusExecuting
}

func (t *ScheduledTask) MarkPublished(videoID int64) {
	t.Status = ScheduledStatusPublished
	t.VideoID = &videoID
}

func (t *ScheduledTask) MarkFailed(errMsg string) {
	t.Status = ScheduledStatusFailed
	t.ErrorMsg = &errMsg
}
