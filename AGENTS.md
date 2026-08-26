# Go Native DB — Agent Memory

## Project Identity

- **Module:** `github.com/MehulxBuilds/native-db`
- **Go version:** 1.24.5
- **Vision:** SQLite, but for JSON/key-value data. Lightweight, embedded, local-first, durable.
- **Core principle:** Deterministic State Machine. Same initial state + same ordered operations = same final state.

## Architecture

```
Public Go API  (db.go)
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
  state.go         — state struct (internal), in-memory keyspace, snapshot/load
  key.go           — Key type alias (currently string)
  value.go         — Value type with typed constructors/accessors
  value_json.go    — JSON marshaling/unmarshaling for Value (MarshalJSON, UnmarshalJSON)
  storage.go       — Storage interface + memoryStorage (no-op for in-memory mode)
  json_storage.go  — jsonStorage: JSON file persistence with atomic writes
  errors.go        — Sentinel errors (ErrKeyNotFound, ErrKeyEmpty)
  db_test.go       — 33 tests (DB ops, concurrency, persistence)
  value_test.go    — 10 tests (Value type, accessors, copy semantics)
```

## Design Decisions

1. **Value is a struct with `typ` + `data`**, not raw `interface{}` or `json.RawMessage`. Decouples the engine from JSON.
2. **Key is `type Key = string`** (alias, not defined type). Keeps it simple. Can evolve later.
3. **state is unexported.** Only DB is the public API surface. State is an internal implementation detail.
4. **Constructors copy slices/maps** (NewArray, NewObject). Accessors also return copies. Values are immutable once created.
5. **sync.RWMutex on DB**, not on state. State is a pure data structure; DB handles all synchronization. RLock for reads, Lock for writes.
6. **Storage interface** decouples DB from persistence format. `memoryStorage` for in-memory, `jsonStorage` for JSON files.
7. **Atomic file writes** — jsonStorage writes to a temp file then renames. No partial writes corrupt the database file.
8. **Open(path) signature** — `Open("")` for in-memory, `Open("file.db")` for persistent. Returns `(*DB, error)`.
9. **Save on Close()** — state is written to storage on Close(). No auto-save on each mutation (WAL in Phase 4 will handle crash durability).
10. **JSON stays contained** — `value_json.go` is the only file that imports `encoding/json`. The rest of the engine is JSON-agnostic.
11. **No WAL, no transactions, no operations model yet.** These are Phases 3-6.

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

## Development Roadmap (Remaining)

- **Phase 3** — Operation Model (explicit Operation struct, deterministic apply)
- **Phase 4** — WAL (write-ahead log, mutation flow: validate -> create op -> write WAL -> apply)
- **Phase 5** — Recovery (crash recovery via WAL replay)
- **Phase 6** — Transactions (BEGIN/COMMIT/ROLLBACK, atomicity)
- **Phase 7** — Concurrency (race detector testing, advanced locking if needed)
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
