package engine

import (
	"errors"
	"sort"
	"sync"
	"testing"
)

func TestOpenCreatesDB(t *testing.T) {
	db := Open()
	if db == nil {
		t.Fatal("Open() returned nil")
	}
	if db.Len() != 0 {
		t.Fatalf("new db should have 0 keys, got %d", db.Len())
	}
}

func TestSetAndGet(t *testing.T) {
	db := Open()

	val := NewObject(map[string]Value{
		"name": NewString("Mehul"),
		"age":  NewNumber(20),
	})

	if err := db.Set("user:1", val); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := db.Get("user:1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Type() != TypeObject {
		t.Fatalf("expected TypeObject, got %s", got.Type())
	}

	obj, _ := got.Object()
	name, _ := obj["name"].String()
	if name != "Mehul" {
		t.Fatalf("expected 'Mehul', got %q", name)
	}
}

func TestGetNotFound(t *testing.T) {
	db := Open()
	_, err := db.Get("nonexistent")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestSetOverwrite(t *testing.T) {
	db := Open()

	db.Set("key", NewString("first"))
	db.Set("key", NewString("second"))

	got, err := db.Get("key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	s, _ := got.String()
	if s != "second" {
		t.Fatalf("expected 'second', got %q", s)
	}
	if db.Len() != 1 {
		t.Fatalf("expected 1 key after overwrite, got %d", db.Len())
	}
}

func TestDelete(t *testing.T) {
	db := Open()
	db.Set("key", NewString("value"))

	err := db.Delete("key")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if db.Exists("key") {
		t.Fatal("key should not exist after delete")
	}
	if db.Len() != 0 {
		t.Fatalf("expected 0 keys after delete, got %d", db.Len())
	}
}

func TestDeleteNotFound(t *testing.T) {
	db := Open()
	err := db.Delete("nonexistent")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestExists(t *testing.T) {
	db := Open()
	if db.Exists("key") {
		t.Fatal("key should not exist in empty db")
	}
	db.Set("key", NewNumber(1))
	if !db.Exists("key") {
		t.Fatal("key should exist after Set")
	}
}

func TestEmptyKeyErrors(t *testing.T) {
	db := Open()

	if err := db.Set("", NewString("x")); !errors.Is(err, ErrKeyEmpty) {
		t.Fatalf("Set with empty key: expected ErrKeyEmpty, got %v", err)
	}
	if _, err := db.Get(""); !errors.Is(err, ErrKeyEmpty) {
		t.Fatalf("Get with empty key: expected ErrKeyEmpty, got %v", err)
	}
	if err := db.Delete(""); !errors.Is(err, ErrKeyEmpty) {
		t.Fatalf("Delete with empty key: expected ErrKeyEmpty, got %v", err)
	}
	if db.Exists("") {
		t.Fatal("Exists with empty key should return false")
	}
}

func TestMultipleKeys(t *testing.T) {
	db := Open()

	keys := []string{"user:1", "user:2", "post:1", "settings:global"}
	for i, k := range keys {
		db.Set(k, NewNumber(float64(i)))
	}

	if db.Len() != len(keys) {
		t.Fatalf("expected %d keys, got %d", len(keys), db.Len())
	}

	for i, k := range keys {
		v, err := db.Get(k)
		if err != nil {
			t.Fatalf("Get(%q) failed: %v", k, err)
		}
		n, _ := v.Number()
		if n != float64(i) {
			t.Fatalf("Get(%q) = %v, want %v", k, n, float64(i))
		}
	}
}

func TestSetAllValueTypes(t *testing.T) {
	db := Open()

	tests := []struct {
		key string
		val Value
		typ ValueType
	}{
		{"null", NewNull(), TypeNull},
		{"bool", NewBoolean(true), TypeBoolean},
		{"num", NewNumber(3.14), TypeNumber},
		{"str", NewString("hello"), TypeString},
		{"arr", NewArray([]Value{NewNumber(1)}), TypeArray},
		{"obj", NewObject(map[string]Value{"a": NewNumber(1)}), TypeObject},
	}

	for _, tt := range tests {
		if err := db.Set(tt.key, tt.val); err != nil {
			t.Fatalf("Set(%q) failed: %v", tt.key, err)
		}
		got, err := db.Get(tt.key)
		if err != nil {
			t.Fatalf("Get(%q) failed: %v", tt.key, err)
		}
		if got.Type() != tt.typ {
			t.Fatalf("Get(%q).Type() = %s, want %s", tt.key, got.Type(), tt.typ)
		}
	}
}

// --- Phase 1 Tests ---

func TestClose(t *testing.T) {
	db := Open()
	db.Set("key", NewString("value"))
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestKeys(t *testing.T) {
	db := Open()

	// Empty database should return empty slice.
	keys := db.Keys()
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}

	// Insert some keys and verify.
	db.Set("c", NewNumber(3))
	db.Set("a", NewNumber(1))
	db.Set("b", NewNumber(2))

	keys = db.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	// Sort for deterministic comparison (map order is random).
	sort.Strings(keys)
	expected := []string{"a", "b", "c"}
	for i, k := range keys {
		if k != expected[i] {
			t.Fatalf("keys[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

func TestSetDeleteSet(t *testing.T) {
	db := Open()

	db.Set("key", NewString("first"))
	db.Delete("key")
	db.Set("key", NewString("second"))

	got, err := db.Get("key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	s, _ := got.String()
	if s != "second" {
		t.Fatalf("expected 'second' after set-delete-set, got %q", s)
	}
	if db.Len() != 1 {
		t.Fatalf("expected 1 key, got %d", db.Len())
	}
}

func TestDeleteThenGet(t *testing.T) {
	db := Open()
	db.Set("key", NewString("value"))
	db.Delete("key")

	_, err := db.Get("key")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("Get after Delete: expected ErrKeyNotFound, got %v", err)
	}
}

func TestDoubleDelete(t *testing.T) {
	db := Open()
	db.Set("key", NewString("value"))
	db.Delete("key")

	err := db.Delete("key")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("second Delete: expected ErrKeyNotFound, got %v", err)
	}
}

func TestSetNullValue(t *testing.T) {
	db := Open()
	db.Set("key", NewNull())

	got, err := db.Get("key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !got.IsNull() {
		t.Fatalf("expected Null value, got type %s", got.Type())
	}
	// Key with null value should still exist.
	if !db.Exists("key") {
		t.Fatal("key with Null value should still exist")
	}
}

func TestSpecialCharacterKeys(t *testing.T) {
	db := Open()

	specialKeys := []string{
		"user:1",
		"a.b.c",
		"key with spaces",
		"emoji-key",
		"/path/like/key",
		"key\twith\ttabs",
		"key\nwith\nnewlines",
		"unicode:日本語",
	}

	for i, k := range specialKeys {
		if err := db.Set(k, NewNumber(float64(i))); err != nil {
			t.Fatalf("Set(%q) failed: %v", k, err)
		}
	}

	if db.Len() != len(specialKeys) {
		t.Fatalf("expected %d keys, got %d", len(specialKeys), db.Len())
	}

	for i, k := range specialKeys {
		v, err := db.Get(k)
		if err != nil {
			t.Fatalf("Get(%q) failed: %v", k, err)
		}
		n, _ := v.Number()
		if n != float64(i) {
			t.Fatalf("Get(%q) = %v, want %v", k, n, float64(i))
		}
	}
}

func TestNestedValues(t *testing.T) {
	db := Open()

	// Build a nested structure: object containing array containing object.
	inner := NewObject(map[string]Value{
		"id":   NewNumber(1),
		"name": NewString("nested"),
	})
	arr := NewArray([]Value{inner, NewNull(), NewBoolean(true)})
	outer := NewObject(map[string]Value{
		"items": arr,
		"count": NewNumber(3),
	})

	db.Set("nested", outer)

	got, err := db.Get("nested")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	obj, ok := got.Object()
	if !ok {
		t.Fatal("expected Object")
	}

	items, ok := obj["items"].Array()
	if !ok {
		t.Fatal("expected Array for 'items'")
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	innerObj, ok := items[0].Object()
	if !ok {
		t.Fatal("expected Object at items[0]")
	}
	name, ok := innerObj["name"].String()
	if !ok || name != "nested" {
		t.Fatalf("expected 'nested', got %q", name)
	}
}

func TestKeysAfterDeletes(t *testing.T) {
	db := Open()
	db.Set("a", NewNumber(1))
	db.Set("b", NewNumber(2))
	db.Set("c", NewNumber(3))
	db.Delete("b")

	keys := db.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys after delete, got %d", len(keys))
	}
	sort.Strings(keys)
	if keys[0] != "a" || keys[1] != "c" {
		t.Fatalf("expected [a, c], got %v", keys)
	}
}

func TestStress(t *testing.T) {
	db := Open()
	const n = 10_000

	// Insert n keys.
	for i := 0; i < n; i++ {
		key := "key:" + string(rune('A'+(i%26))) + ":" + itoa(i)
		db.Set(key, NewNumber(float64(i)))
	}

	if db.Len() != n {
		t.Fatalf("expected %d keys, got %d", n, db.Len())
	}

	// Verify all keys are retrievable.
	for i := 0; i < n; i++ {
		key := "key:" + string(rune('A'+(i%26))) + ":" + itoa(i)
		v, err := db.Get(key)
		if err != nil {
			t.Fatalf("Get(%q) failed: %v", key, err)
		}
		num, _ := v.Number()
		if num != float64(i) {
			t.Fatalf("Get(%q) = %v, want %v", key, num, float64(i))
		}
	}

	// Delete half.
	for i := 0; i < n; i += 2 {
		key := "key:" + string(rune('A'+(i%26))) + ":" + itoa(i)
		db.Delete(key)
	}

	if db.Len() != n/2 {
		t.Fatalf("expected %d keys after deletes, got %d", n/2, db.Len())
	}
}

func TestConcurrentReads(t *testing.T) {
	db := Open()
	db.Set("key", NewString("value"))

	var wg sync.WaitGroup
	const goroutines = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := db.Get("key")
			if err != nil {
				t.Errorf("Get failed: %v", err)
				return
			}
			s, ok := v.String()
			if !ok || s != "value" {
				t.Errorf("expected 'value', got %q (ok=%v)", s, ok)
			}
		}()
	}

	wg.Wait()
}

func TestConcurrentWrites(t *testing.T) {
	db := Open()

	var wg sync.WaitGroup
	const goroutines = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "key:" + itoa(id)
			db.Set(key, NewNumber(float64(id)))
		}(i)
	}

	wg.Wait()

	if db.Len() != goroutines {
		t.Fatalf("expected %d keys, got %d", goroutines, db.Len())
	}

	// Verify all values.
	for i := 0; i < goroutines; i++ {
		key := "key:" + itoa(i)
		v, err := db.Get(key)
		if err != nil {
			t.Fatalf("Get(%q) failed: %v", key, err)
		}
		n, _ := v.Number()
		if n != float64(i) {
			t.Fatalf("Get(%q) = %v, want %v", key, n, float64(i))
		}
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	db := Open()
	db.Set("counter", NewNumber(0))

	var wg sync.WaitGroup
	const readers = 50
	const writers = 50

	// Writers overwrite the same key concurrently.
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			db.Set("counter", NewNumber(float64(id)))
		}(i)
	}

	// Readers read the key concurrently.
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := db.Get("counter")
			if err != nil {
				t.Errorf("Get failed: %v", err)
				return
			}
			if v.Type() != TypeNumber {
				t.Errorf("expected TypeNumber, got %s", v.Type())
			}
		}()
	}

	wg.Wait()

	// After all goroutines finish, the key should still exist with some number value.
	v, err := db.Get("counter")
	if err != nil {
		t.Fatalf("final Get failed: %v", err)
	}
	if v.Type() != TypeNumber {
		t.Fatalf("expected TypeNumber, got %s", v.Type())
	}
}

// itoa is a minimal int-to-string helper to avoid importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	// Reverse.
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
