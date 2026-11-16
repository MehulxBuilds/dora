package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const metaIndexesKey = "\x00_indexes"

// IndexDefinition selects one literal top-level field from keys matching Prefix.
// An empty prefix includes all keys. Field names are not parsed as paths.
type IndexDefinition struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
	Field  string `json:"field"`
}

type indexToken struct {
	typ  ValueType
	data interface{}
}
type secondaryIndex struct {
	definition IndexDefinition
	buckets    map[indexToken]map[Key]struct{}
}

func scalarToken(v Value) (indexToken, bool) {
	switch v.typ {
	case TypeNull, TypeBoolean, TypeString, TypeReference:
		return indexToken{v.typ, v.data}, true
	case TypeNumber:
		n := v.data.(float64)
		if !math.IsNaN(n) && !math.IsInf(n, 0) {
			return indexToken{v.typ, n}, true
		}
	}
	return indexToken{}, false
}

func (idx *secondaryIndex) token(key Key, v Value) (indexToken, bool) {
	if !strings.HasPrefix(key, idx.definition.Prefix) || v.typ != TypeObject {
		return indexToken{}, false
	}
	field, ok := v.data.(map[string]Value)[idx.definition.Field]
	if !ok {
		return indexToken{}, false
	}
	return scalarToken(field)
}

func (idx *secondaryIndex) add(key Key, v Value) {
	if token, ok := idx.token(key, v); ok {
		if idx.buckets[token] == nil {
			idx.buckets[token] = make(map[Key]struct{})
		}
		idx.buckets[token][key] = struct{}{}
	}
}
func (idx *secondaryIndex) remove(key Key, v Value) {
	if token, ok := idx.token(key, v); ok {
		delete(idx.buckets[token], key)
		if len(idx.buckets[token]) == 0 {
			delete(idx.buckets, token)
		}
	}
}

func validIndex(d IndexDefinition) bool { return d.Name != "" && d.Field != "" }

// CreateIndex builds a non-unique exact-match index over existing and future data.
// It returns ErrIndexExists for a duplicate name and ErrIndexInvalid for an empty
// name or field. Index creation increments Seq but does not affect StateHash.
func (db *DB) CreateIndex(name, prefix, field string) error {
	return db.createIndex(name, prefix, field, false)
}

// EnsureIndex creates an index if absent, or succeeds without writing when its
// existing definition matches. A different prefix or field returns ErrIndexConflict.
// Use EnsureIndex for startup initialization that runs after every restart.
func (db *DB) EnsureIndex(name, prefix, field string) error {
	return db.createIndex(name, prefix, field, true)
}

func (db *DB) createIndex(name, prefix, field string, ensure bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return ErrClosed
	}
	d := IndexDefinition{Name: name, Prefix: prefix, Field: field}
	if !validIndex(d) {
		return ErrIndexInvalid
	}
	if idx, ok := db.state.indexes[name]; ok {
		if ensure {
			if idx.definition == d {
				return nil
			}
			return fmt.Errorf("index %q has a different prefix or field: %w", name, ErrIndexConflict)
		}
		return ErrIndexExists
	}
	op := Operation{Type: OpCreateIndex, Index: d}
	if db.wal != nil {
		if err := db.wal.append(op); err != nil {
			return err
		}
	}
	return db.state.apply(op)
}

// DropIndex removes an index definition and its derived buckets, not its records.
func (db *DB) DropIndex(name string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return ErrClosed
	}
	if _, ok := db.state.indexes[name]; !ok {
		return ErrIndexNotFound
	}
	op := Operation{Type: OpDropIndex, Index: IndexDefinition{Name: name}}
	if db.wal != nil {
		if err := db.wal.append(op); err != nil {
			return err
		}
	}
	return db.state.apply(op)
}

func sortedIndexKeys(keys map[Key]struct{}) []Key {
	result := make([]Key, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

// Lookup returns sorted matching keys. Arrays, objects, and non-finite numbers
// return ErrIndexValue. Missing fields are excluded; explicit null is indexed.
// Reference values match their target key without following the reference.
func (db *DB) Lookup(name string, value Value) ([]Key, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil, ErrClosed
	}
	idx, ok := db.state.indexes[name]
	if !ok {
		return nil, ErrIndexNotFound
	}
	token, ok := scalarToken(value)
	if !ok {
		return nil, ErrIndexValue
	}
	return sortedIndexKeys(idx.buckets[token]), nil
}

// Lookup merges committed index candidates with the transaction's current overlay.
// Use this instead of DB.Lookup inside a transaction to avoid deadlock.
func (tx *Tx) Lookup(name string, value Value) ([]Key, error) {
	idx, ok := tx.db.state.indexes[name]
	if !ok {
		return nil, ErrIndexNotFound
	}
	token, ok := scalarToken(value)
	if !ok {
		return nil, ErrIndexValue
	}
	keys := make(map[Key]struct{}, len(idx.buckets[token]))
	for key := range idx.buckets[token] {
		keys[key] = struct{}{}
	}
	for key, entry := range tx.overlay {
		delete(keys, key)
		if !entry.deleted {
			if candidate, ok := idx.token(key, entry.value); ok && candidate == token {
				keys[key] = struct{}{}
			}
		}
	}
	return sortedIndexKeys(keys), nil
}

func (s *state) indexSnapshot() Value {
	definitions := make(map[string]Value, len(s.indexes))
	for name, idx := range s.indexes {
		definitions[name] = NewObject(map[string]Value{"prefix": NewString(idx.definition.Prefix), "field": NewString(idx.definition.Field)})
	}
	return NewObject(definitions)
}

func (s *state) loadIndexes(v Value) error {
	definitions, ok := v.Object()
	if !ok {
		return fmt.Errorf("index metadata: %w", ErrIndexInvalid)
	}
	names := make([]string, 0, len(definitions))
	for name := range definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	seq := s.seq
	defer func() { s.seq = seq }()
	for _, name := range names {
		fields, ok := definitions[name].Object()
		if !ok {
			return ErrIndexInvalid
		}
		prefix, p := fields["prefix"].String()
		field, f := fields["field"].String()
		if !p || !f {
			return ErrIndexInvalid
		}
		if err := s.apply(Operation{Type: OpCreateIndex, Index: IndexDefinition{Name: name, Prefix: prefix, Field: field}}); err != nil {
			return err
		}
	}
	return nil
}
