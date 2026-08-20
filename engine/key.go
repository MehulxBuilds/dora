package engine

// Key is the fundamental key type for the database.
// Keys identify values within the keyspace.
//
// Currently a simple string. This typedef exists so the key representation
// can evolve (validation, normalization, composite keys) without changing
// every call site in the codebase.
type Key = string
