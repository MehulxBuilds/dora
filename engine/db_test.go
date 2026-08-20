package engine

import (
	"errors"
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
