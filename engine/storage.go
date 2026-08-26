package engine

// Storage is the persistence abstraction for the database engine.
//
// The engine interacts with persistence only through this interface.
// This decouples the logical database from any specific storage format
// or medium (JSON files, binary files, memory, network, etc.).
//
// Load reads the entire keyspace from the backing store.
// Save writes the entire keyspace to the backing store.
// Close releases any resources held by the storage (file handles, etc.).
type Storage interface {
	// Load reads all key-value pairs from the backing store.
	// Returns an empty map (not nil) if the store is empty or does not exist yet.
	// Returns an error if the store exists but cannot be read or is corrupt.
	Load() (map[Key]Value, error)

	// Save writes all key-value pairs to the backing store, replacing any
	// previous content entirely.
	Save(data map[Key]Value) error

	// Close releases any resources held by the storage implementation.
	Close() error
}

// memoryStorage is a no-op Storage implementation for in-memory databases.
// Load returns an empty map. Save and Close do nothing.
type memoryStorage struct{}

func (m *memoryStorage) Load() (map[Key]Value, error) {
	return make(map[Key]Value), nil
}

func (m *memoryStorage) Save(_ map[Key]Value) error {
	return nil
}

func (m *memoryStorage) Close() error {
	return nil
}
