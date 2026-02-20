package auth

import "errors"

var (
	ErrKeyNotFound     = errors.New("virtual key not found")
	ErrKeyInactive     = errors.New("virtual key is inactive")
	ErrKeyExpired      = errors.New("virtual key has expired")
	ErrModelBlocked    = errors.New("model is blocked for this key")
	ErrModelNotAllowed = errors.New("model is not in the allowed list for this key")
)
