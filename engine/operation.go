package engine

import "fmt"

// OpType identifies the type of a database operation.
type OpType int

const (
	// OpSet represents a key-value write. Overwrites any existing value.
	OpSet OpType = iota

	// OpDelete represents a key removal.
	OpDelete
	OpCreateIndex
	OpDropIndex
)

// String returns a human-readable name for the OpType.
func (t OpType) String() string {
	switch t {
	case OpSet:
		return "SET"
	case OpDelete:
		return "DELETE"
	case OpCreateIndex:
		return "CREATE_INDEX"
	case OpDropIndex:
		return "DROP_INDEX"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(t))
	}
}

// Operation represents a single, deterministic state mutation.
//
// Every mutation to the database state must be expressible as an Operation.
// This is the fundamental unit of change — the building block for WAL entries,
// transaction groups, and deterministic replay.
//
// For OpSet, Key and Value are both required.
// For OpDelete, only Key is required; Value is ignored.
type Operation struct {
	Index IndexDefinition // meaningful for index creation/removal
	Type  OpType
	Key   Key
	Value Value // meaningful only for OpSet
}

// apply executes an operation against the state, producing a deterministic
// state transition.
//
// Given the same state and the same operation, apply always produces the
// same result. This is the core determinism guarantee of the engine.
//
// On success, the state's sequence number is incremented. Each successful
// apply gets a unique, monotonically increasing sequence number.
//
// Returns ErrKeyNotFound if a DELETE targets a non-existent key.
// Returns an error for unknown operation types.
func (s *state) apply(op Operation) error {
	switch op.Type {
	case OpSet:
		for _, idx := range s.indexes {
			if old, ok := s.data[op.Key]; ok {
				idx.remove(op.Key, old)
			}
			idx.add(op.Key, op.Value)
		}
		s.data[op.Key] = op.Value
		s.seq++
		return nil
	case OpDelete:
		if _, ok := s.data[op.Key]; !ok {
			return ErrKeyNotFound
		}
		for _, idx := range s.indexes {
			idx.remove(op.Key, s.data[op.Key])
		}
		delete(s.data, op.Key)
		s.seq++
		return nil
	case OpCreateIndex:
		if !validIndex(op.Index) {
			return ErrIndexInvalid
		}
		if s.indexes == nil {
			s.indexes = make(map[string]*secondaryIndex)
		}
		idx := &secondaryIndex{definition: op.Index, buckets: make(map[indexToken]map[Key]struct{})}
		for key, value := range s.data {
			idx.add(key, value)
		}
		s.indexes[op.Index.Name] = idx
		s.seq++
		return nil
	case OpDropIndex:
		delete(s.indexes, op.Index.Name)
		s.seq++
		return nil
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}
