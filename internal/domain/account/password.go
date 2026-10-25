package account

import (
	"errors"
	"regexp"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUsernameInvalid = errors.New("username must be 3-32 characters long and contain only letters, numbers, and underscores")
	ErrPasswordTooShort = errors.New("password must be at least 8 characters long")
	ErrPasswordTooLong  = errors.New("password cannot exceed 32 characters")
	ErrPasswordSimple   = errors.New("password must contain both letters and numbers")
	ErrPasswordSpace    = errors.New("password must not contain spaces")

	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
)

// ValidateUsername checks if the username meets format requirements.
func ValidateUsername(username string) error {
	if !usernameRegex.MatchString(username) {
		return ErrUsernameInvalid
	}
	return nil
}

// ValidatePassword validates password complexity:
// - Length between 8 and 32 characters
// - At least one letter and at least one number
// - No whitespace characters
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrPasswordTooShort
	}
	if len(password) > 32 {
		return ErrPasswordTooLong
	}

	var hasLetter, hasDigit bool
	for _, r := range password {
		if unicode.IsSpace(r) {
			return ErrPasswordSpace
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}

	if !hasLetter || !hasDigit {
		return ErrPasswordSimple
	}

	return nil
}

// HashPassword hashes a plain text password using bcrypt.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword compares a bcrypt hashed password with its possible plaintext equivalent.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
