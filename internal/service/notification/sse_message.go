package notification

import (
	"bytes"
	"encoding/json"
	"strconv"

	domainnotification "github.com/Tidehyc/gorangefeed/internal/domain/notification"
)

// SSEMessage represents an individual Server-Sent Events transmission payload.
type SSEMessage struct {
	ID      string
	Event   string
	Data    string
	Comment string
	Retry   int
}

// Format serializes the message into RFC-compliant text/event-stream wire format.
func (m *SSEMessage) Format() []byte {
	if m == nil {
		return nil
	}

	var buf bytes.Buffer
	if m.Comment != "" {
		buf.WriteString(": " + m.Comment + "\n\n")
		return buf.Bytes()
	}

	if m.Retry > 0 {
		buf.WriteString("retry: " + strconv.Itoa(m.Retry) + "\n")
	}
	if m.ID != "" {
		buf.WriteString("id: " + m.ID + "\n")
	}
	if m.Event != "" {
		buf.WriteString("event: " + m.Event + "\n")
	}
	if m.Data != "" {
		buf.WriteString("data: " + m.Data + "\n")
	}
	buf.WriteString("\n")
	return buf.Bytes()
}

// NewNotificationSSEMessage creates a formatted event message from a Notification entity.
func NewNotificationSSEMessage(n *domainnotification.Notification) (*SSEMessage, error) {
	dataBytes, err := json.Marshal(n)
	if err != nil {
		return nil, err
	}

	return &SSEMessage{
		ID:    n.SSEEventID(),
		Event: "notification",
		Data:  string(dataBytes),
	}, nil
}

// NewKeepaliveSSEMessage creates a standard 15s keepalive comment line.
func NewKeepaliveSSEMessage() *SSEMessage {
	return &SSEMessage{
		Comment: "keepalive",
	}
}

// NewRestartingSSEMessage creates a protocol comment indicating the server is restarting.
func NewRestartingSSEMessage() *SSEMessage {
	return &SSEMessage{
		Comment: "server_restarting",
	}
}
