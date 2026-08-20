package engine

import "errors"

// Sentinel errors for the database engine.
// These are the canonical error values that callers should check against
// using errors.Is.
var (
	// ErrKeyNotFound is returned when a requested key does not exist in the keyspace.
	ErrKeyNotFound = errors.New("key not found")

	// ErrKeyEmpty is returned when an operation is attempted with an empty key.
	ErrKeyEmpty = errors.New("key must not be empty")
)
