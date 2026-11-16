package engine

import "fmt"

// ValueType identifies the type of a database Value.
type ValueType int

const (
	TypeNull      ValueType = iota // Null represents a null/nil value.
	TypeBoolean                    // Boolean represents true or false.
	TypeNumber                     // Number represents a numeric value (float64).
	TypeString                     // String represents a text value.
	TypeArray                      // Array represents an ordered list of Values.
	TypeObject                     // Object represents a key-value map of string to Value.
	TypeReference                  // Reference identifies another database key.
)

// String returns a human-readable name for the ValueType.
func (t ValueType) String() string {
	switch t {
	case TypeNull:
		return "Null"
	case TypeBoolean:
		return "Boolean"
	case TypeNumber:
		return "Number"
	case TypeString:
		return "String"
	case TypeArray:
		return "Array"
	case TypeReference:
		return "Reference"
	case TypeObject:
		return "Object"
	default:
		return fmt.Sprintf("Unknown(%d)", int(t))
	}
}

// Value is the fundamental database-level value abstraction.
// It represents any storable value, decoupled from any specific serialization format.
//
// The data field holds the underlying Go value:
//   - TypeNull:    nil
//   - TypeBoolean: bool
//   - TypeNumber:  float64
//   - TypeString:  string
//   - TypeArray:   []Value
//   - TypeObject:  map[string]Value
type Value struct {
	typ  ValueType
	data interface{}
}

// Type returns the ValueType of this value.
func (v Value) Type() ValueType {
	return v.typ
}

// IsNull returns true if the value is Null.
func (v Value) IsNull() bool {
	return v.typ == TypeNull
}

// --- Constructors ---

// NewNull creates a Null value.
func NewNull() Value {
	return Value{typ: TypeNull, data: nil}
}

// NewBoolean creates a Boolean value.
func NewBoolean(b bool) Value {
	return Value{typ: TypeBoolean, data: b}
}

// NewNumber creates a Number value.
func NewNumber(n float64) Value {
	return Value{typ: TypeNumber, data: n}
}

// NewString creates a String value.
func NewString(s string) Value {
	return Value{typ: TypeString, data: s}
}

// NewArray creates an Array value from a slice of Values.
// The input slice is copied to prevent external mutation.
func NewArray(items []Value) Value {
	cp := make([]Value, len(items))
	copy(cp, items)
	return Value{typ: TypeArray, data: cp}
}

// NewObject creates an Object value from a map of string to Value.
// The input map is copied to prevent external mutation.
func NewObject(fields map[string]Value) Value {
	cp := make(map[string]Value, len(fields))
	for k, v := range fields {
		cp[k] = v
	}
	return Value{typ: TypeObject, data: cp}
}

// --- Accessors ---

// Boolean returns the bool and true if the value is a Boolean, or false, false otherwise.
func (v Value) Boolean() (bool, bool) {
	if v.typ != TypeBoolean {
		return false, false
	}
	return v.data.(bool), true
}

// Number returns the float64 and true if the value is a Number, or 0, false otherwise.
func (v Value) Number() (float64, bool) {
	if v.typ != TypeNumber {
		return 0, false
	}
	return v.data.(float64), true
}

// String returns the string and true if the value is a String, or "", false otherwise.
// Note: this shadows the fmt.Stringer interface intentionally for type consistency.
func (v Value) String() (string, bool) {
	if v.typ != TypeString {
		return "", false
	}
	return v.data.(string), true
}

// Array returns a copy of the array and true if the value is an Array, or nil, false otherwise.
func (v Value) Array() ([]Value, bool) {
	if v.typ != TypeArray {
		return nil, false
	}
	src := v.data.([]Value)
	cp := make([]Value, len(src))
	copy(cp, src)
	return cp, true
}

// Object returns a copy of the object map and true if the value is an Object, or nil, false otherwise.
func (v Value) Object() (map[string]Value, bool) {
	if v.typ != TypeObject {
		return nil, false
	}
	src := v.data.(map[string]Value)
	cp := make(map[string]Value, len(src))
	for k, val := range src {
		cp[k] = val
	}
	return cp, true
}

// NewReference creates a reference to a database key. Integrity is checked on write.
func NewReference(key Key) Value { return Value{typ: TypeReference, data: key} }

// Reference returns the target key if this value is a Reference.
func (v Value) Reference() (Key, bool) {
	if v.typ != TypeReference {
		return "", false
	}
	return v.data.(string), true
}
