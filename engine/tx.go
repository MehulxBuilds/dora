package engine

// Tx represents an in-progress database transaction.
//
// A transaction buffers all mutations (Set, Delete) in an overlay and only
// applies them to the database state on commit. Reads (Get, Exists) see the
// transaction's own uncommitted changes layered on top of the base state.
//
// Usage:
//
//	db.Transaction(func(tx *Tx) error {
//	    tx.Set("user:1", user)
//	    tx.Set("post:1", post)
//	    return nil // commit
//	})
//
// If the function returns nil, all buffered operations are atomically
// committed to the WAL and state. If it returns an error, all changes
// are discarded (rollback).
//
// IMPORTANT: Do NOT call DB.Set, DB.Get, DB.Delete, or any other DB
// method from within the transaction function. The transaction holds
// an exclusive write lock — calling DB methods will deadlock. Use the
// Tx methods (tx.Set, tx.Get, tx.Delete, tx.Exists) instead.
type Tx struct {
	db      *DB
	overlay map[Key]txOverlay
	ops     []Operation
}

// txOverlay tracks the transaction-local state of a single key.
//
// If deleted is true, the key has been deleted in this transaction
// (regardless of whether it exists in the base state).
// If deleted is false, value holds the key's current value in this
// transaction.
type txOverlay struct {
	value   Value
	deleted bool
}

// Set stores a value for the given key within the transaction.
//
// The change is buffered and not visible outside the transaction until
// commit. Subsequent tx.Get calls for the same key will return the new
// value immediately.
//
// Returns ErrKeyEmpty if the key is empty.
func (tx *Tx) Set(key Key, val Value) error {
	if key == "" {
		return ErrKeyEmpty
	}
	tx.overlay[key] = txOverlay{value: val, deleted: false}
	tx.ops = append(tx.ops, Operation{Type: OpSet, Key: key, Value: val})
	return nil
}

// Get retrieves the value for the given key from the transaction's
// perspective.
//
// Lookup order:
//  1. Transaction overlay (uncommitted changes from this transaction).
//  2. Base database state (committed data).
//
// Returns ErrKeyNotFound if the key does not exist in either layer,
// or if it was deleted within this transaction.
// Returns ErrKeyEmpty if the key is empty.
func (tx *Tx) Get(key Key) (Value, error) {
	if key == "" {
		return NewNull(), ErrKeyEmpty
	}
	if entry, ok := tx.overlay[key]; ok {
		if entry.deleted {
			return NewNull(), ErrKeyNotFound
		}
		return entry.value, nil
	}
	return tx.db.state.get(key)
}

// Delete marks a key for deletion within the transaction.
//
// The key must exist from the transaction's perspective (either in the
// overlay as a live value, or in the base state and not previously
// deleted in this transaction).
//
// Returns ErrKeyNotFound if the key does not exist.
// Returns ErrKeyEmpty if the key is empty.
func (tx *Tx) Delete(key Key) error {
	if key == "" {
		return ErrKeyEmpty
	}
	if entry, ok := tx.overlay[key]; ok {
		if entry.deleted {
			return ErrKeyNotFound
		}
	} else if !tx.db.state.exists(key) {
		return ErrKeyNotFound
	}
	tx.overlay[key] = txOverlay{deleted: true}
	tx.ops = append(tx.ops, Operation{Type: OpDelete, Key: key})
	return nil
}

// Exists reports whether the key exists from the transaction's perspective.
//
// Returns true if the key is live in the overlay or exists in the base
// state and has not been deleted in this transaction.
func (tx *Tx) Exists(key Key) bool {
	if key == "" {
		return false
	}
	if entry, ok := tx.overlay[key]; ok {
		return !entry.deleted
	}
	return tx.db.state.exists(key)
}

// Transaction executes fn within an atomic transaction.
//
// The function receives a *Tx that provides Set, Get, Delete, and Exists
// methods. All mutations are buffered in the transaction and only applied
// on commit.
//
// Commit: if fn returns nil, all operations are written to the WAL as a
// single atomic entry and applied to the in-memory state. Either all
// operations become visible or none do.
//
// Rollback: if fn returns a non-nil error, all buffered operations are
// discarded and the error is returned to the caller.
//
// Transaction-local reads: tx.Get sees changes made by earlier tx.Set
// or tx.Delete calls within the same transaction, even before commit.
//
// Locking: the transaction holds an exclusive write lock for its entire
// duration. No other reads or writes can proceed until the transaction
// completes. Keep transactions short.
//
// Returns ErrClosed if the database has been closed.
//
// Do NOT call DB methods (db.Set, db.Get, etc.) from within fn — use
// the Tx methods instead. Calling DB methods from within a transaction
// will deadlock.
func (db *DB) Transaction(fn func(tx *Tx) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrClosed
	}

	tx := &Tx{
		db:      db,
		overlay: make(map[Key]txOverlay),
	}

	if err := fn(tx); err != nil {
		// ROLLBACK: discard all buffered operations.
		return err
	}

	// COMMIT: write to WAL and apply to state.
	if len(tx.ops) == 0 {
		return nil // read-only transaction, nothing to commit
	}

	if err := db.validateChanges(tx.overlay); err != nil {
		return err
	}

	if db.wal != nil {
		if err := db.wal.appendBatch(tx.ops); err != nil {
			return err
		}
	}

	for _, op := range tx.ops {
		// All operations were validated during the transaction (tx.Set
		// always succeeds, tx.Delete checked existence). Apply should
		// not fail. Errors are ignored defensively.
		_ = db.state.apply(op)
	}

	return nil
}
