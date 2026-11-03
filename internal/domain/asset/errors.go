package asset

import "errors"

var (
	ErrInvalidUserID             = errors.New("invalid user id")
	ErrInvalidVideoID            = errors.New("invalid video id")
	ErrInvalidFolderID           = errors.New("invalid folder id")
	ErrEmptyFolderName           = errors.New("folder name cannot be empty")
	ErrFolderNameTooLong         = errors.New("folder name exceeds max length (64 characters)")
	ErrFolderDescTooLong         = errors.New("folder description exceeds max length (255 characters)")
	ErrFolderNotFound            = errors.New("favorite folder not found")
	ErrCannotDeleteDefaultFolder = errors.New("cannot delete default favorite folder")
	ErrFolderAccessDenied        = errors.New("permission denied to access favorite folder")
	ErrAlreadyFavorited          = errors.New("video already favorited in this folder")
	ErrFavoriteItemNotFound      = errors.New("favorite item not found")
	ErrInvalidRetentionLimit     = errors.New("retention limit must be positive")
	ErrInvalidPlaybackPosition   = errors.New("invalid playback position")
	ErrInvalidDuration           = errors.New("invalid video duration")
)
