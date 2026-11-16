package engine

import "errors"

// Sentinel errors for the database engine.
// These are the canonical error values that callers should check against
// using errors.Is.
var (
	// ErrIndexConflict indicates EnsureIndex found a different definition with the same name.
	ErrIndexConflict = errors.New("index definition conflicts with existing index")
	ErrIndexExists   = errors.New("index already exists")
	ErrIndexNotFound = errors.New("index not found")
	ErrIndexInvalid  = errors.New("index name and field must not be empty")
	ErrIndexValue    = errors.New("index lookup requires a finite scalar value")

	// ErrReferenceNotFound indicates a reference to an absent or empty target.
	ErrReferenceNotFound = errors.New("reference target not found")
	// ErrKeyReferenced indicates that deleting a key would leave a dangling reference.
	ErrKeyReferenced = errors.New("key is referenced")

	// ErrKeyNotFound is returned when a requested key does not exist in the keyspace.
	ErrKeyNotFound = errors.New("key not found")

	// ErrKeyEmpty is returned when an operation is attempted with an empty key.
	ErrKeyEmpty = errors.New("key must not be empty")

	// ErrClosed is returned when an operation is attempted on a closed database.
	ErrClosed = errors.New("database is closed")
)
