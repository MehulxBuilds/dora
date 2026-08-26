package engine

import "sync"

// DB is the top-level database instance.
//
// DB is safe for concurrent use. Reads (Get, Exists, Len, Keys) acquire a
// shared read lock. Writes (Set, Delete) acquire an exclusive write lock.
// The state itself is a pure data structure; all synchronization happens here.
//
// Persistence is managed through the Storage interface. On Open, the state
// is loaded from storage. On Close, the state is saved back to storage.
type DB struct {
	mu      sync.RWMutex
	state   *state
	storage Storage
}

// Open creates and returns a new database instance.
//
// If path is empty, the database operates purely in memory with no persistence.
// If path is non-empty, it is treated as a file path for JSON persistence:
//   - If the file exists, the state is loaded from it.
//   - If the file does not exist, a new empty database is created.
//   - On Close(), the state is saved to the file.
func Open(path string) (*DB, error) {
	var store Storage
	if path == "" {
		store = &memoryStorage{}
	} else {
		store = newJSONStorage(path)
	}

	data, err := store.Load()
	if err != nil {
		return nil, err
	}

	return &DB{
		state:   newStateFrom(data),
		storage: store,
	}, nil
}

// Close saves the current state to storage and releases resources.
//
// Always call Close when done with the database. For persistent databases,
// this writes the final state to disk. Failing to call Close on a persistent
// database means unsaved mutations are lost.
func (db *DB) Close() error {
	db.mu.Lock()
	snap := db.state.snapshot()
	db.mu.Unlock()

	if err := db.storage.Save(snap); err != nil {
		return err
	}
	return db.storage.Close()
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
