package engine

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

// openMem is a test helper that opens an in-memory database, failing the test on error.
func openMem(t *testing.T) *DB {
	t.Helper()
	db, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") failed: %v", err)
	}
	return db
}

// --- Phase 0 & 1 Tests (updated for new Open signature) ---

func TestOpenCreatesDB(t *testing.T) {
	db := openMem(t)
	if db.Len() != 0 {
		t.Fatalf("new db should have 0 keys, got %d", db.Len())
	}
}

func TestSetAndGet(t *testing.T) {
	db := openMem(t)

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
	db := openMem(t)
	_, err := db.Get("nonexistent")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestSetOverwrite(t *testing.T) {
	db := openMem(t)

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
	db := openMem(t)
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
	db := openMem(t)
	err := db.Delete("nonexistent")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestExists(t *testing.T) {
	db := openMem(t)
	if db.Exists("key") {
		t.Fatal("key should not exist in empty db")
	}
	db.Set("key", NewNumber(1))
	if !db.Exists("key") {
		t.Fatal("key should exist after Set")
	}
}

func TestEmptyKeyErrors(t *testing.T) {
	db := openMem(t)

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
	db := openMem(t)

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
	db := openMem(t)

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

func TestClose(t *testing.T) {
	db := openMem(t)
	db.Set("key", NewString("value"))
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestKeys(t *testing.T) {
	db := openMem(t)

	keys := db.Keys()
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}

	db.Set("c", NewNumber(3))
	db.Set("a", NewNumber(1))
	db.Set("b", NewNumber(2))

	keys = db.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	sort.Strings(keys)
	expected := []string{"a", "b", "c"}
	for i, k := range keys {
		if k != expected[i] {
			t.Fatalf("keys[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

func TestSetDeleteSet(t *testing.T) {
	db := openMem(t)

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
}

func TestDeleteThenGet(t *testing.T) {
	db := openMem(t)
	db.Set("key", NewString("value"))
	db.Delete("key")

	_, err := db.Get("key")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("Get after Delete: expected ErrKeyNotFound, got %v", err)
	}
}

func TestDoubleDelete(t *testing.T) {
	db := openMem(t)
	db.Set("key", NewString("value"))
	db.Delete("key")

	err := db.Delete("key")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("second Delete: expected ErrKeyNotFound, got %v", err)
	}
}

func TestSetNullValue(t *testing.T) {
	db := openMem(t)
	db.Set("key", NewNull())

	got, err := db.Get("key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !got.IsNull() {
		t.Fatalf("expected Null value, got type %s", got.Type())
	}
	if !db.Exists("key") {
		t.Fatal("key with Null value should still exist")
	}
}

func TestSpecialCharacterKeys(t *testing.T) {
	db := openMem(t)

	specialKeys := []string{
		"user:1", "a.b.c", "key with spaces", "/path/like/key",
		"key\twith\ttabs", "unicode:日本語",
	}

	for i, k := range specialKeys {
		db.Set(k, NewNumber(float64(i)))
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
	db := openMem(t)

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

	got, _ := db.Get("nested")
	obj, _ := got.Object()
	items, _ := obj["items"].Array()
	innerObj, _ := items[0].Object()
	name, _ := innerObj["name"].String()
	if name != "nested" {
		t.Fatalf("expected 'nested', got %q", name)
	}
}

func TestKeysAfterDeletes(t *testing.T) {
	db := openMem(t)
	db.Set("a", NewNumber(1))
	db.Set("b", NewNumber(2))
	db.Set("c", NewNumber(3))
	db.Delete("b")

	keys := db.Keys()
	sort.Strings(keys)
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "c" {
		t.Fatalf("expected [a, c], got %v", keys)
	}
}

func TestStress(t *testing.T) {
	db := openMem(t)
	const n = 10_000

	for i := 0; i < n; i++ {
		key := "key:" + itoa(i)
		db.Set(key, NewNumber(float64(i)))
	}

	if db.Len() != n {
		t.Fatalf("expected %d keys, got %d", n, db.Len())
	}

	for i := 0; i < n; i += 2 {
		db.Delete("key:" + itoa(i))
	}

	if db.Len() != n/2 {
		t.Fatalf("expected %d keys after deletes, got %d", n/2, db.Len())
	}
}

func TestConcurrentReads(t *testing.T) {
	db := openMem(t)
	db.Set("key", NewString("value"))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := db.Get("key")
			if err != nil {
				t.Errorf("Get failed: %v", err)
				return
			}
			s, _ := v.String()
			if s != "value" {
				t.Errorf("expected 'value', got %q", s)
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentWrites(t *testing.T) {
	db := openMem(t)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			db.Set("key:"+itoa(id), NewNumber(float64(id)))
		}(i)
	}
	wg.Wait()

	if db.Len() != 100 {
		t.Fatalf("expected 100 keys, got %d", db.Len())
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	db := openMem(t)
	db.Set("counter", NewNumber(0))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			db.Set("counter", NewNumber(float64(id)))
		}(i)
	}
	for i := 0; i < 50; i++ {
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
}

// --- Phase 2 Tests: Persistence ---

// tempDBPath returns a temporary file path for a test database.
func tempDBPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

func TestPersistenceRoundTrip(t *testing.T) {
	path := tempDBPath(t)

	// Open, write data, close.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	db.Set("user:1", NewObject(map[string]Value{
		"name": NewString("Mehul"),
		"age":  NewNumber(20),
	}))
	db.Set("counter", NewNumber(42))
	db.Set("active", NewBoolean(true))

	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Reopen and verify all data persisted.
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer db2.Close()

	if db2.Len() != 3 {
		t.Fatalf("expected 3 keys after reopen, got %d", db2.Len())
	}

	// Verify user:1.
	v, err := db2.Get("user:1")
	if err != nil {
		t.Fatalf("Get(user:1) failed: %v", err)
	}
	obj, ok := v.Object()
	if !ok {
		t.Fatal("expected Object for user:1")
	}
	name, _ := obj["name"].String()
	if name != "Mehul" {
		t.Fatalf("expected 'Mehul', got %q", name)
	}
	age, _ := obj["age"].Number()
	if age != 20 {
		t.Fatalf("expected 20, got %v", age)
	}

	// Verify counter.
	v, _ = db2.Get("counter")
	n, _ := v.Number()
	if n != 42 {
		t.Fatalf("expected 42, got %v", n)
	}

	// Verify active.
	v, _ = db2.Get("active")
	b, _ := v.Boolean()
	if !b {
		t.Fatal("expected true")
	}
}

func TestPersistenceAllValueTypes(t *testing.T) {
	path := tempDBPath(t)

	db, _ := Open(path)
	db.Set("null", NewNull())
	db.Set("bool", NewBoolean(false))
	db.Set("num", NewNumber(-3.14))
	db.Set("str", NewString("hello world"))
	db.Set("arr", NewArray([]Value{NewNumber(1), NewString("two"), NewNull()}))
	db.Set("obj", NewObject(map[string]Value{
		"nested": NewBoolean(true),
	}))
	db.Close()

	db2, _ := Open(path)
	defer db2.Close()

	if db2.Len() != 6 {
		t.Fatalf("expected 6 keys, got %d", db2.Len())
	}

	// Null
	v, _ := db2.Get("null")
	if !v.IsNull() {
		t.Fatalf("expected Null, got %s", v.Type())
	}

	// Boolean
	v, _ = db2.Get("bool")
	b, ok := v.Boolean()
	if !ok || b {
		t.Fatalf("expected false, got %v", b)
	}

	// Number
	v, _ = db2.Get("num")
	n, _ := v.Number()
	if n != -3.14 {
		t.Fatalf("expected -3.14, got %v", n)
	}

	// String
	v, _ = db2.Get("str")
	s, _ := v.String()
	if s != "hello world" {
		t.Fatalf("expected 'hello world', got %q", s)
	}

	// Array
	v, _ = db2.Get("arr")
	arr, _ := v.Array()
	if len(arr) != 3 {
		t.Fatalf("expected 3 items, got %d", len(arr))
	}

	// Object
	v, _ = db2.Get("obj")
	obj, _ := v.Object()
	nested, _ := obj["nested"].Boolean()
	if !nested {
		t.Fatal("expected nested=true")
	}
}

func TestPersistenceNewFile(t *testing.T) {
	path := tempDBPath(t)

	// Opening a non-existent file should create a new empty database.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open new file failed: %v", err)
	}
	if db.Len() != 0 {
		t.Fatalf("new file db should have 0 keys, got %d", db.Len())
	}
	db.Close()
}

func TestPersistenceDeletesSurviveReopen(t *testing.T) {
	path := tempDBPath(t)

	db, _ := Open(path)
	db.Set("a", NewNumber(1))
	db.Set("b", NewNumber(2))
	db.Set("c", NewNumber(3))
	db.Delete("b")
	db.Close()

	db2, _ := Open(path)
	defer db2.Close()

	if db2.Len() != 2 {
		t.Fatalf("expected 2 keys after reopen, got %d", db2.Len())
	}
	if db2.Exists("b") {
		t.Fatal("deleted key 'b' should not exist after reopen")
	}
	if !db2.Exists("a") || !db2.Exists("c") {
		t.Fatal("keys 'a' and 'c' should survive reopen")
	}
}

func TestPersistenceOverwriteSurvivesReopen(t *testing.T) {
	path := tempDBPath(t)

	db, _ := Open(path)
	db.Set("key", NewString("first"))
	db.Set("key", NewString("second"))
	db.Close()

	db2, _ := Open(path)
	defer db2.Close()

	v, _ := db2.Get("key")
	s, _ := v.String()
	if s != "second" {
		t.Fatalf("expected 'second' after reopen, got %q", s)
	}
}

func TestPersistenceCorruptFile(t *testing.T) {
	path := tempDBPath(t)

	// Write invalid JSON to the file.
	os.WriteFile(path, []byte("{invalid json!!!"), 0o644)

	_, err := Open(path)
	if err == nil {
		t.Fatal("Open with corrupt file should return an error")
	}
}

func TestPersistenceEmptyFile(t *testing.T) {
	path := tempDBPath(t)

	// Write an empty file.
	os.WriteFile(path, []byte(""), 0o644)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open with empty file failed: %v", err)
	}
	defer db.Close()

	if db.Len() != 0 {
		t.Fatalf("expected 0 keys from empty file, got %d", db.Len())
	}
}

func TestPersistenceFileIsReadableJSON(t *testing.T) {
	path := tempDBPath(t)

	db, _ := Open(path)
	db.Set("name", NewString("Mehul"))
	db.Close()

	// Read the file directly and verify it's valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Fatal("file should not be empty after Close")
	}

	// Should contain the key and value as readable JSON.
	if !containsSubstring(content, "name") || !containsSubstring(content, "Mehul") {
		t.Fatalf("file content should contain key/value, got:\n%s", content)
	}
}

func TestPersistenceMultipleCloseReopen(t *testing.T) {
	path := tempDBPath(t)

	// Round 1: set some data.
	db, _ := Open(path)
	db.Set("round", NewNumber(1))
	db.Close()

	// Round 2: add more data.
	db, _ = Open(path)
	db.Set("round", NewNumber(2))
	db.Set("extra", NewString("data"))
	db.Close()

	// Round 3: verify everything.
	db, _ = Open(path)
	defer db.Close()

	if db.Len() != 2 {
		t.Fatalf("expected 2 keys, got %d", db.Len())
	}
	v, _ := db.Get("round")
	n, _ := v.Number()
	if n != 2 {
		t.Fatalf("expected round=2, got %v", n)
	}
}

func TestPersistenceNestedValues(t *testing.T) {
	path := tempDBPath(t)

	db, _ := Open(path)
	db.Set("deep", NewObject(map[string]Value{
		"level1": NewObject(map[string]Value{
			"level2": NewArray([]Value{
				NewObject(map[string]Value{
					"level3": NewString("deep value"),
				}),
			}),
		}),
	}))
	db.Close()

	db2, _ := Open(path)
	defer db2.Close()

	v, _ := db2.Get("deep")
	obj, _ := v.Object()
	l1, _ := obj["level1"].Object()
	l2, _ := l1["level2"].Array()
	l3, _ := l2[0].Object()
	s, _ := l3["level3"].String()
	if s != "deep value" {
		t.Fatalf("expected 'deep value', got %q", s)
	}
}

// --- Helpers ---

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && findSubstring(s, sub)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

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
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
