package engine

import (
	"os"
	"path/filepath"
	"sync"
)

// metaSeqKey is a reserved key used to persist the sequence number in
// snapshot files. It uses a NUL-byte prefix to prevent collision with
// user keys (impractical to type, and any legitimate key starting with
// \x00 would be highly unusual).
const metaSeqKey = "\x00_seq"

// dataDir is the directory where all database files are stored.
// This is a branding convention — like prisma/ or drizzle/, all Dora
// database files live under dora/ relative to the working directory.
const dataDir = "dora"

// DB is the top-level database instance.
//
// DB is safe for concurrent use. Reads (Get, Exists, Len, Keys) acquire a
// shared read lock. Writes (Set, Delete, Transaction) acquire an exclusive
// write lock. The state itself is a pure data structure; all synchronization
// happens here at the DB level.
//
// Persistence is managed through the Storage interface. On Open, the state
// is loaded from storage. On Close, the state is saved back to storage.
//
// For persistent databases, a write-ahead log (WAL) ensures durability.
// Every mutation is appended to the WAL before being applied to state.
// On Open, any existing WAL entries are replayed to recover state that
// was committed but not yet captured in a snapshot. On Close, the state
// is saved to storage and the WAL is truncated.
//
// Concurrency model (Phase 7):
//
//   - Single sync.RWMutex guards all state and WAL access.
//   - Reads: RLock (concurrent reads allowed).
//   - Writes: Lock (exclusive, serialized writes).
//   - Transactions: Lock held for entire duration (serialized).
//   - Close: Lock held for entire operation (snapshot + WAL + storage).
//   - After Close(), all methods return ErrClosed (or zero values for
//     non-error-returning methods).
type DB struct {
	mu      sync.RWMutex
	state   *state
	storage Storage
	wal     *wal // nil for in-memory databases
	closed  bool
}

// Open creates and returns a new database instance.
//
// If path is empty, the database operates purely in memory with no persistence
// and no WAL.
//
// If path is non-empty, it is treated as a filename for JSON persistence.
// The file is automatically placed inside the dora/ directory:
//
//	Open("app.db")  →  dora/app.db  +  dora/app.db.wal
//	Open("")        →  in-memory (no files)
//
// The dora/ directory is created automatically if it does not exist.
func Open(path string) (*DB, error) {
	var store Storage
	if path == "" {
		store = &memoryStorage{}
	} else {
		// Place all database files under the dora/ directory.
		path = filepath.Join(dataDir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		store = newJSONStorage(path)
	}

	data, err := store.Load()
	if err != nil {
		return nil, err
	}

	// Extract the persisted sequence number from the snapshot data.
	// It is stored under a reserved key that is removed before creating
	// the state, so it never appears in the user-visible keyspace.
	var seq uint64
	if v, ok := data[metaSeqKey]; ok {
		if n, nok := v.Number(); nok {
			seq = uint64(n)
		}
		delete(data, metaSeqKey)
	}

	indexMeta, hasIndexes := data[metaIndexesKey]
	delete(data, metaIndexesKey)
	s := newStateFrom(data)
	s.seq = seq
	if hasIndexes {
		if err := s.loadIndexes(indexMeta); err != nil {
			return nil, err
		}
	}

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

		if err := validateReferences(s.data, nil); err != nil {
			return nil, err
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
// For persistent databases, Close performs (all under exclusive lock):
//  1. Checkpoint: save snapshot to storage, truncate WAL.
//  2. Close the WAL file handle.
//  3. Close the storage file handle.
//  4. Mark the database as closed.
//
// After Close returns, all subsequent method calls return ErrClosed
// (or zero values for non-error-returning methods like Exists, Len, Keys).
//
// Close is safe to call concurrently with other operations — it acquires
// the exclusive write lock first, ensuring all in-flight reads and writes
// complete before shutdown begins.
//
// Calling Close on an already-closed database returns ErrClosed.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrClosed
	}

	if err := db.checkpoint(); err != nil {
		return err
	}

	if db.wal != nil {
		if err := db.wal.close(); err != nil {
			return err
		}
	}

	if err := db.storage.Close(); err != nil {
		return err
	}

	db.closed = true
	return nil
}

// Snapshot saves the current in-memory state to storage and truncates
// the WAL, without closing the database.
//
// This is the primary mechanism for controlling WAL size and recovery
// time in long-running databases. After a successful Snapshot, the WAL
// is empty and recovery only needs the snapshot file.
//
// Snapshot acquires the exclusive write lock for its duration. No reads
// or writes can proceed while the snapshot is being written. Keep this
// in mind for latency-sensitive workloads.
//
// For in-memory databases (no storage file), Snapshot is a no-op.
//
// Returns ErrClosed if the database has been closed.
func (db *DB) Snapshot() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrClosed
	}

	return db.checkpoint()
}

// checkpoint saves the current state to storage and truncates the WAL.
//
// This is the internal implementation shared by Close() and Snapshot().
// It must be called with db.mu held (exclusive write lock).
//
// The sequence is:
//  1. Copy the in-memory state (snapshot).
//  2. Inject the current sequence number into the snapshot under a
//     reserved key, so it is persisted alongside the data.
//  3. Write the snapshot to storage (atomic file write).
//  4. Truncate the WAL (all entries are now captured in the snapshot).
//
// If step 3 fails, the WAL is NOT truncated — the entries remain for
// the next recovery attempt. If step 4 fails, the snapshot is already
// saved — the next Open will replay WAL entries that are already in the
// snapshot (idempotent, harmless).
func (db *DB) checkpoint() error {
	snap := db.state.snapshot()
	snap[metaSeqKey] = NewNumber(float64(db.state.seq))
	if len(db.state.indexes) > 0 {
		snap[metaIndexesKey] = db.state.indexSnapshot()
	}

	if err := db.storage.Save(snap); err != nil {
		return err
	}

	if db.wal != nil {
		if err := db.wal.truncate(); err != nil {
			return err
		}
	}

	return nil
}

// Set stores a value for the given key.
// If the key already exists, its value is overwritten.
// Returns ErrKeyEmpty if the key is empty.
// Returns ErrClosed if the database has been closed.
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
	if db.closed {
		return ErrClosed
	}
	if err := db.validateChanges(map[Key]txOverlay{key: {value: val}}); err != nil {
		return err
	}
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
// Returns ErrClosed if the database has been closed.
func (db *DB) Get(key Key) (Value, error) {
	if key == "" {
		return NewNull(), ErrKeyEmpty
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return NewNull(), ErrClosed
	}
	return db.state.get(key)
}

// Delete removes a key and its value from the database.
// Returns ErrKeyNotFound if the key does not exist.
// Returns ErrKeyEmpty if the key is empty.
// Returns ErrClosed if the database has been closed.
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
	if db.closed {
		return ErrClosed
	}
	if !db.state.exists(key) {
		return ErrKeyNotFound
	}
	op := Operation{Type: OpDelete, Key: key}
	if err := db.validateChanges(map[Key]txOverlay{key: {deleted: true}}); err != nil {
		return err
	}
	if db.wal != nil {
		if err := db.wal.append(op); err != nil {
			return err
		}
	}
	return db.state.apply(op)
}

// Exists returns true if the key exists in the database.
// Returns false for empty keys without error.
// Returns false if the database has been closed.
func (db *DB) Exists(key Key) bool {
	if key == "" {
		return false
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return false
	}
	return db.state.exists(key)
}

// Len returns the number of keys in the database.
// Returns 0 if the database has been closed.
func (db *DB) Len() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return 0
	}
	return db.state.len()
}

// Keys returns a slice of keys in the database.
//
// If called with no arguments, returns all keys.
// If called with a prefix string, returns only keys that start with
// that prefix:
//
//	db.Keys()          // all keys
//	db.Keys("user:")   // ["user:1", "user:2", ...]
//
// The order of keys is not guaranteed.
// Returns nil if the database has been closed.
func (db *DB) Keys(prefix ...string) []Key {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil
	}
	if len(prefix) > 0 && prefix[0] != "" {
		return db.state.keysWithPrefix(prefix[0])
	}
	return db.state.keys()
}

// Prefix returns all key-value pairs where the key starts with the
// given prefix.
//
//	entries := db.Prefix("user:")
//	// entries = map[Key]Value{
//	//     "user:1": ...,
//	//     "user:2": ...,
//	// }
//
// An empty prefix returns all key-value pairs (equivalent to a full
// state copy).
//
// The returned map is a new map — it is safe to read and iterate
// without holding any lock. Modifications to the returned map do not
// affect the database.
//
// Returns nil if the database has been closed.
func (db *DB) Prefix(prefix string) map[Key]Value {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil
	}
	return db.state.prefix(prefix)
}

// Seq returns the current operation sequence number.
//
// The sequence number is a monotonically increasing counter that is
// incremented on every successful state mutation (Set, Delete, or each
// operation within a committed transaction). It serves as both an
// operation ID and a position in the operation history.
//
// Returns 0 if the database has been closed.
func (db *DB) Seq() uint64 {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return 0
	}
	return db.state.seq
}

// StateHash computes a deterministic SHA-256 hash of the current state.
//
// The hash covers only the key-value data — it does NOT include the
// sequence number. Two databases with identical key-value pairs produce
// the same hash, regardless of operation history.
//
// This is the primary mechanism for deterministic verification:
//
//	same initial state + same operation sequence = same state hash
//
// The hash is computed by sorting all keys lexicographically, then
// feeding length-prefixed key bytes and JSON-encoded value bytes into
// SHA-256. The encoding is unambiguous and deterministic.
//
// Returns ErrClosed if the database has been closed.
func (db *DB) StateHash() ([32]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return [32]byte{}, ErrClosed
	}
	return db.state.hash()
}
