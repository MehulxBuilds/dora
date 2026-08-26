package engine

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// jsonStorage implements Storage by reading/writing a single JSON file.
//
// File format: a JSON object where keys are database keys and values are
// JSON-encoded Values.
//
//	{
//	    "user:1": {"name": "Mehul", "age": 20},
//	    "counter": 42,
//	    "active": true
//	}
//
// On Save, the file is written atomically: data is written to a temporary
// file in the same directory, then renamed over the target. This prevents
// partial writes from corrupting the database file.
type jsonStorage struct {
	path string
}

// newJSONStorage creates a jsonStorage for the given file path.
func newJSONStorage(path string) *jsonStorage {
	return &jsonStorage{path: path}
}

// Load reads and decodes the JSON file into a map of Key→Value.
// If the file does not exist, returns an empty map (new database).
func (s *jsonStorage) Load() (map[Key]Value, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// File doesn't exist yet — this is a new database.
			return make(map[Key]Value), nil
		}
		return nil, err
	}

	// Empty file is treated as an empty database.
	if len(data) == 0 {
		return make(map[Key]Value), nil
	}

	var kvMap map[Key]Value
	if err := json.Unmarshal(data, &kvMap); err != nil {
		return nil, err
	}

	// Ensure non-nil map even if JSON was "{}".
	if kvMap == nil {
		kvMap = make(map[Key]Value)
	}

	return kvMap, nil
}

// Save encodes the key-value map as JSON and writes it to disk atomically.
//
// Atomic write strategy:
//  1. Write to a temporary file in the same directory.
//  2. Rename the temp file over the target file.
//
// This ensures the database file is never in a partially-written state.
func (s *jsonStorage) Save(data map[Key]Value) error {
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	// Ensure the parent directory exists.
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Write to a temporary file in the same directory.
	tmp, err := os.CreateTemp(dir, "nativedb-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(encoded); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	// Atomic rename over the target file.
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return err
	}

	return nil
}

// Close is a no-op for jsonStorage (no persistent file handles).
func (s *jsonStorage) Close() error {
	return nil
}
