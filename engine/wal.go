package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// walEntry is the on-disk representation of a single WAL record.
// Each entry is stored as one JSON line in the WAL file.
//
// The Type field uses the human-readable string form ("SET", "DELETE", "TX")
// rather than the numeric OpType, keeping the WAL inspectable with any
// text editor or command-line tool.
//
// For individual operations (SET, DELETE): Key and Value are populated.
// For transactions (TX): Ops contains the list of sub-operations as a
// single atomic group. Key and Value are empty.
type walEntry struct {
	Type  string     `json:"type"`
	Key   Key        `json:"key,omitempty"`
	Value *Value     `json:"value,omitempty"` // present for SET, absent for DELETE/TX
	Ops   []walEntry `json:"ops,omitempty"`   // present for TX, absent for SET/DELETE
}

// wal is the write-ahead log for the database engine.
//
// Every mutation is appended to the WAL before being applied to the
// in-memory state. This guarantees that committed operations survive
// process crashes. On recovery (Open), the WAL is replayed against the
// last snapshot to rebuild the current state.
//
// The WAL uses JSON Lines format: one JSON object per line, terminated by
// a newline character. This format is simple, human-readable, and allows
// each entry to be written and fsynced independently.
//
// Lifecycle:
//
//	Open:   load snapshot → replay WAL → open WAL for appending
//	Write:  append entry → fsync → apply to state
//	Close:  save snapshot → truncate WAL → close
type wal struct {
	path string
	file *os.File
}

// openWAL opens or creates the WAL file for appending.
// The file is created with 0644 permissions if it does not exist.
func openWAL(path string) (*wal, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open WAL: %w", err)
	}
	return &wal{path: path, file: f}, nil
}

// append writes an operation to the WAL and fsyncs it to disk.
//
// After this method returns nil, the operation is durable — it will
// survive a process crash. The caller should only apply the operation
// to the in-memory state after a successful append.
//
// Each entry is written as a single JSON line followed by a newline,
// then fsynced. This ensures that either the entire entry is on disk
// or none of it is. Partial writes (crash mid-write) may leave an
// incomplete final line, which recovery can detect and skip.
func (w *wal) append(op Operation) error {
	entry := walEntry{
		Type: op.Type.String(),
		Key:  op.Key,
	}
	if op.Type == OpSet {
		entry.Value = &op.Value
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("WAL marshal: %w", err)
	}
	data = append(data, '\n')

	if _, err := w.file.Write(data); err != nil {
		return fmt.Errorf("WAL write: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("WAL sync: %w", err)
	}
	return nil
}

// appendBatch writes a group of operations to the WAL as a single atomic
// entry (type "TX") and fsyncs it to disk.
//
// This is the WAL mechanism for transaction atomicity. The entire group
// is serialized as one JSON line:
//
//	{"type":"TX","ops":[{"type":"SET","key":"a","value":1},{"type":"DELETE","key":"b"}]}
//
// Because it's a single write+fsync, either the entire transaction is
// durable or none of it is. If the process crashes mid-write, recovery
// will see an incomplete JSON line and discard it — no partial commits.
func (w *wal) appendBatch(ops []Operation) error {
	sub := make([]walEntry, len(ops))
	for i, op := range ops {
		sub[i] = walEntry{
			Type: op.Type.String(),
			Key:  op.Key,
		}
		if op.Type == OpSet {
			sub[i].Value = &op.Value
		}
	}

	tx := walEntry{Type: "TX", Ops: sub}

	data, err := json.Marshal(tx)
	if err != nil {
		return fmt.Errorf("WAL marshal tx: %w", err)
	}
	data = append(data, '\n')

	if _, err := w.file.Write(data); err != nil {
		return fmt.Errorf("WAL write tx: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("WAL sync tx: %w", err)
	}
	return nil
}

// loadWALEntries reads all valid operations from an existing WAL file.
//
// Returns an empty slice if the WAL file does not exist (normal case
// for a fresh database or after a clean shutdown that truncated the WAL).
//
// Recovery semantics: entries are parsed sequentially. If a corrupt or
// incomplete entry is encountered (the expected result of a crash
// mid-write), parsing stops at that point. All preceding entries are
// valid — they were individually fsynced before the next operation began.
//
// When a partial entry is detected, it is removed from the WAL file by
// truncating the file to the end of the last valid entry. This ensures
// subsequent appends (after recovery) do not write after garbage bytes,
// which would cause data loss on the next recovery cycle.
//
// The valid entries are NOT removed. They remain in the WAL until
// Close() saves a snapshot and truncates the WAL. This protects against
// a second crash before Close(): the entries can be replayed again.
func loadWALEntries(path string) ([]Operation, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read WAL: %w", err)
	}

	if len(content) == 0 {
		return nil, nil
	}

	var ops []Operation
	pos := 0       // current read position in content
	validEnd := 0  // byte offset past the last valid entry (for truncation)
	needsRepair := false

	for pos < len(content) {
		// Find the next newline to extract a line.
		lineEnd := pos
		for lineEnd < len(content) && content[lineEnd] != '\n' {
			lineEnd++
		}

		line := content[pos:lineEnd]

		// Advance past the newline (if present).
		nextPos := lineEnd
		if nextPos < len(content) {
			nextPos++
		}

		// Skip empty lines.
		if len(line) == 0 {
			pos = nextPos
			validEnd = pos
			continue
		}

		// Attempt to parse the entry.
		var entry walEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Partial or corrupt entry. With fsync-per-entry, only the
			// last entry can be incomplete (crash mid-write). All
			// preceding entries were fully synced. Stop here.
			needsRepair = true
			break
		}

		op, err := entryToOperation(entry)
		if err != nil {
			// Structurally valid JSON but semantically invalid (e.g.,
			// unknown operation type, SET without value). This is real
			// corruption, not a partial write.
			return nil, err
		}

		ops = append(ops, op...)
		pos = nextPos
		validEnd = pos
	}

	// If a partial entry was found, truncate the file to remove it.
	// The valid entries remain intact for crash safety.
	if needsRepair {
		if err := os.Truncate(path, int64(validEnd)); err != nil {
			return nil, fmt.Errorf("WAL repair: %w", err)
		}
	}

	return ops, nil
}

// entryToOperation converts an on-disk walEntry into one or more Operations.
//
// For SET and DELETE entries, returns a single-element slice.
// For TX entries, recursively unpacks all sub-operations into a flat slice,
// preserving the original operation order.
func entryToOperation(e walEntry) ([]Operation, error) {
	switch e.Type {
	case "SET":
		if e.Value == nil {
			return nil, fmt.Errorf("WAL SET entry missing value for key %q", e.Key)
		}
		return []Operation{{Type: OpSet, Key: e.Key, Value: *e.Value}}, nil

	case "DELETE":
		return []Operation{{Type: OpDelete, Key: e.Key}}, nil

	case "TX":
		var ops []Operation
		for _, sub := range e.Ops {
			subOps, err := entryToOperation(sub)
			if err != nil {
				return nil, fmt.Errorf("WAL TX sub-entry: %w", err)
			}
			ops = append(ops, subOps...)
		}
		return ops, nil

	default:
		return nil, fmt.Errorf("WAL unknown operation type: %q", e.Type)
	}
}

// truncate clears the WAL file, resetting it to empty.
//
// Called after a successful snapshot save. At that point, all WAL entries
// are already captured in the snapshot, so the log can be safely discarded.
func (w *wal) truncate() error {
	if err := w.file.Truncate(0); err != nil {
		return fmt.Errorf("WAL truncate: %w", err)
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("WAL seek: %w", err)
	}
	return nil
}

// close closes the WAL file handle.
func (w *wal) close() error {
	return w.file.Close()
}
