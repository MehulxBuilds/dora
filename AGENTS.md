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
  Key-Value State  (state.go)
      |
  Value abstraction  (value.go)
      |
  Error system  (errors.go)
```

JSON is a serialization format, NOT the core architecture. The Value type is a database-level abstraction independent of serialization.

## Package Structure

```
engine/
  db.go          — DB struct, public API (Open, Close, Set, Get, Delete, Exists, Len, Keys), sync.RWMutex
  state.go       — state struct (internal), in-memory keyspace (map[Key]Value), keys()
  key.go         — Key type alias (currently string)
  value.go       — Value type with typed constructors/accessors (Null, Boolean, Number, String, Array, Object)
  errors.go      — Sentinel errors (ErrKeyNotFound, ErrKeyEmpty)
  db_test.go     — 23 tests covering DB operations, concurrency, edge cases
  value_test.go  — 10 tests covering Value type, accessors, copy semantics
```

## Design Decisions

1. **Value is a struct with `typ` + `data`**, not raw `interface{}` or `json.RawMessage`. This decouples the entire engine from JSON.
2. **Key is `type Key = string`** (alias, not defined type). Keeps it simple. Can evolve later.
3. **state is unexported.** Only DB is the public API surface. State is an internal implementation detail.
4. **Constructors copy slices/maps** (NewArray, NewObject). Accessors also return copies. Values are immutable once created.
5. **sync.RWMutex on DB**, not on state. State is a pure data structure; DB handles all synchronization. RLock for reads (Get, Exists, Len, Keys), Lock for writes (Set, Delete).
6. **No persistence yet.** Phase 2 will add JSON persistence behind a storage abstraction.
7. **No WAL, no transactions, no operations model yet.** These are Phases 3-6.

## Completed Phases

### Phase 0 — Project Foundation (DONE)
- DB, State, Key, Value, Error system, package structure
- 20 tests, all passing
- Purely in-memory, no persistence

### Phase 1 — In-Memory Key-Value Engine (DONE)
- Added sync.RWMutex to DB for concurrency safety
- Added Keys() method (returns all keys, unordered)
- Added Close() method (no-op now, lifecycle hook for Phase 2)
- 33 tests total, all passing
- New tests: concurrency (concurrent reads, writes, mixed read/write), operation sequences (set-delete-set, double delete), edge cases (special characters, unicode keys, nested values, null values), stress test (10k keys)

## Development Roadmap (Remaining)

- **Phase 2** — JSON Persistence (save/load state to disk behind abstraction)
- **Phase 3** — Operation Model (explicit Operation struct, deterministic apply)
- **Phase 4** — WAL (write-ahead log, mutation flow: validate -> create op -> write WAL -> apply)
- **Phase 5** — Recovery (crash recovery via WAL replay)
- **Phase 6** — Transactions (BEGIN/COMMIT/ROLLBACK, atomicity)
- **Phase 7** — Concurrency (sync.RWMutex, race detector testing)
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
