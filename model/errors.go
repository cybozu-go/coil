package model

import "errors"

var (
	// ErrConflicted is returned when storage operation conflicted.
	ErrConflicted = errors.New("conflicted")

	// ErrPoolExists is returned when a pool of the same name already exists.
	ErrPoolExists = errors.New("pool already exists")

	// ErrUsedSubnet is returned when the subnet is already in use.
	ErrUsedSubnet = errors.New("subnet in use")

	// ErrOutOfBlocks is returned when target freeList is empty.
	ErrOutOfBlocks = errors.New("out of address blocks")

	// ErrBlockIsFull is returned when all IP addresses in a block are allocated.
	ErrBlockIsFull = errors.New("block is full")

	// ErrNotFound is returned when a key does not exist.
	ErrNotFound = errors.New("not found")
)
