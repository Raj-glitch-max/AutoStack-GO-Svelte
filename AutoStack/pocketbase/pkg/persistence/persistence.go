// Package persistence defines the durable storage interface for all runtime
// subsystems. Implementations may be backed by PocketBase, PostgreSQL, or any
// append-only store. The in-memory implementation is provided for tests.
package persistence

import (
	"context"
	"fmt"
	"sync"
)

// StorageError is returned when a persistence boundary is violated.
type StorageError struct {
	Op  string
	Key string
	Err string
}

func (e StorageError) Error() string {
	return fmt.Sprintf("persistence: %s %q: %s", e.Op, e.Key, e.Err)
}

// DurableKVStore is the minimal key-value interface all subsystem stores are
// expressed in terms of. Keys are namespaced by the caller (e.g., "task:t-1").
// All writes are append-only: Put on an existing key must error unless the
// implementation explicitly supports update. Delete is not supported.
type DurableKVStore interface {
	// Put writes a value under key. Returns StorageError on duplicate key.
	Put(ctx context.Context, key string, value []byte) error
	// Get retrieves a value. Returns (nil, false, nil) when not found.
	Get(ctx context.Context, key string) ([]byte, bool, error)
	// List returns all values whose keys have the given prefix, in key order.
	List(ctx context.Context, prefix string) ([][]byte, error)
}

// InMemoryKVStore is a thread-safe, test-friendly DurableKVStore.
// It stores each key once (append-only: duplicate Put returns an error).
type InMemoryKVStore struct {
	mu   sync.RWMutex
	data map[string][]byte
	keys []string // insertion order
}

// NewInMemoryKVStore returns an empty store.
func NewInMemoryKVStore() *InMemoryKVStore {
	return &InMemoryKVStore{data: make(map[string][]byte)}
}

// Put stores a value. Returns StorageError on duplicate key.
func (s *InMemoryKVStore) Put(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.data[key]; exists {
		return StorageError{Op: "Put", Key: key, Err: "key already exists (append-only)"}
	}
	cp := make([]byte, len(value))
	copy(cp, value)
	s.data[key] = cp
	s.keys = append(s.keys, key)
	return nil
}

// Get retrieves a value. Returns (nil, false, nil) when not found.
func (s *InMemoryKVStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	if !ok {
		return nil, false, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, true, nil
}

// List returns all values whose keys have the given prefix, in insertion order.
func (s *InMemoryKVStore) List(_ context.Context, prefix string) ([][]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out [][]byte
	for _, k := range s.keys {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			cp := make([]byte, len(s.data[k]))
			copy(cp, s.data[k])
			out = append(out, cp)
		}
	}
	return out, nil
}

// Size returns the number of stored keys.
func (s *InMemoryKVStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}
