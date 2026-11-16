# Dora DB

## Project Identity

- **Module:** `github.com/MehulxBuilds/dora`
- **Go version:** 1.24.5
- **License:** MIT
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
  db.go            — DB struct, public API (Open, Close, Set, Get, Delete, Exists, Len, Keys, Prefix, Snapshot, Seq, StateHash), sync.RWMutex
  tx.go            — Tx struct, tx.Set/Get/Delete/Exists, DB.Transaction (atomic commit/rollback)
  operation.go     — OpType (SET, DELETE), Operation struct, state.apply (deterministic state transition)
  wal.go           — WAL struct, append (write+fsync), appendBatch (TX entry), loadWALEntries (replay), truncate, JSON Lines format
  state.go         — state struct (internal), in-memory keyspace (map[Key]Value + seq uint64), snapshot, hash, prefix queries
  key.go           — Key type alias (currently string)
  value.go         — Value type with typed constructors/accessors
  value_json.go    — JSON marshaling/unmarshaling for Value (MarshalJSON, UnmarshalJSON)
  storage.go       — Storage interface + memoryStorage (no-op for in-memory mode)
  json_storage.go  — jsonStorage: JSON file persistence with atomic writes (temp+rename)
  errors.go        — Sentinel errors (ErrKeyNotFound, ErrKeyEmpty, ErrClosed)
```

## Design Decisions

1. **Value is a struct with `typ` + `data`**, not raw `interface{}` or `json.RawMessage`. Decouples the engine from JSON.
2. **Key is `type Key = string`** (alias, not defined type). Keeps it simple. Can evolve later.
3. **state is unexported.** Only DB is the public API surface. State is an internal implementation detail.
4. **Constructors copy slices/maps** (NewArray, NewObject). Accessors also return copies. Values are immutable once created.
5. **sync.RWMutex on DB**, not on state. State is a pure data structure; DB handles all synchronization. RLock for reads, Lock for writes. All methods use defer for unlock to prevent leaks on error paths.
6. **Storage interface** decouples DB from persistence format. `memoryStorage` for in-memory, `jsonStorage` for JSON files.
7. **Atomic file writes** — jsonStorage writes to a temp file then renames. No partial writes corrupt the database file.
8. **Open(path) signature** — `Open("")` for in-memory, `Open("file.db")` for persistent. Returns `(*DB, error)`. Persistent files are automatically placed inside `dora/` directory (branding convention, like `prisma/` or `drizzle/`). The `dora/` directory is created automatically.
9. **Save on Close()** — state is written to storage on Close(). WAL is truncated after snapshot save.
10. **JSON stays contained** — `value_json.go` is the only file that imports `encoding/json` (besides storage/WAL layers). The rest of the engine is JSON-agnostic.
11. **DB.Set/Delete route through Operations.** Every mutation creates an Operation and calls state.apply. This is the foundation for WAL and transactions.
12. **WAL uses JSON Lines format.** One JSON object per line, fsync after each write. Human-readable, each entry self-contained. WAL file is `<db-path>.wal`.
13. **WAL write-before-apply.** Operations are durable on disk before being applied to in-memory state. Crash between WAL write and apply is recovered on next Open.
14. **Delete validates before WAL write.** Existence check happens under write lock before WAL append, ensuring only valid operations reach the WAL.
15. **Transaction atomicity via single WAL entry.** All operations in a transaction are written as one `{"type":"TX","ops":[...]}` JSON line. One write+fsync = all-or-nothing.
16. **Serialized transactions.** Exclusive write lock held for the entire transaction duration. No MVCC, no optimistic concurrency — simple and correct.
17. **Closed guard on all public methods.** `closed bool` field checked under lock. Error-returning methods return `ErrClosed`. Non-error methods (Exists, Len, Keys) return zero values.
18. **Seq persisted via reserved key.** `\x00_seq` is injected into snapshot map on save, extracted and removed on load. No Storage interface changes needed. NUL-prefix prevents user collision.
19. **StateHash is data-only.** SHA-256 covers key-value content, NOT the sequence number. Two databases with identical data hash identically regardless of history.
20. **Prefix queries are pure filtering.** O(n) scan with `strings.HasPrefix`. No index, no tree — correctness before performance.

## Internal Component Reference

### DB (db.go)

The public API surface. Holds `sync.RWMutex`, coordinates state/WAL/storage.

**Fields:**

- `mu sync.RWMutex` — all synchronization
- `state *state` — the in-memory keyspace
- `storage Storage` — persistence backend
- `wal *wal` — write-ahead log (nil for in-memory)
- `closed bool` — prevents use after Close()

**Constants:**

- `metaSeqKey = "\x00_seq"` — reserved key for persisting seq in snapshots

**Public methods:**

- `Open(path) (*DB, error)` — constructor. `""` = in-memory, else persistent with WAL
- `Close() error` — checkpoint + close handles + set closed=true
- `Snapshot() error` — checkpoint without closing (controls WAL growth)
- `Set(key, val) error` — WAL append → state.apply
- `Get(key) (Value, error)` — state.get under RLock
- `Delete(key) error` — existence check → WAL append → state.apply
- `Exists(key) bool` — state.exists under RLock
- `Len() int` — state.len under RLock
- `Keys(prefix ...string) []Key` — all keys or prefix-filtered
- `Prefix(prefix) map[Key]Value` — key-value pairs by prefix
- `Seq() uint64` — current sequence number
- `StateHash() ([32]byte, error)` — SHA-256 of state data

**Internal methods:**

- `checkpoint() error` — snapshot state → inject seq → save to storage → truncate WAL. Called by Close() and Snapshot(). Must be called under write lock.

### State (state.go)

Pure in-memory data structure. No locks, no I/O.

**Fields:**

- `data map[Key]Value` — the keyspace
- `seq uint64` — operation counter, incremented on each successful apply

**Methods:**

- `get(key) (Value, error)` — map lookup
- `set(key, val)` — direct map write (used by legacy code paths only)
- `delete(key) error` — map delete with existence check
- `exists(key) bool` — map membership
- `len() int` — map size
- `keys() []Key` — all keys (unordered)
- `keysWithPrefix(prefix) []Key` — filtered by `strings.HasPrefix`
- `prefix(pfx) map[Key]Value` — filtered key-value pairs
- `snapshot() map[Key]Value` — shallow copy for persistence
- `hash() ([32]byte, error)` — SHA-256 (sorted keys, length-prefixed key+value bytes)
- `apply(op) error` — deterministic state transition (defined in operation.go)

### Operation Model (operation.go)

**Types:**

- `OpType int` — enum: `OpSet`, `OpDelete`
- `Operation struct` — `Type OpType`, `Key Key`, `Value Value`

**`state.apply(op) error`** — the single deterministic state transition function:

- `OpSet`: `s.data[op.Key] = op.Value; s.seq++`
- `OpDelete`: check existence → `delete(s.data, op.Key); s.seq++`
- Unknown type: return error

Failed operations (e.g. DELETE non-existent key) do NOT increment seq.

### WAL (wal.go)

File-based write-ahead log. JSON Lines format.

**On-disk format (`walEntry`):**

```json
{"type":"SET","key":"user:1","value":{"name":"Mehul"}}
{"type":"DELETE","key":"temp:session"}
{"type":"TX","ops":[{"type":"SET","key":"a","value":1},{"type":"SET","key":"b","value":2}]}
```

**Fields on walEntry struct:**

- `Type string` — `"SET"`, `"DELETE"`, or `"TX"`
- `Key Key` — present for SET/DELETE, omitempty for TX
- `Value *Value` — present for SET only, omitempty
- `Ops []walEntry` — present for TX only, omitempty

**Methods:**

- `openWAL(path) (*wal, error)` — open/create in append mode
- `append(op) error` — marshal → write → fsync (single operation)
- `appendBatch(ops) error` — marshal TX entry → write → fsync (transaction)
- `truncate() error` — clear file to 0 bytes (after snapshot save)
- `close() error` — close file handle

**`loadWALEntries(path) ([]Operation, error)`** — recovery function:

1. Read entire file with `os.ReadFile`
2. Parse entries line by line with manual byte-offset tracking
3. On corrupt/incomplete entry: stop parsing, truncate file to last valid offset
4. For TX entries: recursively unpack into flat `[]Operation` list
5. Return all valid operations for replay

**`entryToOperation(walEntry) ([]Operation, error)`** — converts on-disk entry to Operation(s):

- SET → single-element slice
- DELETE → single-element slice
- TX → recursively unpack all sub-entries

### Transaction (tx.go)

**Tx struct fields:**

- `db *DB` — reference to parent database (for reading base state)
- `overlay map[Key]txOverlay` — transaction-local changes
- `ops []Operation` — ordered operation list for WAL

**txOverlay struct:**

- `value Value` — the value (if not deleted)
- `deleted bool` — true if key was deleted in this transaction

**tx.Get lookup order:**

1. Check overlay → if deleted, return ErrKeyNotFound; if set, return value
2. Fall through to `db.state.get(key)` (base state, no lock needed — write lock held by Transaction)

**tx.Delete validation:**

1. Check overlay → if already deleted, return ErrKeyNotFound
2. If not in overlay, check `db.state.exists(key)`
3. Mark deleted in overlay, append OpDelete to ops

**DB.Transaction(fn) flow:**

1. Acquire exclusive write lock
2. Create Tx with empty overlay
3. Call fn(tx) — user code runs tx.Set/Get/Delete/Exists
4. If fn returns error → ROLLBACK (discard Tx, return error)
5. If fn returns nil → COMMIT:
   - If no ops (read-only): return nil, no WAL entry
   - Write all ops as single TX WAL entry via appendBatch
   - Apply all ops to state via state.apply (errors ignored — already validated)
6. Release lock

### Storage (storage.go, json_storage.go)

**Storage interface:**

```go
Load() (map[Key]Value, error)
Save(data map[Key]Value) error
Close() error
```

**memoryStorage:** no-op for all methods. Load returns empty map.

**jsonStorage:**

- `Load()`: ReadFile → handle not-exist (empty map) / empty file (empty map) → json.Unmarshal
- `Save(data)`: json.MarshalIndent → write temp file → os.Rename (atomic)
- `Close()`: no-op (no persistent file handles)

### Value (value.go, value_json.go)

**ValueType enum:** TypeNull, TypeBoolean, TypeNumber, TypeString, TypeArray, TypeObject

**Value struct:** `typ ValueType`, `data interface{}`

**Constructors:** NewNull, NewBoolean(bool), NewNumber(float64), NewString(string), NewArray([]Value), NewObject(map[string]Value). Array and Object constructors copy their input.

**Accessors:** return `(typed_value, ok bool)`. Array and Object accessors return copies.

**JSON (value_json.go):**

- `MarshalJSON()` — switches on typ, marshals underlying Go value
- `UnmarshalJSON(data)` — unmarshals to `interface{}`, then `valueFromInterface` recursively converts
- `valueFromInterface(raw)` — nil→Null, bool→Boolean, float64→Number, string→String, []interface{}→Array, map[string]interface{}→Object

### Errors (errors.go)

Three sentinel errors:

- `ErrKeyNotFound` — Get/Delete on non-existent key
- `ErrKeyEmpty` — any operation with `""` key
- `ErrClosed` — any operation after Close()

## Mutation Flow

Every write goes through this exact pipeline:

```
db.Set("key", val)
        |
  Validate key != ""              →  ErrKeyEmpty
        |
  Acquire write lock (defer unlock)
        |
  Check closed                    →  ErrClosed
        |
  Create Operation{OpSet, key, val}
        |
  WAL append (if persistent)      →  marshal JSON → write → fsync
        |                              (durable after this point)
  state.apply(op)                 →  s.data[key] = val; s.seq++
        |
  Release lock (via defer)
```

```
db.Delete("key")
        |
  Validate key != ""              →  ErrKeyEmpty
        |
  Acquire write lock (defer unlock)
        |
  Check closed                    →  ErrClosed
        |
  Check state.exists(key)         →  ErrKeyNotFound
        |
  Create Operation{OpDelete, key}
        |
  WAL append (if persistent)      →  marshal JSON → write → fsync
        |
  state.apply(op)                 →  delete(s.data, key); s.seq++
        |
  Release lock (via defer)
```

## Recovery Flow

```
Open("app.db")
      |
  Create jsonStorage("app.db")
      |
  storage.Load()                  →  read JSON file → map[Key]Value
      |
  Extract seq from data["\x00_seq"], delete reserved key
      |
  newStateFrom(data), set state.seq = seq
      |
  loadWALEntries("app.db.wal")
      |   - parse line by line
      |   - stop at first corrupt entry
      |   - truncate file to remove partial entry
      |   - unpack TX entries into flat list
      |
  Replay: for each op, state.apply(op) (errors ignored)
      |   - seq increments with each successful apply
      |
  openWAL("app.db.wal") for appending
      |
  Return &DB{state, storage, wal}
```

## Snapshot File Format

Human-readable JSON. The `\x00_seq` key is internal metadata:

```json
{
  "\u0000_seq": 42,
  "counter": 10,
  "user:1": {
    "age": 20,
    "name": "Mehul"
  }
}
```

On load, `\x00_seq` is extracted and removed from the map before creating the state. The user never sees it through Keys(), Len(), Get(), etc.

## WAL File Format

JSON Lines. One JSON object per line. Three entry types:

```
{"type":"SET","key":"user:1","value":{"name":"Mehul","age":20}}
{"type":"DELETE","key":"temp:session"}
{"type":"TX","ops":[{"type":"SET","key":"a","value":1},{"type":"DELETE","key":"b"}]}
```

Each entry is followed by `\n`. Each write is followed by `fsync`. Crash mid-write leaves at most one incomplete line, which recovery detects and truncates.

## State Hash Algorithm

`db.StateHash()` returns SHA-256 of the state data (not seq). Algorithm:

1. Collect all keys, sort lexicographically
2. Write key count as 4-byte big-endian uint32
3. For each key in sorted order:
   - Write key byte length as 4-byte big-endian uint32
   - Write key bytes
   - Marshal value to JSON via Value.MarshalJSON()
   - Write value byte length as 4-byte big-endian uint32
   - Write value JSON bytes
4. Return SHA-256 of the entire concatenation

Determinism guaranteed by: sorted keys, Go's json.Marshal sorting map keys, length-prefix encoding preventing ambiguity.

## Concurrency Model

```
Reads  (Get, Exists, Len, Keys, Prefix, Seq, StateHash)  →  RLock  (concurrent)
Writes (Set, Delete)                                       →  Lock   (exclusive)
Transactions                                               →  Lock   (exclusive, held for full fn duration)
Close / Snapshot                                           →  Lock   (exclusive, held for full operation)
```

Single `sync.RWMutex` on DB. State has no locks. WAL has no locks (always accessed under DB write lock). Value is race-free (passed by value, constructors/accessors copy).

## Known Limitations and Edge Cases

1. **No MVCC.** Transactions hold exclusive lock — all reads and writes block until the transaction completes. Keep transactions short.
2. **No query language.** API is Go functions only. A query language is deliberately postponed.
3. **O(n) prefix queries.** `Keys("prefix")` and `Prefix("prefix")` scan all keys. No index or sorted structure yet.
4. **float64 numbers.** All numbers are float64. No big integers, no decimals. Integers up to 2^53 are exact.
5. **Seq may over-count after crash-between-snapshot-and-truncate.** If a crash happens after snapshot save but before WAL truncate, WAL replay re-applies already-snapshotted operations, incrementing seq redundantly. StateHash is still correct. Fix: add seq to WAL entries (future work).
6. **No partial updates.** To change one field in an Object, you must read, modify, and write the entire value. Document-level PATCH is future work.
7. **Snapshot blocks all access.** `Snapshot()` and `Close()` hold the write lock for the entire disk write. For large databases, this can cause noticeable latency.
8. **Single file, single process.** No file locking. Running two processes on the same database file will corrupt it.
9. **WAL fsync per entry.** Every Set/Delete fsyncs. This is correct but slow under heavy write load. Batching/group commit is future work.
10. **Reserved key `\x00_seq`.** A user who explicitly sets a key starting with `\x00` could collide with internal metadata. Extremely unlikely in practice.

## Completed Phases

### Phase 0 — Project Foundation (DONE)

- DB, State, Key, Value, Error system, package structure
- Purely in-memory, no persistence

### Phase 1 — In-Memory Key-Value Engine (DONE)

- Added sync.RWMutex to DB for concurrency safety
- Added Keys() method, Close() lifecycle hook

### Phase 2 — JSON Persistence (DONE)

- Storage interface: abstracts persistence behind Load/Save/Close
- jsonStorage: reads/writes human-readable JSON files with atomic writes (temp+rename)
- memoryStorage: no-op implementation for in-memory databases
- Value JSON serialization: MarshalJSON/UnmarshalJSON contained in value_json.go
- Open(path) loads state from file; Close() saves state to file

### Phase 3 — Operation Model (DONE)

- OpType enum (OpSet, OpDelete) with String() method
- Operation struct: Type, Key, Value
- state.apply(op) — single deterministic state-transition function
- DB.Set and DB.Delete now create Operations and apply them

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

### Phase 8 — Snapshots (DONE)

- Added public `db.Snapshot()` method — saves current state to storage and truncates WAL without closing the database
- Extracted internal `db.checkpoint()` method — shared implementation between Snapshot() and Close()
- Refactored `Close()` to call `checkpoint()` then close file handles — eliminates code duplication
- Snapshot() acquires exclusive write lock, returns ErrClosed if database is closed
- For in-memory databases, Snapshot() runs through harmlessly (memoryStorage.Save is a no-op)
- Recovery model: Load snapshot → replay WAL entries added since last Snapshot/Close
- Snapshot is a checkpoint, not canonical history — WAL is the source of truth until next checkpoint
- No automatic/periodic snapshotting — callers control when to checkpoint (e.g. via goroutine + ticker)

### Phase 9 — Deterministic Verification (DONE)

- Added `seq uint64` to state — monotonically increasing counter, incremented on every successful `state.apply()`
- `db.Seq() uint64` — returns current sequence number (operation ID / position in history)
- `db.StateHash() ([32]byte, error)` — computes SHA-256 of entire state (data only, NOT seq)
- State hash algorithm: sort keys lexicographically, feed length-prefixed key bytes + JSON value bytes into SHA-256
- Hash is deterministic: same key-value data → same hash, regardless of operation history
- Seq persisted in snapshots via reserved key `\x00_seq` (NUL-prefix prevents user collision)
- On Open: seq extracted from loaded data and removed before creating state (invisible to user)
- On checkpoint: seq injected into snapshot map before saving
- Seq increments during WAL replay (each replayed apply increments seq naturally)
- No changes to Storage interface, WAL format, or json_storage — seq piggybacks on existing data map

### Phase 10 — Prefixes/Namespaces (DONE)

- `db.Keys("user:")` — returns all keys matching a prefix (variadic: `db.Keys()` with no args still returns all keys)
- `db.Prefix("user:")` — returns `map[Key]Value` of all key-value pairs matching a prefix
- `state.keysWithPrefix(prefix)` and `state.prefix(prefix)` — internal helpers using `strings.HasPrefix`
- Empty prefix matches all keys/pairs (consistent with no-arg behavior)
- Pure filtering layer — no changes to operations, WAL, storage, or transactions
- Returns nil if database is closed (consistent with other read methods)

## Future Work

- **Document-level updates** — PATCH operations for partial Object updates
- **Compaction** — WAL compaction and snapshot optimization
- **Advanced querying** — range queries, filtering, sorting
- **Query planner** — cost-based query optimization
- **MVCC** — multi-version concurrency control for snapshot isolation
- **Time travel** — read state at a previous sequence number
- **Replication** — operation log shipping to replicas
- **Binary storage format** — replace JSON with a compact binary format
- **File locking** — prevent multiple processes from corrupting the same file
- **Group commit** — batch WAL fsyncs for higher write throughput
- **Incremental hashing** — maintain running hash instead of recomputing from scratch
- **Comprehensive test suite** — reference tests now exist; broader engine coverage remains future work

## Build and Verify

```bash
# Build
go build ./engine/

# Vet
go vet ./engine/

# Test (when tests exist)
go test ./engine/ -v -count=1

# Race detector (requires CGO)
go test ./engine/ -race -count=1
```

## Phase 11 - References / Relations (DONE)

- `TypeReference`, `NewReference(key)`, and `Value.Reference()` represent typed cross-key references, including inside arrays and objects.
- Targets must exist in the final state. Empty targets return `ErrReferenceNotFound`; deleting a target still referenced by another live value returns `ErrKeyReferenced`.
- Self-references and cycles are allowed. Reads return references without automatic traversal.
- DB writes validate before WAL append; transactions validate the complete final overlay before committing. Forward references and deleting related keys in either order are supported within a transaction.
- Recovery replays operations without per-operation integrity checks, then validates the recovered state before opening the WAL handle.
- JSON encodes references as `{"\u0000_ref":"target"}`. Objects containing either reserved marker (`\u0000_ref`, `\u0000_object`) are escaped inside a `\u0000_object` wrapper to preserve their type. Historical files using these exact marker shapes are ambiguous and require migration before opening.
- Integrity validation copies/scans the final keyspace and recursively visits values; no reverse index or cascading deletes yet.
- `engine/reference.go` contains deterministic validation. `engine/reference_test.go` covers integrity, transaction atomicity, encoding collisions, WAL replay and snapshots.

## Documentation workflow

- `main.go` includes runnable reference examples in an isolated in-memory database: nested object/array references, forward references in a transaction, explicit target lookup, missing-target errors, protected deletion, and atomic deletion of related records. Run with `go run .`; the existing basic demo also writes `dora/demo.db`.

Always update AGENTS.md and README.md at the end of implementation work to describe the resulting behavior and limitations.

## Phase 12 - Secondary Indexes (DONE)

- `engine/index.go`: `DB.CreateIndex(name, prefix, field)`, `DB.DropIndex(name)`, `DB.Lookup(name, value)`, and `Tx.Lookup(name, value)`.
- Non-unique exact-match indexes select literal top-level object fields from prefix-matching keys. Empty prefix selects all keys; empty index names/fields are rejected.
- Scalar buckets distinguish null, boolean, finite number, string, and reference types. References match their target key without traversal; numeric negative zero equals zero. Missing fields, non-object records, arrays, objects, and non-finite numbers are excluded.
- Lookup returns sorted, independent key slices (empty slice on no match). Unsupported lookup values return `ErrIndexValue`; missing indexes return `ErrIndexNotFound`; duplicate names return `ErrIndexExists`; invalid definitions return `ErrIndexInvalid`.
- State owns derived index buckets. `state.apply` updates them on SET/DELETE and builds/removes them for `OpCreateIndex`/`OpDropIndex`. Public methods validate, acquire DB locks, and write WAL before applying changes.
- Index creation/drop increment Seq but do not affect data-only StateHash. Create/drop are DB methods only; inside transaction callbacks use Tx.Lookup and never call DB methods.
- Transactions combine committed candidates with the overlay for lookups; commit maintains buckets through normal operations, rollback leaves indexes unchanged.
- WAL adds CREATE_INDEX/DROP_INDEX records with an index definition. Snapshots store definitions under reserved `\x00_indexes`, removed before exposing data. Buckets are rebuilt from snapshot records when definitions load and maintained during replay; buckets are never serialized.
- Reserved snapshot metadata keys must not be used as application keys. Existing files using `\x00_indexes` as user data require migration.
- Index creation scans existing data under the write lock. Bucket lookups cost O(matches + sorting); Tx.Lookup additionally scans the overlay. Write-time reference validation still scans the keyspace.
- `main.go` uses the persistent demo DB for index examples. `engine/index_test.go` covers maintenance, scalar distinctions, rollback, transaction visibility, WAL failure, snapshots, and crash recovery.
- Deferred: unique constraints (next addition), nested paths, compound indexes, ranges, and query planning.
- Verification: `go test ./... -count=1`, `go vet ./...`, and `go build ./...` pass. Race verification requires a working 64-bit C compiler (the installed compiler previously failed).

## Restart-safe index initialization

- `DB.EnsureIndex(name, prefix, field)` shares the locked create path with CreateIndex. Missing definitions are created normally; identical definitions are a no-op (no WAL write, bucket rebuild, or Seq increment).
- A same-name definition with a different prefix or field returns `ErrIndexConflict`. CreateIndex retains strict `ErrIndexExists` semantics.
- `main.go` uses EnsureIndex and keeps the index for subsequent runs rather than dropping it at the end.
- `TestEnsureIndexRestart` verifies reopen, lookup, unchanged WAL/Seq on reuse, conflicts, strict CreateIndex, and invalid/closed guards.

## Phase 13 - Dora Studio (DONE)

- Root `cli/` contains CLI code; `cli/cmd/dora/main.go` is the sole executable entry point for both development and installation. `cli/server` is an importable local HTTP server; `studio/` contains React/TypeScript/Vite source and the embedded compiled dist/ assets.
- CLI accepts --path (default ./dora), --port (default 7432, zero chooses an available port), --no-open, and an optional explicit file path. It opens the browser after binding, prints the URL, and gracefully shuts down on Ctrl+C. The default discovery root is <cwd>/dora; --path selects a directory. Explicit file paths are exact and do not prepend dora/.
- `engine/inspect.go`: DB.Inspect captures a detached consistent view under RLock; InspectFile reads snapshot + WAL without opening a DB, writing, repairing, truncating, creating directories, or checkpointing. Offline inspection requires the owning application to be stopped; there is no inter-process file lock.
- WAL parsing shares readWALEntries(path, repair). Normal recovery uses repair=true; inspection uses false and leaves incomplete tails intact. Semantic corruption remains an error.
- Inspection exposes detached Records, sorted Indexes, Seq, Hash, and a read-only Lookup scanning the detached records. Mutating returned containers does not affect DB state.
- Live integration: server.Start(ctx, db, server.Options{Port:7432,Name:"app.db"}); the server never closes the caller's DB. Each request captures a new view, so consistency is per request, not across the browser session.
- GET/HEAD endpoints: /api/overview (metadata, key/type/preview summaries, indexes), /api/record?key=..., /api/lookup?name=...&value=<JSON>. GET does not perform mutations. Errors use JSON; non-read methods return 405; unknown endpoints return 404.
- Local listener only (127.0.0.1). Handler checks loopback Host, rejects foreign Origin and cross-site fetch metadata, serves no CORS headers, uses no-store caching and restrictive content security headers.
- UI: record table, case-insensitive key search, prefix namespace filters, 50-row pagination, JSON inspector, clickable references, index definitions/exact-match lookup, refresh, errors/empty states, mode/Seq/hash metadata. No mutation controls.
- Frontend build: cd studio; bun install --frozen-lockfile; bun run build (npm run build also works with installed dependencies). studio/assets.go embeds dist; generated assets are tracked and rebuilt with UI source changes. No Node.js runtime needed by end users. Vite proxies /api to 127.0.0.1:7432 for development.
- Inspection and HTTP tests verify no file changes (including partial WAL tails), detached snapshots, index recovery, mutation/origin rejection, reference reads, error paths and live server cancellation without closing the DB.
- Limits: all key summaries are fetched; full inspection copies/hashes state; detached lookup scans records. No file locking or automatic polling. Browser UI automation unavailable in this session; frontend compilation and HTTP integration are verified.

Studio verification: frontend TypeScript/Vite build, `go test ./... -count=1`, `go vet ./...`, `go build ./...`, and real CLI HTTP smoke passed. CLI smoke confirmed embedded assets and record inspection, with snapshot/WAL SHA-256 hashes unchanged.

## Source formatting

Studio TS/TSX/CSS/HTML uses Prettier formatting (2-space indentation, single quotes, no semicolons, 100-column print width). Go source uses gofmt. Keep source readable; generated studio/dist assets remain minified.

## Studio CLI directory discovery

- Canonical installed command: `dora studio`. Install from checkout with `go install ./cli/cmd/dora`; ensure GOBIN (or GOPATH/bin) is on PATH. Published install path: github.com/MehulxBuilds/dora/cli/cmd/dora@latest.
- Command logic lives in cli/cmd (package command). The sole entry point, cli/cmd/dora/main.go, calls command.Run and handles process exit. Development uses `go run ./cli/cmd/dora studio`; there is no duplicate cli/main.go entry point.
- Default discovery root: <current-working-directory>/dora. `--path <directory>` accepts absolute or relative overrides. Optional positional files remain supported for compatibility, but cannot be combined with --path.
- Discovery recursively includes .db/.json/.dora snapshots and WAL-backed filenames, deduplicates snapshot/WAL pairs, skips symlink entries and temporary files, validates candidate databases, sorts relative display names, and errors without creating missing/empty directories. Restart to discover added files.
- server.Options.Databases supplies catalog sources. GET /api/databases lists names; db=<catalog-name> selects a source for overview, record and lookup. Unknown names return 404; request parameters cannot open arbitrary filesystem paths.
- React shows a database selector for multiple sources and resets filters, selections and lookup state when switching. Record/reference/lookup requests stay scoped to the selected database.
- Tests cover default/custom/nested/WAL-only discovery, deduplication, errors and argument handling, catalog routing, and rejection of arbitrary database paths.

Directory discovery verification: frontend build, Go tests/vet/build and CLI smoke passed. The compiled dora executable was run from a separate application root using both default dora/ discovery and an absolute --path override; database selection worked and all file hashes remained unchanged.

Studio styling uses Tailwind utilities and shadcn components, with self-hosted Geist Sans throughout (including JSON). Theme CSS contains tokens and base layers only; bespoke application CSS classes are removed.

## Studio dark interface

Studio always uses dark mode, including initial HTML and native control color scheme. The interface uses Tailwind utilities and shadcn controls, with self-hosted Geist Sans for every label and JSON value. No application CSS classes or mono fonts are used. The layout has a compact desktop namespace sidebar, mobile database/namespace selectors, consistent icon sizing, structured tables, and a keyboard-dismissable record inspector.

Database, tab, namespace, key search, pagination, index selection and record inspection are reflected in the URL. Studio handles empty/invalid API responses with connection guidance. In frontend development, start the Go backend on port 7432 (`go run ./cli/cmd/dora studio --no-open`) before Vite; Vite alone cannot read database files. Restart the Go CLI after rebuilding the embedded frontend.

Validation: `cd studio && npm run build`; `node --test --test-isolation=none tests/read-api.test.mjs` checks successful reads and malformed/offline API errors. Browser rendering and keyboard interaction still require an actual browser check.

Keep generated shadcn components in `studio/src/components/ui/` unchanged. Apply Studio-specific styling through component props, Tailwind utilities, or wrappers in `components/studio/`. TypeScript checks the application from `src/main.tsx` and its imports.

Studio uses a viewport-height shell: the top bar and desktop sidebar remain in place while the main content pane scrolls independently. Long namespace lists scroll inside the sidebar without moving its header or footer. Default shadcn components remain unchanged.
