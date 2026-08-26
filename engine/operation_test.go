package engine

import (
	"errors"
	"sort"
	"testing"
)

// --- state.apply: OpSet ---

func TestApplySet(t *testing.T) {
	s := newState()

	op := Operation{Type: OpSet, Key: "user:1", Value: NewString("Mehul")}
	if err := s.apply(op); err != nil {
		t.Fatalf("apply SET failed: %v", err)
	}

	val, err := s.get("user:1")
	if err != nil {
		t.Fatalf("get after SET failed: %v", err)
	}
	str, ok := val.String()
	if !ok || str != "Mehul" {
		t.Fatalf("expected 'Mehul', got %q", str)
	}
}

func TestApplySetOverwrite(t *testing.T) {
	s := newState()

	s.apply(Operation{Type: OpSet, Key: "key", Value: NewString("first")})
	s.apply(Operation{Type: OpSet, Key: "key", Value: NewString("second")})

	val, _ := s.get("key")
	str, _ := val.String()
	if str != "second" {
		t.Fatalf("expected 'second' after overwrite, got %q", str)
	}
	if s.len() != 1 {
		t.Fatalf("expected 1 key after overwrite, got %d", s.len())
	}
}

// --- state.apply: OpDelete ---

func TestApplyDelete(t *testing.T) {
	s := newState()

	s.apply(Operation{Type: OpSet, Key: "key", Value: NewNumber(42)})
	if err := s.apply(Operation{Type: OpDelete, Key: "key"}); err != nil {
		t.Fatalf("apply DELETE failed: %v", err)
	}

	if s.exists("key") {
		t.Fatal("key should not exist after DELETE")
	}
	if s.len() != 0 {
		t.Fatalf("expected 0 keys after DELETE, got %d", s.len())
	}
}

func TestApplyDeleteNotFound(t *testing.T) {
	s := newState()

	err := s.apply(Operation{Type: OpDelete, Key: "nonexistent"})
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

// --- Unknown operation type ---

func TestApplyUnknownOpType(t *testing.T) {
	s := newState()

	err := s.apply(Operation{Type: OpType(99), Key: "key"})
	if err == nil {
		t.Fatal("expected error for unknown OpType")
	}
}

// --- OpType.String ---

func TestOpTypeString(t *testing.T) {
	if OpSet.String() != "SET" {
		t.Fatalf("expected 'SET', got %q", OpSet.String())
	}
	if OpDelete.String() != "DELETE" {
		t.Fatalf("expected 'DELETE', got %q", OpDelete.String())
	}
	unknown := OpType(99).String()
	if unknown == "SET" || unknown == "DELETE" {
		t.Fatalf("unknown OpType should not match known names, got %q", unknown)
	}
}

// --- Deterministic replay ---

// TestDeterministicReplay verifies the core determinism guarantee:
// applying the same sequence of operations to two independent fresh states
// must produce identical final states.
func TestDeterministicReplay(t *testing.T) {
	ops := []Operation{
		{Type: OpSet, Key: "user:1", Value: NewObject(map[string]Value{
			"name": NewString("Mehul"),
			"age":  NewNumber(20),
		})},
		{Type: OpSet, Key: "user:2", Value: NewObject(map[string]Value{
			"name": NewString("Alice"),
			"age":  NewNumber(25),
		})},
		{Type: OpSet, Key: "counter", Value: NewNumber(0)},
		{Type: OpSet, Key: "counter", Value: NewNumber(1)}, // overwrite
		{Type: OpDelete, Key: "user:2"},
		{Type: OpSet, Key: "post:1", Value: NewObject(map[string]Value{
			"title":  NewString("Hello"),
			"author": NewString("user:1"),
		})},
		{Type: OpSet, Key: "flag", Value: NewBoolean(true)},
		{Type: OpDelete, Key: "flag"},
		{Type: OpSet, Key: "items", Value: NewArray([]Value{
			NewNumber(1), NewNumber(2), NewNumber(3),
		})},
	}

	// Apply to two independent states.
	s1 := newState()
	s2 := newState()

	for _, op := range ops {
		if err := s1.apply(op); err != nil {
			t.Fatalf("s1 apply failed: %v", err)
		}
		if err := s2.apply(op); err != nil {
			t.Fatalf("s2 apply failed: %v", err)
		}
	}

	// Verify same length.
	if s1.len() != s2.len() {
		t.Fatalf("length mismatch: s1=%d, s2=%d", s1.len(), s2.len())
	}

	// Verify same keys.
	keys1 := s1.keys()
	keys2 := s2.keys()
	sort.Strings(keys1)
	sort.Strings(keys2)

	if len(keys1) != len(keys2) {
		t.Fatalf("key count mismatch: %d vs %d", len(keys1), len(keys2))
	}
	for i := range keys1 {
		if keys1[i] != keys2[i] {
			t.Fatalf("key mismatch at %d: %q vs %q", i, keys1[i], keys2[i])
		}
	}

	// Verify same values.
	for _, key := range keys1 {
		v1, _ := s1.get(key)
		v2, _ := s2.get(key)
		if v1.Type() != v2.Type() {
			t.Fatalf("type mismatch for key %q: %s vs %s", key, v1.Type(), v2.Type())
		}
		assertValuesEqual(t, key, v1, v2)
	}
}

// TestDeterministicReplayDeletedKeysAbsent verifies that keys deleted during
// replay are not present in the final state.
func TestDeterministicReplayDeletedKeysAbsent(t *testing.T) {
	ops := []Operation{
		{Type: OpSet, Key: "a", Value: NewNumber(1)},
		{Type: OpSet, Key: "b", Value: NewNumber(2)},
		{Type: OpSet, Key: "c", Value: NewNumber(3)},
		{Type: OpDelete, Key: "b"},
	}

	s := newState()
	for _, op := range ops {
		s.apply(op)
	}

	if s.exists("b") {
		t.Fatal("deleted key 'b' should not exist after replay")
	}
	if s.len() != 2 {
		t.Fatalf("expected 2 keys, got %d", s.len())
	}
}

// TestReplayFromEmptyMatchesDirectMutation verifies that building state via
// operations produces the same result as direct set/delete calls.
func TestReplayFromEmptyMatchesDirectMutation(t *testing.T) {
	// State built via operations.
	sOp := newState()
	sOp.apply(Operation{Type: OpSet, Key: "x", Value: NewString("hello")})
	sOp.apply(Operation{Type: OpSet, Key: "y", Value: NewNumber(42)})
	sOp.apply(Operation{Type: OpDelete, Key: "x"})

	// State built via direct methods.
	sDirect := newState()
	sDirect.set("x", NewString("hello"))
	sDirect.set("y", NewNumber(42))
	sDirect.delete("x")

	// Must match.
	if sOp.len() != sDirect.len() {
		t.Fatalf("length mismatch: op=%d, direct=%d", sOp.len(), sDirect.len())
	}

	keys := sOp.keys()
	for _, key := range keys {
		v1, _ := sOp.get(key)
		v2, _ := sDirect.get(key)
		if v1.Type() != v2.Type() {
			t.Fatalf("type mismatch for key %q", key)
		}
		assertValuesEqual(t, key, v1, v2)
	}
}

// TestApplySequenceViaDB verifies that DB.Set and DB.Delete produce the same
// state as applying the equivalent operations directly to a fresh state.
func TestApplySequenceViaDB(t *testing.T) {
	db, _ := Open("")
	db.Set("a", NewString("alpha"))
	db.Set("b", NewNumber(2))
	db.Delete("a")
	db.Set("c", NewBoolean(true))

	s := newState()
	s.apply(Operation{Type: OpSet, Key: "a", Value: NewString("alpha")})
	s.apply(Operation{Type: OpSet, Key: "b", Value: NewNumber(2)})
	s.apply(Operation{Type: OpDelete, Key: "a"})
	s.apply(Operation{Type: OpSet, Key: "c", Value: NewBoolean(true)})

	if db.Len() != s.len() {
		t.Fatalf("length mismatch: db=%d, state=%d", db.Len(), s.len())
	}

	for _, key := range s.keys() {
		v1, _ := db.Get(key)
		v2, _ := s.get(key)
		if v1.Type() != v2.Type() {
			t.Fatalf("type mismatch for key %q", key)
		}
		assertValuesEqual(t, key, v1, v2)
	}
}

// --- Helpers ---

// assertValuesEqual compares two Values for equality, failing the test if
// they differ. Supports all Value types.
func assertValuesEqual(t *testing.T, context string, a, b Value) {
	t.Helper()
	if a.Type() != b.Type() {
		t.Fatalf("[%s] type mismatch: %s vs %s", context, a.Type(), b.Type())
	}
	switch a.Type() {
	case TypeNull:
		// both null, equal
	case TypeBoolean:
		ba, _ := a.Boolean()
		bb, _ := b.Boolean()
		if ba != bb {
			t.Fatalf("[%s] boolean mismatch: %v vs %v", context, ba, bb)
		}
	case TypeNumber:
		na, _ := a.Number()
		nb, _ := b.Number()
		if na != nb {
			t.Fatalf("[%s] number mismatch: %v vs %v", context, na, nb)
		}
	case TypeString:
		sa, _ := a.String()
		sb, _ := b.String()
		if sa != sb {
			t.Fatalf("[%s] string mismatch: %q vs %q", context, sa, sb)
		}
	case TypeArray:
		aa, _ := a.Array()
		ab, _ := b.Array()
		if len(aa) != len(ab) {
			t.Fatalf("[%s] array length mismatch: %d vs %d", context, len(aa), len(ab))
		}
		for i := range aa {
			assertValuesEqual(t, context+"[]", aa[i], ab[i])
		}
	case TypeObject:
		oa, _ := a.Object()
		ob, _ := b.Object()
		if len(oa) != len(ob) {
			t.Fatalf("[%s] object size mismatch: %d vs %d", context, len(oa), len(ob))
		}
		for k, va := range oa {
			vb, ok := ob[k]
			if !ok {
				t.Fatalf("[%s] object key %q missing in second value", context, k)
			}
			assertValuesEqual(t, context+"."+k, va, vb)
		}
	}
}
