package model

import "errors"

var (
	// ErrConflicted is returned when storage operation conflicted.
	ErrConflicted = errors.New("conflicted")
)
