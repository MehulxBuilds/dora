# Go Native DB — Agent Memory

## Project Identity

- **Module:** `github.com/MehulxBuilds/native-db`
- **Go version:** 1.24.5
- **Vision:** SQLite, but for JSON/key-value data. Lightweight, embedded, local-first, durable.
- **Core principle:** Deterministic State Machine. Same initial state + same ordered operations = same final state.

## Architecture

```
Public Go API  (db.go, tx.go)
      |
  Transaction Layer  (tx.go)
      |
  Operation Model  (operation.go)
      |
  WAL  (wal.go)
      |
  Storage interface  (storage.go)
      |
  JSON Storage  (json_storage.go)
      |
  Key-Value State  (state.go)
      |
  Value abstraction  (value.go, value_json.go)
      |
  Error system  (errors.go)
```

JSON is a serialization format, NOT the core architecture. The Value type is a database-level abstraction independent of serialization.

## Package Structure

```
engine/
  db.go            — DB struct, public API (Open, Close, Set, Get, Delete, Exists, Len, Keys), sync.RWMutex
  tx.go            — Tx struct, tx.Set/Get/Delete/Exists, DB.Transaction (atomic commit/rollback)
  operation.go     — OpType (SET, DELETE), Operation struct, state.apply (deterministic state transition)
  wal.go           — WAL struct, append (write+fsync), appendBatch (TX entry), loadWALEntries (replay), truncate, JSON Lines format
  state.go         — state struct (internal), in-memory keyspace, snapshot/load
  key.go           — Key type alias (currently string)
  value.go         — Value type with typed constructors/accessors
  value_json.go    — JSON marshaling/unmarshaling for Value (MarshalJSON, UnmarshalJSON)
  storage.go       — Storage interface + memoryStorage (no-op for in-memory mode)
  json_storage.go  — jsonStorage: JSON file persistence with atomic writes
  errors.go        — Sentinel errors (ErrKeyNotFound, ErrKeyEmpty)
```

## Design Decisions

1. **Value is a struct with `typ` + `data`**, not raw `interface{}` or `json.RawMessage`. Decouples the engine from JSON.
2. **Key is `type Key = string`** (alias, not defined type). Keeps it simple. Can evolve later.
3. **state is unexported.** Only DB is the public API surface. State is an internal implementation detail.
4. **Constructors copy slices/maps** (NewArray, NewObject). Accessors also return copies. Values are immutable once created.
5. **sync.RWMutex on DB**, not on state. State is a pure data structure; DB handles all synchronization. RLock for reads, Lock for writes. All methods use defer for unlock to prevent leaks on error paths.
6. **Storage interface** decouples DB from persistence format. `memoryStorage` for in-memory, `jsonStorage` for JSON files.
7. **Atomic file writes** — jsonStorage writes to a temp file then renames. No partial writes corrupt the database file.
8. **Open(path) signature** — `Open("")` for in-memory, `Open("file.db")` for persistent. Returns `(*DB, error)`.
9. **Save on Close()** — state is written to storage on Close(). WAL is truncated after snapshot save.
10. **JSON stays contained** — `value_json.go` is the only file that imports `encoding/json` (besides storage/WAL layers). The rest of the engine is JSON-agnostic.
11. **No transactions yet.** These are Phase 6.
12. **DB.Set/Delete route through Operations.** Every mutation creates an Operation and calls state.apply. This is the foundation for WAL (Phase 4) and transactions (Phase 6).
13. **WAL uses JSON Lines format.** One JSON object per line, fsync after each write. Human-readable, each entry self-contained. WAL file is `<db-path>.wal`.
14. **WAL write-before-apply.** Operations are durable on disk before being applied to in-memory state. Crash between WAL write and apply is recovered on next Open.
15. **Delete validates before WAL write.** Existence check happens under write lock before WAL append, ensuring only valid operations reach the WAL.

## Completed Phases

### Phase 0 — Project Foundation (DONE)
- DB, State, Key, Value, Error system, package structure
- 20 tests, all passing
- Purely in-memory, no persistence

### Phase 1 — In-Memory Key-Value Engine (DONE)
- Added sync.RWMutex to DB for concurrency safety
- Added Keys() method, Close() lifecycle hook
- 33 tests total (added concurrency, operation sequences, edge cases, stress)

### Phase 2 — JSON Persistence (DONE)
- Storage interface: abstracts persistence behind Load/Save/Close
- jsonStorage: reads/writes human-readable JSON files with atomic writes (temp+rename)
- memoryStorage: no-op implementation for in-memory databases
- Value JSON serialization: MarshalJSON/UnmarshalJSON contained in value_json.go
- Open(path) loads state from file; Close() saves state to file
- 43 tests total, all passing
- New persistence tests: round-trip, all value types, reopen, deletes survive, overwrites survive, corrupt file rejection, empty file, readable JSON verification, multiple close/reopen cycles, deeply nested values

### Phase 3 — Operation Model (DONE)
- OpType enum (OpSet, OpDelete) with String() method
- Operation struct: Type, Key, Value
- state.apply(op) — single deterministic state-transition function
- DB.Set and DB.Delete now create Operations and apply them (no longer call state.set/state.delete directly)
- 53 tests total, all passing
- New operation tests: apply SET, apply SET overwrite, apply DELETE, apply DELETE not found, unknown OpType error, OpType.String, deterministic replay (two independent states), deleted keys absent after replay, replay matches direct mutation, DB sequence matches direct apply

### Phase 4 — WAL (DONE)
- WAL struct: file-based write-ahead log with JSON Lines format
- append(op): marshals operation to JSON, writes line, fsyncs for durability
- loadWALEntries(path): reads all entries from existing WAL file for replay
- truncate(): clears WAL after successful snapshot save
- DB.Set mutation flow: validate → create op → WAL append → apply to state
- DB.Delete mutation flow: validate → check existence → create op → WAL append → apply
- Open() replays WAL entries after loading snapshot (crash recovery)
- Close() saves snapshot then truncates WAL (clean shutdown)
- In-memory databases (Open("")) have no WAL — wal field is nil
- WAL file location: `<db-path>.wal`

### Phase 5 — Recovery (DONE)
- loadWALEntries rewritten for crash tolerance: parses entries sequentially, stops at first corrupt/partial entry instead of returning a hard error
- Partial entry repair: WAL file is truncated to the byte offset of the last valid entry, removing trailing garbage from crash mid-write
- Valid entries are preserved in WAL after repair (not removed) — protects against a second crash before Close() saves a snapshot
- Recovery guarantees: with fsync-per-entry, at most one entry (the last) can be incomplete; all preceding entries are fully durable
- Semantic errors (unknown op type, SET without value) still return hard errors — these indicate real corruption, not partial writes
- Replaced bufio.Scanner with os.ReadFile + manual byte-offset tracking for precise truncation positioning

### Phase 6 — Transactions (DONE)
- Tx struct with overlay map for transaction-local reads: tx.Get sees uncommitted tx.Set/tx.Delete changes
- tx.Set, tx.Get, tx.Delete, tx.Exists — mirror DB API but buffer mutations in overlay
- DB.Transaction(fn) — functional API: fn returns nil → COMMIT, fn returns error → ROLLBACK
- Atomicity: all operations committed to WAL as single TX entry, or none (rollback discards overlay)
- WAL TX entry: `{"type":"TX","ops":[...]}` — single JSON line, single write+fsync, all-or-nothing
- wal.appendBatch(ops) — serializes transaction as one atomic WAL entry
- loadWALEntries updated to unpack TX entries into flat operation list during replay
- Serialized transactions: exclusive write lock held for entire transaction duration
- Read-only transactions produce no WAL entry
- Empty key validation on all Tx methods

### Phase 7 — Concurrency (DONE)
- Full concurrency audit of all code paths for data races
- **Bug fixed:** Close() previously released the write lock before operating on WAL/storage — concurrent Set/Delete could write to a closing WAL. Now Close() holds the exclusive lock for its entire duration (snapshot + save + truncate + close)
- Added `closed bool` field to DB struct, checked under lock in all public methods
- Added `ErrClosed` sentinel error — returned by Set, Get, Delete, Transaction, Close (double-close)
- Non-error-returning methods (Exists, Len, Keys) return zero values (false, 0, nil) after close
- All unlock calls now use defer — prevents lock leaks on early returns and error paths
- Concurrency model remains simple: single sync.RWMutex, no per-key locks, no MVCC
- Value type is race-free by design: passed by value, constructors copy slices/maps, accessors return copies

## Development Roadmap (Remaining)

- **Phase 8** — Snapshots (periodic checkpoints, snapshot + WAL replay for recovery)
- **Phase 9** — Deterministic Verification (operation IDs, sequence numbers, state hashing)
- **Phase 10** — Prefixes/Namespaces (db.Keys("user:"), db.Prefix("user:"))

## Rules

- Do NOT implement a query language
- Do NOT skip tests
- Do NOT introduce abstractions without concrete need
- Correctness before performance
- Every mutation must be representable as a deterministic operation (enforced from Phase 3)
- Snapshots are checkpoints, not canonical history
- Keep the engine embeddable (no server, no network)

## Environment Notes

- Race detector (`go test -race`) requires 64-bit CGO compiler, not available in current dev environment. Test without `-race` for now; use `-race` in CI.
