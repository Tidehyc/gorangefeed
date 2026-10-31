package relation

import "errors"

var (
	ErrSelfFollow         = errors.New("cannot follow yourself")
	ErrSelfUnfollow       = errors.New("cannot unfollow yourself")
	ErrTargetUserNotFound = errors.New("target user not found")
	ErrInvalidFollowerID  = errors.New("invalid follower id")
	ErrInvalidVloggerID   = errors.New("invalid vlogger id")
)
