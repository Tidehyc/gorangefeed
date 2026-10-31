package interaction_test

import (
	"strings"
	"testing"

	"github.com/Tidehyc/gorangefeed/internal/domain/interaction"
)

func TestComment_Validate(t *testing.T) {
	validComment := func() *interaction.Comment {
		return &interaction.Comment{
			VideoID:        1,
			UserID:         2,
			ParentID:       0,
			Content:        "Hello world",
			IdempotencyKey: "unique-key-123",
			Status:         interaction.CommentStatusPublished,
		}
	}

	t.Run("valid comment", func(t *testing.T) {
		c := validComment()
		if err := c.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if !c.IsPublished() {
			t.Errorf("expected published to be true")
		}
	})

	t.Run("invalid user id", func(t *testing.T) {
		c := validComment()
		c.UserID = 0
		if err := c.Validate(); err != interaction.ErrInvalidUserID {
			t.Errorf("expected ErrInvalidUserID, got %v", err)
		}
	})

	t.Run("invalid video id", func(t *testing.T) {
		c := validComment()
		c.VideoID = 0
		if err := c.Validate(); err != interaction.ErrInvalidVideoID {
			t.Errorf("expected ErrInvalidVideoID, got %v", err)
		}
	})

	t.Run("empty content", func(t *testing.T) {
		c := validComment()
		c.Content = "   "
		if err := c.Validate(); err != interaction.ErrEmptyCommentContent {
			t.Errorf("expected ErrEmptyCommentContent, got %v", err)
		}
	})

	t.Run("content too long", func(t *testing.T) {
		c := validComment()
		c.Content = strings.Repeat("字", 501)
		if err := c.Validate(); err != interaction.ErrCommentContentTooLong {
			t.Errorf("expected ErrCommentContentTooLong, got %v", err)
		}
	})

	t.Run("empty idempotency key", func(t *testing.T) {
		c := validComment()
		c.IdempotencyKey = ""
		if err := c.Validate(); err != interaction.ErrEmptyIdempotencyKey {
			t.Errorf("expected ErrEmptyIdempotencyKey, got %v", err)
		}
	})

	t.Run("invalid parent id", func(t *testing.T) {
		c := validComment()
		c.ParentID = -1
		if err := c.Validate(); err != interaction.ErrInvalidParentComment {
			t.Errorf("expected ErrInvalidParentComment, got %v", err)
		}
	})
}
