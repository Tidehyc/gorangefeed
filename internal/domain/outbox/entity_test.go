package outbox_test

import (
	"encoding/json"
	"testing"

	"github.com/Tidehyc/gorangefeed/internal/domain/outbox"
)

func TestNewEvent(t *testing.T) {
	t.Run("valid payload struct", func(t *testing.T) {
		payload := map[string]any{"video_id": 123, "author_id": 456}
		evt, err := outbox.NewEvent(outbox.EventTypeVideoPublished, payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if evt.EventID == "" {
			t.Errorf("expected non-empty EventID")
		}
		if evt.Status != outbox.StatusPending {
			t.Errorf("expected StatusPending, got %s", evt.Status)
		}
		if evt.EventType != outbox.EventTypeVideoPublished {
			t.Errorf("expected event type %s, got %s", outbox.EventTypeVideoPublished, evt.EventType)
		}

		var m map[string]any
		if err := json.Unmarshal(evt.Payload, &m); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		if int(m["video_id"].(float64)) != 123 {
			t.Errorf("expected video_id 123, got %v", m["video_id"])
		}
	})

	t.Run("empty event type", func(t *testing.T) {
		_, err := outbox.NewEvent("", "payload")
		if err == nil {
			t.Errorf("expected error for empty event type")
		}
	})
}
