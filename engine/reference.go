package engine

import (
	"fmt"
	"sort"
)

// validateReferences checks a proposed final state, before any WAL write.
// Sorting makes the first reported violation deterministic.
func validateReferences(data map[Key]Value, deleted map[Key]bool) error {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, source := range keys {
		err := walkReferences(data[source], func(target Key) error {
			if target != "" {
				if _, ok := data[target]; ok {
					return nil
				}
			}
			reason := ErrReferenceNotFound
			if deleted[target] {
				reason = ErrKeyReferenced
			}
			return fmt.Errorf("key %q references %q: %w", source, target, reason)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func walkReferences(v Value, visit func(Key) error) error {
	switch v.typ {
	case TypeReference:
		return visit(v.data.(string))
	case TypeArray:
		for _, item := range v.data.([]Value) {
			if err := walkReferences(item, visit); err != nil {
				return err
			}
		}
	case TypeObject:
		fields := v.data.(map[string]Value)
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := walkReferences(fields[key], visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func (db *DB) validateChanges(overlay map[Key]txOverlay) error {
	data := db.state.snapshot()
	deleted := make(map[Key]bool)
	for key, entry := range overlay {
		if entry.deleted {
			delete(data, key)
			deleted[key] = true
		} else {
			data[key] = entry.value
		}
	}
	return validateReferences(data, deleted)
}
