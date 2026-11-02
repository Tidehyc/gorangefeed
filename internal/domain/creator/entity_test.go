package creator

import (
	"strings"
	"testing"
	"time"
)

func TestDraft_Validation(t *testing.T) {
	validDraft := &Draft{
		AuthorID:    1,
		Title:       "Valid Title",
		Description: "A great description",
		PlayURL:     "http://example.com/play.mp4",
		CoverURL:    "http://example.com/cover.jpg",
		Status:      DraftStatusDraft,
	}

	t.Run("Valid draft for save and publish", func(t *testing.T) {
		if err := validDraft.ValidateForSave(); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if err := validDraft.ValidateForPublish(); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("Invalid author ID", func(t *testing.T) {
		d := *validDraft
		d.AuthorID = 0
		if err := d.ValidateForSave(); err != ErrInvalidAuthorID {
			t.Fatalf("expected ErrInvalidAuthorID, got %v", err)
		}
	})

	t.Run("Title too long", func(t *testing.T) {
		d := *validDraft
		d.Title = strings.Repeat("a", MaxDraftTitleLength+1)
		if err := d.ValidateForSave(); err != ErrDraftTitleTooLong {
			t.Fatalf("expected ErrDraftTitleTooLong, got %v", err)
		}
	})

	t.Run("Description too long", func(t *testing.T) {
		d := *validDraft
		d.Description = strings.Repeat("b", MaxDraftDescriptionLength+1)
		if err := d.ValidateForSave(); err != ErrDraftDescTooLong {
			t.Fatalf("expected ErrDraftDescTooLong, got %v", err)
		}
	})

	t.Run("Empty title when publishing", func(t *testing.T) {
		d := *validDraft
		d.Title = "   "
		if err := d.ValidateForPublish(); err != ErrEmptyDraftTitle {
			t.Fatalf("expected ErrEmptyDraftTitle, got %v", err)
		}
	})

	t.Run("Empty play url when publishing", func(t *testing.T) {
		d := *validDraft
		d.PlayURL = "   "
		if err := d.ValidateForPublish(); err != ErrEmptyDraftPlayURL {
			t.Fatalf("expected ErrEmptyDraftPlayURL, got %v", err)
		}
	})

	t.Run("Cannot publish already published draft", func(t *testing.T) {
		d := *validDraft
		d.Status = DraftStatusPublished
		if err := d.ValidateForPublish(); err != ErrDraftAlreadyPublished {
			t.Fatalf("expected ErrDraftAlreadyPublished, got %v", err)
		}
	})
}

func TestDraft_StateTransitions(t *testing.T) {
	d := &Draft{
		ID:       10,
		AuthorID: 100,
		Status:   DraftStatusDraft,
	}

	if d.StatusName() != "draft" {
		t.Fatalf("expected draft, got %s", d.StatusName())
	}

	d.MarkScheduled()
	if d.Status != DraftStatusScheduled || d.StatusName() != "scheduled" {
		t.Fatalf("expected scheduled, got %s", d.StatusName())
	}

	d.MarkPublished()
	if d.Status != DraftStatusPublished || d.StatusName() != "published" {
		t.Fatalf("expected published, got %s", d.StatusName())
	}

	d.MarkDraft()
	if d.Status != DraftStatusDraft {
		t.Fatalf("expected draft, got %s", d.StatusName())
	}

	d.MarkDeleted()
	if d.Status != DraftStatusDeleted || d.StatusName() != "deleted" {
		t.Fatalf("expected deleted, got %s", d.StatusName())
	}
}

func TestDraftCursor_EncodeDecode(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	cursor := &DraftCursor{
		UpdatedAt: now,
		ID:        42,
	}

	encoded := cursor.Encode()
	if encoded == "" {
		t.Fatal("expected non-empty encoded cursor")
	}

	decoded, err := DecodeDraftCursor(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.ID != 42 {
		t.Fatalf("expected ID 42, got %d", decoded.ID)
	}
	if !decoded.UpdatedAt.Equal(now) {
		t.Fatalf("expected UpdatedAt %v, got %v", now, decoded.UpdatedAt)
	}

	// Test backward compatibility with numeric ID cursor
	numDecoded, err := DecodeDraftCursor("123")
	if err != nil {
		t.Fatalf("numeric decode failed: %v", err)
	}
	if numDecoded.ID != 123 {
		t.Fatalf("expected ID 123, got %d", numDecoded.ID)
	}

	// Test empty cursor
	empty, err := DecodeDraftCursor("")
	if err != nil || empty != nil {
		t.Fatalf("expected nil for empty cursor, got %v, %v", empty, err)
	}

	// Test invalid cursor string
	_, err = DecodeDraftCursor("not-a-valid-cursor-string-@@")
	if err == nil {
		t.Fatal("expected error for invalid cursor, got nil")
	}
}

func TestScheduledTask_Validation(t *testing.T) {
	now := time.Now().UTC()
	validTask := &ScheduledTask{
		TaskID:      "task-uuid-1234",
		AuthorID:    1,
		Title:       "Valid Scheduled Title",
		Description: "Desc",
		PlayURL:     "http://example.com/play.mp4",
		CoverURL:    "http://example.com/cover.jpg",
		ScheduledAt: now.Add(10 * time.Minute),
		Status:      ScheduledStatusScheduled,
	}

	t.Run("Valid scheduled task", func(t *testing.T) {
		if err := validTask.Validate(now); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("Invalid lead time (too soon, under 5m)", func(t *testing.T) {
		task := *validTask
		task.ScheduledAt = now.Add(2 * time.Minute)
		if err := task.Validate(now); err != ErrInvalidScheduledTime {
			t.Fatalf("expected ErrInvalidScheduledTime, got %v", err)
		}
	})

	t.Run("Invalid lead time (in the past)", func(t *testing.T) {
		task := *validTask
		task.ScheduledAt = now.Add(-1 * time.Minute)
		if err := task.Validate(now); err != ErrInvalidScheduledTime {
			t.Fatalf("expected ErrInvalidScheduledTime, got %v", err)
		}
	})

	t.Run("Invalid lead time (too far, > 30 days)", func(t *testing.T) {
		task := *validTask
		task.ScheduledAt = now.Add(31 * 24 * time.Hour)
		if err := task.Validate(now); err != ErrInvalidScheduledTime {
			t.Fatalf("expected ErrInvalidScheduledTime, got %v", err)
		}
	})

	t.Run("Empty TaskID", func(t *testing.T) {
		task := *validTask
		task.TaskID = ""
		if err := task.Validate(now); err != ErrInvalidTaskID {
			t.Fatalf("expected ErrInvalidTaskID, got %v", err)
		}
	})

	t.Run("Task state transitions", func(t *testing.T) {
		task := *validTask
		if !task.CanCancel() {
			t.Fatal("expected CanCancel to be true")
		}

		task.MarkExecuting()
		if task.Status != ScheduledStatusExecuting || task.CanCancel() {
			t.Fatalf("expected executing and cannot cancel")
		}

		task.MarkPublished(999)
		if task.Status != ScheduledStatusPublished || *task.VideoID != 999 {
			t.Fatalf("expected published with videoID 999")
		}

		task.MarkFailed("transcoding error")
		if task.Status != ScheduledStatusFailed || *task.ErrorMsg != "transcoding error" {
			t.Fatalf("expected failed with error message")
		}

		task.Status = ScheduledStatusScheduled
		task.MarkCanceled()
		if task.Status != ScheduledStatusCanceled {
			t.Fatalf("expected canceled")
		}
	})
}
