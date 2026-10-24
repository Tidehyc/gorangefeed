package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestResponse_Success(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	testData := map[string]string{"foo": "bar"}
	OK(c, testData)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("expected error to be nil, got %+v", resp.Error)
	}

	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok || dataMap["foo"] != "bar" {
		t.Fatalf("unexpected data: %+v", resp.Data)
	}
}

func TestResponse_KnownAppError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid param", ErrInvalidParam("missing query"), http.StatusBadRequest, CodeInvalidParam},
		{"unauthorized", ErrUnauthorized("token expired"), http.StatusUnauthorized, CodeUnauthorized},
		{"forbidden", ErrForbidden("no permission"), http.StatusForbidden, CodeForbidden},
		{"not found", ErrNotFound("video not found"), http.StatusNotFound, CodeNotFound},
		{"conflict", ErrConflict("duplicate request"), http.StatusConflict, CodeConflict},
		{"rate limited", ErrRateLimited("slow down"), http.StatusTooManyRequests, CodeRateLimited},
		{"internal app error", ErrInternal("db error"), http.StatusInternalServerError, CodeInternal},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			Fail(c, tc.err)

			if w.Code != tc.wantStatus {
				t.Fatalf("expected HTTP %d, got %d", tc.wantStatus, w.Code)
			}

			var resp Response
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal JSON: %v", err)
			}

			if resp.Data != nil {
				t.Fatalf("expected data to be nil, got %+v", resp.Data)
			}

			if resp.Error == nil || resp.Error.Code != tc.wantCode {
				t.Fatalf("expected error code %q, got %+v", tc.wantCode, resp.Error)
			}
		})
	}
}

func TestResponse_UnrecognizedErrorFallbackToInternal(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	genericErr := errors.New("something unexpected happened in database query")
	Fail(c, genericErr)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected HTTP 500, got %d", w.Code)
	}

	var resp Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if resp.Data != nil {
		t.Fatalf("expected data to be nil, got %+v", resp.Data)
	}

	if resp.Error == nil || resp.Error.Code != CodeInternal {
		t.Fatalf("expected fallback error code %q, got %+v", CodeInternal, resp.Error)
	}
}
