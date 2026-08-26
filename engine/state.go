package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

// state is the in-memory keyspace. It holds the current materialized state
// of all key-value pairs and the operation sequence number.
//
// state is an internal type. It is not exposed through the public API.
// The DB struct owns and operates on a state instance.
type state struct {
	data map[Key]Value
	seq  uint64 // monotonically increasing, incremented on each successful apply
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

// keysWithPrefix returns all keys that start with the given prefix.
// The order is not guaranteed.
func (s *state) keysWithPrefix(prefix string) []Key {
	var result []Key
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			result = append(result, k)
		}
	}
	return result
}

// prefix returns all key-value pairs where the key starts with the
// given prefix. The returned map is a new map — safe to use without
// holding locks.
func (s *state) prefix(pfx string) map[Key]Value {
	result := make(map[Key]Value)
	for k, v := range s.data {
		if strings.HasPrefix(k, pfx) {
			result[k] = v
		}
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

// hash computes a deterministic SHA-256 hash of the entire state.
//
// The hash covers only the key-value data, NOT the sequence number.
// Two states with identical key-value pairs produce the same hash,
// regardless of how they arrived at that state.
//
// Algorithm:
//  1. Sort all keys lexicographically.
//  2. For each key, write: key-length (4 bytes big-endian) + key bytes +
//     value-length (4 bytes big-endian) + value JSON bytes.
//  3. Return SHA-256 of the concatenation.
//
// The length-prefix encoding ensures the hash is unambiguous — different
// key-value pairs cannot produce the same byte sequence.
//
// Determinism relies on Value.MarshalJSON being deterministic (it is:
// Go's encoding/json sorts map keys, and all other types have a single
// canonical representation).
func (s *state) hash() ([32]byte, error) {
	keys := s.keys()
	sort.Strings(keys)

	h := sha256.New()

	// Include key count so an empty state has a distinct hash from no state.
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(len(keys)))
	h.Write(buf[:])

	for _, key := range keys {
		keyBytes := []byte(key)
		valBytes, err := s.data[key].MarshalJSON()
		if err != nil {
			return [32]byte{}, fmt.Errorf("hash: marshal value for key %q: %w", key, err)
		}

		// Length-prefix key.
		binary.BigEndian.PutUint32(buf[:], uint32(len(keyBytes)))
		h.Write(buf[:])
		h.Write(keyBytes)

		// Length-prefix value.
		binary.BigEndian.PutUint32(buf[:], uint32(len(valBytes)))
		h.Write(buf[:])
		h.Write(valBytes)
	}

	return [32]byte(h.Sum(nil)), nil
}
