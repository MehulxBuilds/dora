package engine

import "fmt"

// OpType identifies the type of a database operation.
type OpType int

const (
	// OpSet represents a key-value write. Overwrites any existing value.
	OpSet OpType = iota

	// OpDelete represents a key removal.
	OpDelete
)

// String returns a human-readable name for the OpType.
func (t OpType) String() string {
	switch t {
	case OpSet:
		return "SET"
	case OpDelete:
		return "DELETE"
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
// Returns ErrKeyNotFound if a DELETE targets a non-existent key.
// Returns an error for unknown operation types.
func (s *state) apply(op Operation) error {
	switch op.Type {
	case OpSet:
		s.data[op.Key] = op.Value
		return nil
	case OpDelete:
		if _, ok := s.data[op.Key]; !ok {
			return ErrKeyNotFound
		}
		delete(s.data, op.Key)
		return nil
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}
