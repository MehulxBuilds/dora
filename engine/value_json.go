package engine

import (
	"encoding/json"
	"fmt"
)

// MarshalJSON implements json.Marshaler for Value.
//
// The mapping is direct and natural:
//
//	TypeNull    → null
//	TypeBoolean → true / false
//	TypeNumber  → number
//	TypeString  → "string"
//	TypeArray   → [...]
//	TypeObject  → {...}
//
// This keeps JSON serialization contained to this file.
// The rest of the engine never touches encoding/json.
func (v Value) MarshalJSON() ([]byte, error) {
	switch v.typ {
	case TypeNull:
		return []byte("null"), nil
	case TypeBoolean:
		return json.Marshal(v.data.(bool))
	case TypeNumber:
		return json.Marshal(v.data.(float64))
	case TypeString:
		return json.Marshal(v.data.(string))
	case TypeArray:
		return json.Marshal(v.data.([]Value))
	case TypeObject:
		return json.Marshal(v.data.(map[string]Value))
	default:
		return nil, fmt.Errorf("cannot marshal unknown ValueType(%d)", int(v.typ))
	}
}

// UnmarshalJSON implements json.Unmarshaler for Value.
//
// It uses json.Decoder with UseNumber to distinguish integers from floats
// where possible, but stores all numbers as float64 internally (matching
// the JSON specification and our Value model).
func (v *Value) UnmarshalJSON(data []byte) error {
	// Use the standard library to decode into interface{}, then convert.
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	result, err := valueFromInterface(raw)
	if err != nil {
		return err
	}
	*v = result
	return nil
}

// valueFromInterface recursively converts a json.Unmarshal result
// (interface{}) into our Value type.
//
// json.Unmarshal produces:
//
//	nil           → TypeNull
//	bool          → TypeBoolean
//	float64       → TypeNumber
//	string        → TypeString
//	[]interface{} → TypeArray
//	map[string]interface{} → TypeObject
func valueFromInterface(raw interface{}) (Value, error) {
	if raw == nil {
		return NewNull(), nil
	}
	switch v := raw.(type) {
	case bool:
		return NewBoolean(v), nil
	case float64:
		return NewNumber(v), nil
	case string:
		return NewString(v), nil
	case []interface{}:
		items := make([]Value, len(v))
		for i, item := range v {
			val, err := valueFromInterface(item)
			if err != nil {
				return NewNull(), err
			}
			items[i] = val
		}
		return NewArray(items), nil
	case map[string]interface{}:
		fields := make(map[string]Value, len(v))
		for k, item := range v {
			val, err := valueFromInterface(item)
			if err != nil {
				return NewNull(), err
			}
			fields[k] = val
		}
		return NewObject(fields), nil
	default:
		return NewNull(), fmt.Errorf("unexpected type %T during JSON unmarshal", raw)
	}
}
