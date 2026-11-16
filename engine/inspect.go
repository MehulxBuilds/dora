package engine

import (
	"fmt"
	"os"
	"sort"
)

// Inspection is a detached view. Modifying its maps or slices cannot change a DB.
// It exposes no write, checkpoint, or file repair operations.
type Inspection struct {
	Records map[Key]Value
	Indexes []IndexDefinition
	Seq     uint64
	Hash    [32]byte
}

// Inspect captures data, index definitions, sequence and hash under one read lock.
func (db *DB) Inspect() (*Inspection, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil, ErrClosed
	}
	return inspectState(db.state)
}

func inspectState(s *state) (*Inspection, error) {
	hash, err := s.hash()
	if err != nil {
		return nil, err
	}
	indexes := make([]IndexDefinition, 0, len(s.indexes))
	for _, idx := range s.indexes {
		indexes = append(indexes, idx.definition)
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].Name < indexes[j].Name })
	return &Inspection{Records: s.snapshot(), Indexes: indexes, Seq: s.seq, Hash: hash}, nil
}

// InspectFile reads an exact filesystem path (no implicit dora/ prefix).
// The owning application must be stopped: Dora does not yet have file locking.
// Snapshot and WAL files are never created, repaired, truncated, or checkpointed.
// An incomplete WAL tail is ignored in memory and left untouched on disk.
func InspectFile(path string) (*Inspection, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if _, walErr := os.Stat(path + ".wal"); walErr != nil {
			return nil, fmt.Errorf("database not found: %w", err)
		}
	}
	data, err := newJSONStorage(path).Load()
	if err != nil {
		return nil, err
	}
	var seq uint64
	if v, ok := data[metaSeqKey]; ok {
		if n, ok := v.Number(); ok {
			seq = uint64(n)
		}
		delete(data, metaSeqKey)
	}
	meta, hasMeta := data[metaIndexesKey]
	delete(data, metaIndexesKey)
	s := newStateFrom(data)
	s.seq = seq
	if hasMeta {
		if err := s.loadIndexes(meta); err != nil {
			return nil, err
		}
	}
	ops, err := readWALEntries(path+".wal", false)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		if err := s.apply(op); err != nil && err != ErrKeyNotFound {
			return nil, err
		}
	}
	if err := validateReferences(s.data, nil); err != nil {
		return nil, err
	}
	return inspectState(s)
}

// Lookup performs exact matching in this detached view without changing indexes.
func (view *Inspection) Lookup(name string, value Value) ([]Key, error) {
	var definition *IndexDefinition
	for i := range view.Indexes {
		if view.Indexes[i].Name == name {
			definition = &view.Indexes[i]
			break
		}
	}
	if definition == nil {
		return nil, ErrIndexNotFound
	}
	token, ok := scalarToken(value)
	if !ok {
		return nil, ErrIndexValue
	}
	idx := secondaryIndex{definition: *definition}
	keys := make([]Key, 0)
	for key, record := range view.Records {
		if t, ok := idx.token(key, record); ok && t == token {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}
