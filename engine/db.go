package engine

import "sync"

// DB is the top-level database instance.
//
// DB is safe for concurrent use. Reads (Get, Exists, Len, Keys) acquire a
// shared read lock. Writes (Set, Delete) acquire an exclusive write lock.
// The state itself is a pure data structure; all synchronization happens here.
type DB struct {
	mu    sync.RWMutex
	state *state
}

// Open creates and returns a new database instance.
//
// Currently this creates a purely in-memory database. In future phases,
// this will accept a path and handle persistence, WAL recovery, etc.
func Open() *DB {
	return &DB{
		state: newState(),
	}
}

// Close releases any resources held by the database.
//
// Currently a no-op for the in-memory engine. In future phases, this will
// flush WAL, write snapshots, and close file handles. Callers should always
// call Close when done with the database to ensure correct behavior as the
// engine evolves.
func (db *DB) Close() error {
	return nil
}

// Set stores a value for the given key.
// If the key already exists, its value is overwritten.
// Returns ErrKeyEmpty if the key is empty.
func (db *DB) Set(key Key, val Value) error {
	if key == "" {
		return ErrKeyEmpty
	}
	db.mu.Lock()
	db.state.set(key, val)
	db.mu.Unlock()
	return nil
}

// Get retrieves the value for the given key.
// Returns ErrKeyNotFound if the key does not exist.
// Returns ErrKeyEmpty if the key is empty.
func (db *DB) Get(key Key) (Value, error) {
	if key == "" {
		return NewNull(), ErrKeyEmpty
	}
	db.mu.RLock()
	val, err := db.state.get(key)
	db.mu.RUnlock()
	return val, err
}

// Delete removes a key and its value from the database.
// Returns ErrKeyNotFound if the key does not exist.
// Returns ErrKeyEmpty if the key is empty.
func (db *DB) Delete(key Key) error {
	if key == "" {
		return ErrKeyEmpty
	}
	db.mu.Lock()
	err := db.state.delete(key)
	db.mu.Unlock()
	return err
}

// Exists returns true if the key exists in the database.
// Returns false for empty keys without error.
func (db *DB) Exists(key Key) bool {
	if key == "" {
		return false
	}
	db.mu.RLock()
	ok := db.state.exists(key)
	db.mu.RUnlock()
	return ok
}

// Len returns the number of keys in the database.
func (db *DB) Len() int {
	db.mu.RLock()
	n := db.state.len()
	db.mu.RUnlock()
	return n
}

// Keys returns a slice of all keys in the database.
// The order of keys is not guaranteed.
// Returns an empty slice if the database is empty.
func (db *DB) Keys() []Key {
	db.mu.RLock()
	keys := db.state.keys()
	db.mu.RUnlock()
	return keys
}
