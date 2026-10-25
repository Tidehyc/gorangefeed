package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ParseJSONResponse unmarshals the response body into target or fails the test.
func ParseJSONResponse(t *testing.T, w *httptest.ResponseRecorder, target interface{}) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
		t.Fatalf("failed to parse JSON response: %v, raw body: %s", err, w.Body.String())
	}
}

// NewJSONRequest constructs an HTTP request with JSON payload and optional client IP.
func NewJSONRequest(method, path string, body interface{}, ip string) *http.Request {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if ip != "" {
		req.RemoteAddr = ip + ":54321"
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}
