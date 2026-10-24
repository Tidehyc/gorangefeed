package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}

	for _, tc := range tests {
		if got := ParseLevel(tc.input); got != tc.expected {
			t.Errorf("ParseLevel(%q) = %v, expected %v", tc.input, got, tc.expected)
		}
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New("info", "json", &buf)

	l.Info("test json log", slog.String("key", "value"))

	output := buf.String()
	if !strings.Contains(output, `"msg":"test json log"`) {
		t.Errorf("expected json message, got: %s", output)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["key"] != "value" {
		t.Errorf("expected key=value, got %v", parsed["key"])
	}
}

func TestLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New("info", "text", &buf)

	l.Info("test text log", slog.String("service", "orange"))

	output := buf.String()
	if !strings.Contains(output, "msg=\"test text log\"") || !strings.Contains(output, "service=orange") {
		t.Errorf("expected text format log output, got: %s", output)
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := New("warn", "json", &buf)

	l.Debug("debug message")
	l.Info("info message")
	if buf.Len() != 0 {
		t.Errorf("expected no output for debug/info when level is warn, got: %s", buf.String())
	}

	l.Warn("warn message")
	if !strings.Contains(buf.String(), "warn message") {
		t.Errorf("expected warn message to be logged, got: %s", buf.String())
	}
}
