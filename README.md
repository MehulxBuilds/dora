# Dora

An embedded key-value database engine for Go. Think SQLite, but for JSON/key-value data instead of SQL tables.

No server. No network. No Docker. Just a Go library you import into your application.

```go
db, _ := engine.Open("app.db")
defer db.Close()

db.Set("user:1", engine.NewObject(map[string]engine.Value{
    "name": engine.NewString("Mehul"),
    "age":  engine.NewNumber(20),
}))

val, _ := db.Get("user:1")
```

Your data lives in a `dora/` directory right next to your application -- like `prisma/` or `drizzle/`, but for your database. Open it, read it, write it, close it. That's it.

---

## Install

```bash
go get github.com/MehulxBuilds/dora
```

```go
import "github.com/MehulxBuilds/dora/engine"
```

Requires Go 1.24+. Zero external dependencies -- only the Go standard library.

---

## Quick Start

### Open a database

```go
// Persistent -- files created at dora/myapp.db and dora/myapp.db.wal
db, err := engine.Open("myapp.db")

// In-memory -- no files, data lost when process exits
db, err := engine.Open("")
```

`Open("")` gives you a fast in-memory database. `Open("myapp.db")` gives you a persistent database. All files are automatically placed inside a `dora/` directory (created if it doesn't exist).

### Store and retrieve values

```go
// Store a string
db.Set("greeting", engine.NewString("hello world"))

// Store a number
db.Set("counter", engine.NewNumber(42))

// Store a JSON-like object
db.Set("user:1", engine.NewObject(map[string]engine.Value{
    "name":  engine.NewString("Mehul"),
    "age":   engine.NewNumber(20),
    "admin": engine.NewBoolean(true),
}))

// Read it back
val, err := db.Get("user:1")
if err != nil {
    // engine.ErrKeyNotFound if the key doesn't exist
}

obj, _ := val.Object()
name, _ := obj["name"].String()
fmt.Println(name) // "Mehul"
```

### Delete keys

```go
err := db.Delete("user:1")
// Returns engine.ErrKeyNotFound if the key doesn't exist
```

### Check if a key exists

```go
if db.Exists("user:1") {
    // ...
}
```

### Count keys

```go
fmt.Println(db.Len()) // number of keys in the database
```

### Close the database

```go
err := db.Close()
```

Always call `Close()` when you're done. For persistent databases, this writes the final state to disk. If you forget, individual operations are still safe in the WAL (explained below), but the snapshot file won't be up to date.

---

## Value Types

Dora doesn't store raw bytes or plain strings. It has a typed value system that mirrors JSON:

| Type | Constructor | Example |
|------|-------------|---------|
| Null | `NewNull()` | `null` |
| Boolean | `NewBoolean(true)` | `true`, `false` |
| Number | `NewNumber(3.14)` | `42`, `3.14`, `-1` |
| String | `NewString("hello")` | `"hello world"` |
| Array | `NewArray([]Value{...})` | `[1, "two", true]` |
| Object | `NewObject(map[string]Value{...})` | `{"name": "Mehul"}` |

### Reading values back

Every accessor returns the value and a boolean indicating whether the type matched:

```go
val, _ := db.Get("counter")

n, ok := val.Number()   // n = 42.0, ok = true
s, ok := val.String()   // s = "", ok = false (it's a Number, not a String)

if val.IsNull() {
    // handle null
}
```

### Nested values

Values can be nested to any depth:

```go
db.Set("config", engine.NewObject(map[string]engine.Value{
    "server": engine.NewObject(map[string]engine.Value{
        "host": engine.NewString("localhost"),
        "port": engine.NewNumber(8080),
        "tags": engine.NewArray([]engine.Value{
            engine.NewString("production"),
            engine.NewString("us-east"),
        }),
    }),
}))
```

### Values are immutable

Once you create a value, it can't be changed. Constructors copy their input (slices and maps), and accessors return copies. This means you can safely pass values between goroutines without worrying about data races.

---

## Prefix Queries

Keys are just strings. You can use any naming convention you want, but a common pattern is `type:id`:

```
user:1
user:2
user:3
post:1
post:2
settings:theme
```

Dora gives you two ways to query by prefix:

### Keys with a prefix

```go
// Get all keys that start with "user:"
userKeys := db.Keys("user:")
// ["user:1", "user:2", "user:3"]

// Get all keys (no prefix filter)
allKeys := db.Keys()
```

### Key-value pairs with a prefix

```go
// Get all users as a map
users := db.Prefix("user:")
// map[Key]Value{
//     "user:1": ...,
//     "user:2": ...,
//     "user:3": ...,
// }

for key, val := range users {
    obj, _ := val.Object()
    name, _ := obj["name"].String()
    fmt.Printf("%s: %s\n", key, name)
}
```

The returned map is a copy. You can read and iterate it freely without affecting the database.

---

## Transactions

A transaction groups multiple operations into a single atomic unit. Either all operations succeed, or none of them do.

```go
err := db.Transaction(func(tx *engine.Tx) error {
    tx.Set("user:1", engine.NewObject(map[string]engine.Value{
        "name":    engine.NewString("Mehul"),
        "balance": engine.NewNumber(100),
    }))

    tx.Set("user:2", engine.NewObject(map[string]engine.Value{
        "name":    engine.NewString("Alice"),
        "balance": engine.NewNumber(200),
    }))

    return nil // commit -- both users are saved
})
```

### Rollback

If the function returns an error, everything is discarded:

```go
err := db.Transaction(func(tx *engine.Tx) error {
    tx.Set("user:1", engine.NewObject(map[string]engine.Value{
        "name": engine.NewString("Mehul"),
    }))

    // Something went wrong
    return fmt.Errorf("validation failed")
    // user:1 is NOT saved -- rollback
})
```

### Reading your own writes

Inside a transaction, you can read values you just wrote:

```go
db.Transaction(func(tx *engine.Tx) error {
    tx.Set("counter", engine.NewNumber(1))

    val, _ := tx.Get("counter")
    n, _ := val.Number()
    // n == 1 -- you see your own uncommitted write

    tx.Set("counter", engine.NewNumber(n + 1))
    return nil
})
```

### Important: use `tx`, not `db`

Inside the transaction function, always use `tx.Set`, `tx.Get`, `tx.Delete`, `tx.Exists`. Do NOT call `db.Set`, `db.Get`, etc. -- this will deadlock because the transaction holds an exclusive lock.

---

## Persistence and Durability

When you open a database with a file path, Dora creates two files inside the `dora/` directory:

| File | Purpose |
|------|---------|
| `dora/app.db` | Snapshot -- the full database state as human-readable JSON |
| `dora/app.db.wal` | Write-ahead log -- a journal of recent operations |

### How writes work

When you call `db.Set(...)`:

1. The operation is written to the WAL file and fsynced to disk.
2. The operation is applied to the in-memory state.

The write-ahead log is the key to durability. Once step 1 completes, the operation survives a crash -- even if the process dies before step 2. On the next `Open()`, the WAL is replayed to recover any operations that weren't captured in the snapshot.

### How the snapshot file works

The snapshot file (`dora/app.db`) is a point-in-time copy of the entire database, written as pretty-printed JSON:

```json
{
  "user:1": {
    "name": "Mehul",
    "age": 20
  },
  "counter": 42
}
```

The snapshot is updated when you call `Close()` or `Snapshot()`. Between those calls, the WAL holds the operations that are ahead of the snapshot.

### Crash recovery

If your process crashes:

```
1. Open("app.db")
2. Load the snapshot (dora/app.db)
3. Replay the WAL (dora/app.db.wal) -- apply any operations added since the last snapshot
4. Database is recovered to the exact state before the crash
```

At most one operation can be lost -- the one that was being written at the exact moment of the crash. All operations that completed (fsync returned successfully) are guaranteed to survive.

### Snapshots

For long-running applications, the WAL can grow large. Call `Snapshot()` periodically to write the current state and clear the WAL:

```go
// Take a snapshot every 5 minutes
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        db.Snapshot()
    }
}()
```

After a snapshot, recovery is instant -- the WAL is empty and the snapshot file has everything.

---

## Concurrency

Dora is safe to use from multiple goroutines.

- **Reads** (`Get`, `Exists`, `Len`, `Keys`, `Prefix`, `Seq`, `StateHash`) can run concurrently with each other.
- **Writes** (`Set`, `Delete`) are exclusive -- only one write at a time.
- **Transactions** hold an exclusive lock for their entire duration.

```go
// Safe -- multiple goroutines can read concurrently
go func() { db.Get("key") }()
go func() { db.Get("key") }()
go func() { db.Exists("key") }()

// Safe -- writes are serialized automatically
go func() { db.Set("a", engine.NewNumber(1)) }()
go func() { db.Set("b", engine.NewNumber(2)) }()
```

You don't need to add your own locks. The database handles all synchronization internally.

### After Close

Once you call `Close()`, all methods return errors (or zero values):

```go
db.Close()

err := db.Set("key", engine.NewString("value"))
// err == engine.ErrClosed

_, err = db.Get("key")
// err == engine.ErrClosed

db.Exists("key")  // false
db.Len()          // 0
db.Keys()         // nil
```

---

## Deterministic Verification

Every mutation increments a sequence number. You can use this, combined with the state hash, to verify that two databases are in the same state.

### Sequence number

```go
db.Set("a", engine.NewNumber(1))  // seq = 1
db.Set("b", engine.NewNumber(2))  // seq = 2
db.Delete("a")                     // seq = 3

fmt.Println(db.Seq()) // 3
```

The sequence number is persisted across restarts.

### State hash

```go
hash, err := db.StateHash()
// hash is a [32]byte -- SHA-256 of the entire database contents
```

The hash is deterministic: two databases with the same key-value data always produce the same hash, regardless of how they got there. The hash does not include the sequence number -- it's purely a function of the data.

### Verification example

```go
db1, _ := engine.Open("")
db2, _ := engine.Open("")

db1.Set("x", engine.NewNumber(1))
db1.Set("y", engine.NewNumber(2))

db2.Set("x", engine.NewNumber(1))
db2.Set("y", engine.NewNumber(2))

h1, _ := db1.StateHash()
h2, _ := db2.StateHash()

fmt.Println(h1 == h2)           // true
fmt.Println(db1.Seq() == db2.Seq()) // true
```

Same operations in the same order produce the same sequence number and the same state hash. This is the core determinism guarantee.

---

## Errors

Dora uses sentinel errors that you can check with `errors.Is`:

```go
import "errors"

val, err := db.Get("nonexistent")
if errors.Is(err, engine.ErrKeyNotFound) {
    // key doesn't exist
}

err = db.Set("", engine.NewString("value"))
if errors.Is(err, engine.ErrKeyEmpty) {
    // empty keys are not allowed
}

db.Close()
err = db.Set("key", engine.NewString("value"))
if errors.Is(err, engine.ErrClosed) {
    // database is closed
}
```

| Error | When |
|-------|------|
| `ErrKeyNotFound` | `Get` or `Delete` on a key that doesn't exist |
| `ErrKeyEmpty` | Any operation with an empty string key |
| `ErrClosed` | Any operation after `Close()` has been called |

---

## Complete API Reference

### Database lifecycle

```go
func Open(path string) (*DB, error)   // open or create a database
func (db *DB) Close() error           // save state and close
func (db *DB) Snapshot() error         // save state without closing
```

### Key-value operations

```go
func (db *DB) Set(key Key, val Value) error     // create or overwrite
func (db *DB) Get(key Key) (Value, error)        // read a value
func (db *DB) Delete(key Key) error              // remove a key
func (db *DB) Exists(key Key) bool               // check existence
func (db *DB) Len() int                          // count keys
func (db *DB) Keys(prefix ...string) []Key       // list keys (optional prefix filter)
func (db *DB) Prefix(prefix string) map[Key]Value // key-value pairs by prefix
```

### Transactions

```go
func (db *DB) Transaction(fn func(tx *Tx) error) error

func (tx *Tx) Set(key Key, val Value) error
func (tx *Tx) Get(key Key) (Value, error)
func (tx *Tx) Delete(key Key) error
func (tx *Tx) Exists(key Key) bool
```

### Verification

```go
func (db *DB) Seq() uint64                     // current operation sequence number
func (db *DB) StateHash() ([32]byte, error)    // SHA-256 of all data
```

### Value constructors

```go
func NewNull() Value
func NewBoolean(b bool) Value
func NewNumber(n float64) Value
func NewString(s string) Value
func NewArray(items []Value) Value
func NewObject(fields map[string]Value) Value
```

### Value accessors

```go
func (v Value) Type() ValueType
func (v Value) IsNull() bool
func (v Value) Boolean() (bool, bool)
func (v Value) Number() (float64, bool)
func (v Value) String() (string, bool)
func (v Value) Array() ([]Value, bool)
func (v Value) Object() (map[string]Value, bool)
```

---

## Architecture

```
Your Go Application
        |
   engine.Open("app.db")
        |
        v
   +----DB----+          (public API, locking)
   |          |
   |  state   |          (in-memory key-value map)
   |          |
   +----+-----+
        |
   +----+-----+-----+
   |          |      |
  WAL     Storage   Tx
   |          |
   dora/              (all files live here)
    |- app.db.wal
    |- app.db
```

### How the layers fit together

**DB** is the public API. It holds the lock, validates input, coordinates the WAL and storage, and delegates to the state.

**State** is a plain Go map (`map[Key]Value`) with a sequence counter. It has no locks, no I/O, no side effects. It's a pure data structure. All reads and writes go through the DB, which holds the appropriate lock.

**WAL** (write-ahead log) is a file where every mutation is recorded before it's applied to state. This is what makes the database durable. The WAL uses JSON Lines format -- one JSON object per line, human-readable, easy to inspect:

```
{"type":"SET","key":"user:1","value":{"name":"Mehul","age":20}}
{"type":"DELETE","key":"temp:session"}
{"type":"TX","ops":[{"type":"SET","key":"a","value":1},{"type":"SET","key":"b","value":2}]}
```

**Storage** is an interface with `Load`, `Save`, and `Close` methods. The default implementation (`jsonStorage`) writes human-readable JSON files with atomic writes (write to temp file, then rename). The interface is there so a future binary storage engine can be swapped in without changing the rest of the codebase.

**Tx** (transaction) buffers mutations in an overlay map. On commit, all operations are written to the WAL as a single atomic entry and then applied to state. On rollback, the overlay is discarded.

### The operation model

Every mutation -- whether from `db.Set`, `db.Delete`, or a transaction -- goes through the same path:

```
Validate key
     |
Create Operation (SET or DELETE)
     |
Write to WAL (fsync)      <-- durable after this point
     |
Apply to state             <-- visible after this point
     |
Increment sequence number
```

This is why the database is a **deterministic state machine**. The same sequence of operations, starting from the same initial state, always produces the same final state and the same state hash.

---

## In-Memory vs Persistent

| | In-Memory | Persistent |
|---|---|---|
| Open | `Open("")` | `Open("app.db")` |
| Files | None | `dora/app.db` + `dora/app.db.wal` |
| Snapshot | No-op | Writes `dora/app.db` |
| Speed | Fastest | Slightly slower (disk fsync) |
| Survives restart | No | Yes |
| Use case | Tests, caches, temp data | Real applications |

Both modes have the same API. You can develop with in-memory and switch to persistent by changing one string.

---

## File Format

All database files live inside the `dora/` directory:

```
your-project/
  dora/
    app.db          <-- snapshot (human-readable JSON)
    app.db.wal      <-- write-ahead log (JSON Lines)
  main.go
  go.mod
```

The snapshot file is valid, human-readable JSON. You can open it in any text editor:

```json
{
  "counter": 42,
  "user:1": {
    "age": 20,
    "name": "Mehul"
  }
}
```

The WAL file uses JSON Lines (one JSON object per line):

```
{"type":"SET","key":"counter","value":42}
{"type":"SET","key":"user:1","value":{"age":20,"name":"Mehul"}}
```

Both files are designed to be inspected and debugged by humans. A future binary format may be added for performance, but the JSON format will remain as an option.

---

## References / Relations

Run `go run .` to try the examples in `main.go`. The reference examples use the persistent demo database and demonstrate object/array references, forward references, following a target, integrity errors, and deleting related records in one transaction. The basic persistence example also writes `dora/demo.db`.

Use `engine.NewReference(key)` to store a typed reference, including inside an object or array. `Value.Reference()` returns `(targetKey, true)` for reference values. `Get` returns the stored value; follow the target explicitly with another `Get`.

```go
err := db.Transaction(func(tx *engine.Tx) error {
    // Forward references are checked against the final transaction state.
    if err := tx.Set("post:1", engine.NewObject(map[string]engine.Value{
        "author": engine.NewReference("user:1"),
    })); err != nil {
        return err
    }
    return tx.Set("user:1", engine.NewString("Mehul"))
})
if err != nil {
    return err
}
```

Missing or empty targets produce `engine.ErrReferenceNotFound`. Deleting a key that remains referenced produces `engine.ErrKeyReferenced`. Check errors with `errors.Is`. Rejected changes leave state, sequence and WAL unchanged. Remove references or delete related keys together in a transaction; there are no cascading deletes. Self-references and cycles are supported.

References survive WAL recovery and snapshots. Their JSON encoding is `{"\u0000_ref":"user:1"}`. Ordinary objects using reserved fields `\u0000_ref` or `\u0000_object` are escaped with an object wrapper. Existing hand-authored files using these exact marker shapes need migration before opening because their original intent cannot be inferred.

Integrity checks currently copy and scan the final keyspace, recursively examining values. Transactions validate once at commit, allowing temporary dangling references inside the callback. Recovery validates the final recovered state.

## Secondary indexes

Create a non-unique exact-match index on a top-level object field:

```go
if err := db.CreateIndex("users_email", "user:", "email"); err != nil {
    return err
}
keys, err := db.Lookup("users_email", engine.NewString("mehul@example.com"))
if err != nil {
    return err
}
// keys contains matching database keys, sorted lexicographically.
```

Creation indexes existing records as well as future writes. Set, overwrite, delete, and committed transactions maintain the index automatically. Multiple records may have the same value. Inside transaction callbacks, use `tx.Lookup(...)` to see uncommitted changes; calling DB methods there would deadlock.

`db.DropIndex("users_email")` removes the index while keeping the records. Run `go run .` for startup initialization, lookup, and transaction examples in `main.go`.

- The prefix filters database keys; an empty prefix includes all keys.
- Field names are literal: `"author.email"` selects a field with that exact name, not a nested path.
- Supported values: null, boolean, finite number, string, and reference. Types stay distinct; a reference matches its target key without following it. Numeric positive and negative zero match.
- Missing fields, non-object records, array/object fields, and non-finite numbers are not indexed. Explicit null is indexed.
- Lookups return sorted copies, with an empty slice when no records match. Unsupported lookup values return `ErrIndexValue`.
- Duplicate names return `ErrIndexExists`; unknown names return `ErrIndexNotFound`; empty names/fields return `ErrIndexInvalid`. Check errors with `errors.Is`. Closed DB methods return `ErrClosed`.

Index definitions survive snapshots and WAL recovery. Snapshot metadata uses reserved `\u0000_indexes`; do not use reserved metadata keys for application data. Existing files using this key as ordinary data need migration. Index buckets are derived in memory rather than persisted. Creation and drop each increment `Seq()`; `StateHash()` continues to cover only record data.

Creation scans records under the exclusive lock. Lookups read matching buckets and sort results; transaction lookups also scan the overlay. Existing reference integrity validation still scans data on writes. Unique constraints, nested paths, compound indexes, range queries, and automatic query planning are deferred.

### Initialize indexes on startup

Indexes persist across restarts. Use `EnsureIndex` in startup code:

```go
if err := db.EnsureIndex("users_email", "user:", "email"); err != nil {
    return err
}
```

It creates a missing index and reuses an existing matching definition without rebuilding, writing the WAL, or incrementing Seq. If the same name has a different prefix or field, it returns `ErrIndexConflict`; it does not silently change the index. `CreateIndex` remains strict and returns `ErrIndexExists` for duplicate names. The persistent `main.go` example uses `EnsureIndex` and keeps the index across runs.

## License

MIT