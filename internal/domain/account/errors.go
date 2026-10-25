package account

import "errors"

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUsernameTaken      = errors.New("username already exists")
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserSuspended      = errors.New("user account is suspended")
	ErrUserDeleted        = errors.New("user account is deleted")
	ErrReplayDetected     = errors.New("replay attack detected: invalid refresh token")
)
