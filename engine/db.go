package engine

// DB is the top-level database instance.
//
// In Phase 0, DB operates entirely in memory with no persistence, no WAL,
// no transactions, and no concurrency control. These will be added in
// subsequent phases behind this same interface.
type DB struct {
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

// Set stores a value for the given key.
// If the key already exists, its value is overwritten.
// Returns ErrKeyEmpty if the key is empty.
func (db *DB) Set(key Key, val Value) error {
	if key == "" {
		return ErrKeyEmpty
	}
	db.state.set(key, val)
	return nil
}

// Get retrieves the value for the given key.
// Returns ErrKeyNotFound if the key does not exist.
// Returns ErrKeyEmpty if the key is empty.
func (db *DB) Get(key Key) (Value, error) {
	if key == "" {
		return NewNull(), ErrKeyEmpty
	}
	return db.state.get(key)
}

// Delete removes a key and its value from the database.
// Returns ErrKeyNotFound if the key does not exist.
// Returns ErrKeyEmpty if the key is empty.
func (db *DB) Delete(key Key) error {
	if key == "" {
		return ErrKeyEmpty
	}
	return db.state.delete(key)
}

// Exists returns true if the key exists in the database.
// Returns false for empty keys without error.
func (db *DB) Exists(key Key) bool {
	if key == "" {
		return false
	}
	return db.state.exists(key)
}

// Len returns the number of keys in the database.
func (db *DB) Len() int {
	return db.state.len()
}
