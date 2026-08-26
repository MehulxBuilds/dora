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
//
// For persistent databases, a write-ahead log (WAL) ensures durability.
// Every mutation is appended to the WAL before being applied to state.
// On Open, any existing WAL entries are replayed to recover state that
// was committed but not yet captured in a snapshot. On Close, the state
// is saved to storage and the WAL is truncated.
type DB struct {
	mu      sync.RWMutex
	state   *state
	storage Storage
	wal     *wal // nil for in-memory databases
}

// Open creates and returns a new database instance.
//
// If path is empty, the database operates purely in memory with no persistence
// and no WAL.
//
// If path is non-empty, it is treated as a file path for JSON persistence:
//   - If the file exists, the state is loaded from it.
//   - If the file does not exist, a new empty database is created.
//   - A WAL file (<path>.wal) is opened for durability.
//   - Any existing WAL entries are replayed to recover committed state.
//   - On Close(), the state is saved and the WAL is truncated.
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

	s := newStateFrom(data)

	// For persistent databases, set up the WAL.
	var w *wal
	if path != "" {
		walPath := path + ".wal"

		// Replay any existing WAL entries to recover state committed
		// after the last snapshot. This handles the case where the process
		// crashed between WAL writes and the next Close/snapshot.
		ops, err := loadWALEntries(walPath)
		if err != nil {
			return nil, err
		}
		for _, op := range ops {
			// Errors during replay are ignored. A DELETE of a non-existent
			// key can occur if the snapshot already captured the state after
			// that delete (crash between snapshot save and WAL truncate).
			// SET operations never fail. The end result is always correct.
			_ = s.apply(op)
		}

		w, err = openWAL(walPath)
		if err != nil {
			return nil, err
		}
	}

	return &DB{
		state:   s,
		storage: store,
		wal:     w,
	}, nil
}

// Close saves the current state to storage and releases resources.
//
// For persistent databases, Close performs:
//  1. Snapshot the in-memory state (under lock).
//  2. Save the snapshot to storage.
//  3. Truncate the WAL (all entries are now in the snapshot).
//  4. Close the WAL and storage file handles.
//
// Always call Close when done with the database. For persistent databases,
// failing to call Close means the final snapshot is not written (though
// individual operations are still durable in the WAL).
func (db *DB) Close() error {
	db.mu.Lock()
	snap := db.state.snapshot()
	db.mu.Unlock()

	// Save snapshot first. If this succeeds, the snapshot is the new
	// authoritative state and the WAL can be safely discarded.
	if err := db.storage.Save(snap); err != nil {
		return err
	}

	// Truncate and close the WAL after successful snapshot save.
	if db.wal != nil {
		if err := db.wal.truncate(); err != nil {
			return err
		}
		if err := db.wal.close(); err != nil {
			return err
		}
	}

	return db.storage.Close()
}

// Set stores a value for the given key.
// If the key already exists, its value is overwritten.
// Returns ErrKeyEmpty if the key is empty.
//
// The mutation flow is:
//  1. Validate the key.
//  2. Create an OpSet operation.
//  3. Append the operation to the WAL (persistent databases only).
//  4. Apply the operation to the in-memory state.
//
// After step 3, the operation is durable on disk. If the process crashes
// after step 3 but before step 4, recovery will replay it from the WAL.
func (db *DB) Set(key Key, val Value) error {
	if key == "" {
		return ErrKeyEmpty
	}
	op := Operation{Type: OpSet, Key: key, Value: val}
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.wal != nil {
		if err := db.wal.append(op); err != nil {
			return err
		}
	}
	return db.state.apply(op)
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
//
// The mutation flow is:
//  1. Validate the key.
//  2. Verify the key exists (under write lock, before WAL write).
//  3. Create an OpDelete operation.
//  4. Append the operation to the WAL (persistent databases only).
//  5. Apply the operation to the in-memory state.
//
// The existence check in step 2 ensures only valid operations reach the WAL.
// This prevents WAL entries for deleting non-existent keys, which simplifies
// recovery reasoning.
func (db *DB) Delete(key Key) error {
	if key == "" {
		return ErrKeyEmpty
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if !db.state.exists(key) {
		return ErrKeyNotFound
	}
	op := Operation{Type: OpDelete, Key: key}
	if db.wal != nil {
		if err := db.wal.append(op); err != nil {
			return err
		}
	}
	return db.state.apply(op)
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
