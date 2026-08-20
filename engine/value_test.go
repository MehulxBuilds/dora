package engine

import "testing"

func TestValueNull(t *testing.T) {
	v := NewNull()
	if v.Type() != TypeNull {
		t.Fatalf("expected TypeNull, got %s", v.Type())
	}
	if !v.IsNull() {
		t.Fatal("expected IsNull to be true")
	}
}

func TestValueBoolean(t *testing.T) {
	v := NewBoolean(true)
	if v.Type() != TypeBoolean {
		t.Fatalf("expected TypeBoolean, got %s", v.Type())
	}
	b, ok := v.Boolean()
	if !ok || !b {
		t.Fatalf("expected true, got %v (ok=%v)", b, ok)
	}

	v2 := NewBoolean(false)
	b2, ok := v2.Boolean()
	if !ok || b2 {
		t.Fatalf("expected false, got %v (ok=%v)", b2, ok)
	}
}

func TestValueNumber(t *testing.T) {
	v := NewNumber(42.5)
	if v.Type() != TypeNumber {
		t.Fatalf("expected TypeNumber, got %s", v.Type())
	}
	n, ok := v.Number()
	if !ok || n != 42.5 {
		t.Fatalf("expected 42.5, got %v (ok=%v)", n, ok)
	}
}

func TestValueString(t *testing.T) {
	v := NewString("hello")
	if v.Type() != TypeString {
		t.Fatalf("expected TypeString, got %s", v.Type())
	}
	s, ok := v.String()
	if !ok || s != "hello" {
		t.Fatalf("expected 'hello', got %v (ok=%v)", s, ok)
	}
}

func TestValueArray(t *testing.T) {
	items := []Value{NewNumber(1), NewString("two"), NewBoolean(true)}
	v := NewArray(items)
	if v.Type() != TypeArray {
		t.Fatalf("expected TypeArray, got %s", v.Type())
	}
	arr, ok := v.Array()
	if !ok {
		t.Fatal("expected Array accessor to return ok=true")
	}
	if len(arr) != 3 {
		t.Fatalf("expected 3 items, got %d", len(arr))
	}
	// Verify the copy: mutating the returned slice should not affect the original.
	arr[0] = NewNull()
	arr2, _ := v.Array()
	n, _ := arr2[0].Number()
	if n != 1 {
		t.Fatal("Array accessor should return a copy, but original was mutated")
	}
}

func TestValueObject(t *testing.T) {
	fields := map[string]Value{
		"name": NewString("Mehul"),
		"age":  NewNumber(20),
	}
	v := NewObject(fields)
	if v.Type() != TypeObject {
		t.Fatalf("expected TypeObject, got %s", v.Type())
	}
	obj, ok := v.Object()
	if !ok {
		t.Fatal("expected Object accessor to return ok=true")
	}
	if len(obj) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(obj))
	}
	name, ok := obj["name"].String()
	if !ok || name != "Mehul" {
		t.Fatalf("expected 'Mehul', got %v", name)
	}
	// Verify the copy: mutating the returned map should not affect the original.
	obj["name"] = NewString("changed")
	obj2, _ := v.Object()
	name2, _ := obj2["name"].String()
	if name2 != "Mehul" {
		t.Fatal("Object accessor should return a copy, but original was mutated")
	}
}

func TestValueWrongTypeAccessors(t *testing.T) {
	v := NewString("hello")

	if _, ok := v.Boolean(); ok {
		t.Fatal("Boolean() should return ok=false for a String value")
	}
	if _, ok := v.Number(); ok {
		t.Fatal("Number() should return ok=false for a String value")
	}
	if _, ok := v.Array(); ok {
		t.Fatal("Array() should return ok=false for a String value")
	}
	if _, ok := v.Object(); ok {
		t.Fatal("Object() should return ok=false for a String value")
	}
	if v.IsNull() {
		t.Fatal("IsNull() should be false for a String value")
	}
}

func TestValueTypeString(t *testing.T) {
	tests := []struct {
		typ  ValueType
		want string
	}{
		{TypeNull, "Null"},
		{TypeBoolean, "Boolean"},
		{TypeNumber, "Number"},
		{TypeString, "String"},
		{TypeArray, "Array"},
		{TypeObject, "Object"},
		{ValueType(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("ValueType(%d).String() = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestNewArrayCopiesInput(t *testing.T) {
	items := []Value{NewNumber(1)}
	v := NewArray(items)
	// Mutating the original slice should not affect the Value.
	items[0] = NewNull()
	arr, _ := v.Array()
	n, _ := arr[0].Number()
	if n != 1 {
		t.Fatal("NewArray should copy the input slice")
	}
}

func TestNewObjectCopiesInput(t *testing.T) {
	fields := map[string]Value{"a": NewNumber(1)}
	v := NewObject(fields)
	// Mutating the original map should not affect the Value.
	fields["a"] = NewNull()
	obj, _ := v.Object()
	n, _ := obj["a"].Number()
	if n != 1 {
		t.Fatal("NewObject should copy the input map")
	}
}
