package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// walEntry is the on-disk representation of a single WAL record.
// Each entry is stored as one JSON line in the WAL file.
//
// The Type field uses the human-readable string form ("SET", "DELETE")
// rather than the numeric OpType, keeping the WAL inspectable with any
// text editor or command-line tool.
type walEntry struct {
	Type  string `json:"type"`
	Key   Key    `json:"key"`
	Value *Value `json:"value,omitempty"` // present for SET, absent for DELETE
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

// loadWALEntries reads all operations from an existing WAL file.
//
// Returns an empty slice if the WAL file does not exist (normal case
// for a fresh database or after a clean shutdown that truncated the WAL).
//
// Returns an error if the file exists but contains corrupt entries.
// Partial/incomplete lines (from crash mid-write) are not yet handled
// here — that is a Phase 5 concern.
func loadWALEntries(path string) ([]Operation, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read WAL: %w", err)
	}
	defer f.Close()

	var ops []Operation
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue // skip blank lines
		}

		var entry walEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("WAL corrupt entry: %w", err)
		}

		op, err := entryToOperation(entry)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("WAL scan: %w", err)
	}
	return ops, nil
}

// entryToOperation converts an on-disk walEntry back into an Operation.
func entryToOperation(e walEntry) (Operation, error) {
	var op Operation
	op.Key = e.Key

	switch e.Type {
	case "SET":
		op.Type = OpSet
		if e.Value == nil {
			return op, fmt.Errorf("WAL SET entry missing value for key %q", e.Key)
		}
		op.Value = *e.Value
	case "DELETE":
		op.Type = OpDelete
	default:
		return op, fmt.Errorf("WAL unknown operation type: %q", e.Type)
	}
	return op, nil
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
