package account

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

// loginForToken registers a user through the HTTP layer and returns a usable access token.
func loginForToken(t *testing.T, r *gin.Engine, username, password string) string {
	t.Helper()

	body, _ := json.Marshal(RegisterRequest{Username: username, Password: password})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("register returned %d: %s", w.Code, w.Body.String())
	}

	body, _ = json.Marshal(LoginRequest{Username: username, Password: password})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login returned %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data LoginResponse `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Tokens.AccessToken == "" {
		t.Fatalf("login response carried no access token: %s", w.Body.String())
	}
	return resp.Data.Tokens.AccessToken
}

func doJSON(t *testing.T, r *gin.Engine, method, path, token string, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if payload == nil {
		reader = bytes.NewReader(nil)
	} else {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		reader = bytes.NewReader(body)
	}

	req, _ := http.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_ChangePassword(t *testing.T) {
	h, _, jwtMgr, _, tempDir := setupTestHandler(t)
	defer os.RemoveAll(tempDir)
	r := setupTestEngine(h, jwtMgr)

	token := loginForToken(t, r, "pwd_user", "Password123")

	// 1. Unauthenticated -> 401
	w := doJSON(t, r, http.MethodPut, "/api/v1/users/me/password", "",
		ChangePasswordRequest{OldPassword: "Password123", NewPassword: "NewPassword456"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a token, got %d", w.Code)
	}

	// 2. Missing field -> 400
	w = doJSON(t, r, http.MethodPut, "/api/v1/users/me/password", token,
		ChangePasswordRequest{OldPassword: "Password123"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when new_password is missing, got %d", w.Code)
	}

	// 3. Wrong current password -> 401
	w = doJSON(t, r, http.MethodPut, "/api/v1/users/me/password", token,
		ChangePasswordRequest{OldPassword: "Nope12345678", NewPassword: "NewPassword456"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a wrong current password, got %d: %s", w.Code, w.Body.String())
	}

	// 4. Valid change -> 200, and the old credential stops working
	w = doJSON(t, r, http.MethodPut, "/api/v1/users/me/password", token,
		ChangePasswordRequest{OldPassword: "Password123", NewPassword: "NewPassword456"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for a valid change, got %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, r, http.MethodPost, "/api/v1/auth/login", "",
		LoginRequest{Username: "pwd_user", Password: "Password123"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("old password should be rejected after the change, got %d", w.Code)
	}
	w = doJSON(t, r, http.MethodPost, "/api/v1/auth/login", "",
		LoginRequest{Username: "pwd_user", Password: "NewPassword456"})
	if w.Code != http.StatusOK {
		t.Fatalf("new password should be accepted after the change, got %d", w.Code)
	}
}

func TestHandler_DeleteAccount(t *testing.T) {
	h, _, jwtMgr, _, tempDir := setupTestHandler(t)
	defer os.RemoveAll(tempDir)
	r := setupTestEngine(h, jwtMgr)

	token := loginForToken(t, r, "del_user", "Password123")

	// 1. Missing password -> 400
	w := doJSON(t, r, http.MethodDelete, "/api/v1/users/me", token, DeleteAccountRequest{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without a password, got %d", w.Code)
	}

	// 2. Wrong password -> 401 and the account survives
	w = doJSON(t, r, http.MethodDelete, "/api/v1/users/me", token, DeleteAccountRequest{Password: "Nope12345678"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a wrong password, got %d", w.Code)
	}
	w = doJSON(t, r, http.MethodPost, "/api/v1/auth/login", "",
		LoginRequest{Username: "del_user", Password: "Password123"})
	if w.Code != http.StatusOK {
		t.Fatalf("account should still be usable after a rejected deletion, got %d", w.Code)
	}

	// 3. Valid deletion -> 200 and login is refused afterwards
	w = doJSON(t, r, http.MethodDelete, "/api/v1/users/me", token, DeleteAccountRequest{Password: "Password123"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for a valid deletion, got %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, r, http.MethodPost, "/api/v1/auth/login", "",
		LoginRequest{Username: "del_user", Password: "Password123"})
	if w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
		t.Fatalf("deleted account must not log in, got %d", w.Code)
	}
}
