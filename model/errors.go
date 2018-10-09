package model

import "errors"

var (
	// ErrConflicted is returned when storage operation conflicted.
	ErrConflicted = errors.New("conflicted")

	// ErrPoolExists is returned when a pool of the same name already exists.
	ErrPoolExists = errors.New("pool already exists")

	// ErrUsedSubnet is returned when the subnet is already in use.
	ErrUsedSubnet = errors.New("subnet in use")

	// ErrFullBlock is returned when target freeList is full
	ErrFullBlock = errors.New("block is full")

	// ErrNotFound is returned when a key does not exist.
	ErrNotFound = errors.New("not found")
)
