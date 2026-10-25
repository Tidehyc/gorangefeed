package account

import (
	"testing"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		username string
		wantErr  bool
	}{
		{"alice", false},
		{"alice_123", false},
		{"User_999", false},
		{"ab", true},                                         // too short
		{"a_very_long_username_that_exceeds_32_chars", true}, // too long
		{"alice with space", true},                           // space
		{"alice@domain", true},                               // special char
		{"", true},                                           // empty
	}

	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUsername(%q) error = %v, wantErr %v", tt.username, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid standard", "Password123", false},
		{"valid with symbols", "P@ssw0rd!#$", false},
		{"too short", "Pass1", true},
		{"too long", "A123456789012345678901234567890123", true},
		{"letters only", "PasswordOnly", true},
		{"numbers only", "1234567890", true},
		{"with spaces", "Pass word 123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestHashAndCheckPassword(t *testing.T) {
	plain := "SecurePass123"
	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == plain {
		t.Fatalf("hash should not equal plain text")
	}

	if !CheckPassword(plain, hash) {
		t.Fatalf("CheckPassword failed for correct password")
	}

	if CheckPassword("WrongPassword123", hash) {
		t.Fatalf("CheckPassword succeeded for wrong password")
	}
}
