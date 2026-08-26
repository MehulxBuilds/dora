package engine

// state is the in-memory keyspace. It holds the current materialized state
// of all key-value pairs.
//
// state is an internal type. It is not exposed through the public API.
// The DB struct owns and operates on a state instance.
type state struct {
	data map[Key]Value
}

// newState creates a new empty state.
func newState() *state {
	return &state{
		data: make(map[Key]Value),
	}
}

// newStateFrom creates a state pre-populated with the given data.
// Used when loading from storage. The map is taken directly (not copied),
// since the caller (storage layer) is handing ownership to the state.
func newStateFrom(data map[Key]Value) *state {
	if data == nil {
		data = make(map[Key]Value)
	}
	return &state{data: data}
}

// get retrieves the value for a key. Returns ErrKeyNotFound if the key does not exist.
func (s *state) get(key Key) (Value, error) {
	val, ok := s.data[key]
	if !ok {
		return NewNull(), ErrKeyNotFound
	}
	return val, nil
}

// set stores a value for a key, overwriting any existing value.
func (s *state) set(key Key, val Value) {
	s.data[key] = val
}

// delete removes a key from the state. Returns ErrKeyNotFound if the key does not exist.
func (s *state) delete(key Key) error {
	if _, ok := s.data[key]; !ok {
		return ErrKeyNotFound
	}
	delete(s.data, key)
	return nil
}

// exists returns true if the key exists in the state.
func (s *state) exists(key Key) bool {
	_, ok := s.data[key]
	return ok
}

// len returns the number of keys in the state.
func (s *state) len() int {
	return len(s.data)
}

// keys returns a slice of all keys in the state.
// The order is not guaranteed (map iteration order).
func (s *state) keys() []Key {
	result := make([]Key, 0, len(s.data))
	for k := range s.data {
		result = append(result, k)
	}
	return result
}

// snapshot returns a copy of the entire keyspace for persistence.
// The returned map is a shallow copy — safe for the storage layer to
// serialize without holding locks.
func (s *state) snapshot() map[Key]Value {
	cp := make(map[Key]Value, len(s.data))
	for k, v := range s.data {
		cp[k] = v
	}
	return cp
}
