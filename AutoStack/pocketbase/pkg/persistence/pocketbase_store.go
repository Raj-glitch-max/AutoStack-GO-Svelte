package persistence

import (
	"context"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// PocketBaseKVStore is a DurableKVStore backed by the PocketBase application DB.
// It stores all keys in a single table: autostack_kv(key TEXT PRIMARY KEY, value BLOB).
// The table is created on first use via EnsurePocketBaseKVTable.
// Append-only: Put on an existing key returns StorageError{Duplicate: true}.
//
// This store is intended for production use with the existing PocketBase deployment.
// It reuses PocketBase's connection pool and transaction model.
type PocketBaseKVStore struct {
	app core.App
}

// NewPocketBaseKVStore returns a PocketBaseKVStore using the provided PocketBase App.
// Call EnsurePocketBaseKVTable before first use.
func NewPocketBaseKVStore(app core.App) *PocketBaseKVStore {
	return &PocketBaseKVStore{app: app}
}

// EnsurePocketBaseKVTable creates the autostack_kv table if it does not exist.
// Safe to call on every startup.
func EnsurePocketBaseKVTable(app core.App) error {
	_, err := app.DB().NewQuery(`
		CREATE TABLE IF NOT EXISTS autostack_kv (
			key   TEXT NOT NULL PRIMARY KEY,
			value BLOB NOT NULL
		)
	`).Execute()
	if err != nil {
		return fmt.Errorf("ensure autostack_kv table: %w", err)
	}
	return nil
}

// Put writes value under key. Returns StorageError{Duplicate: true} if key exists.
func (s *PocketBaseKVStore) Put(_ context.Context, key string, value []byte) error {
	_, err := s.app.DB().Insert("autostack_kv", dbx.Params{
		"key":   key,
		"value": value,
	}).Execute()
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "UNIQUE constraint") ||
			strings.Contains(msg, "SQLITE_CONSTRAINT") ||
			strings.Contains(msg, "duplicate") {
			return StorageError{Op: "put", Key: key,
				Err: fmt.Sprintf("key %q already exists (append-only)", key)}
		}
		return fmt.Errorf("pocketbase put %q: %w", key, err)
	}
	return nil
}

// Get retrieves the value for key. Returns (nil, false, nil) when not found.
func (s *PocketBaseKVStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	var row struct {
		Value []byte `db:"value"`
	}
	err := s.app.DB().
		NewQuery("SELECT value FROM autostack_kv WHERE key={:key}").
		Bind(dbx.Params{"key": key}).
		One(&row)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") || err.Error() == "sql: no rows in result set" {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("pocketbase get %q: %w", key, err)
	}
	cp := make([]byte, len(row.Value))
	copy(cp, row.Value)
	return cp, true, nil
}

// List returns all values whose keys have the given prefix, in key order.
func (s *PocketBaseKVStore) List(_ context.Context, prefix string) ([][]byte, error) {
	var rows []struct {
		Value []byte `db:"value"`
	}
	err := s.app.DB().
		NewQuery("SELECT value FROM autostack_kv WHERE key LIKE {:prefix} ORDER BY key").
		Bind(dbx.Params{"prefix": prefix + "%"}).
		All(&rows)
	if err != nil {
		return nil, fmt.Errorf("pocketbase list %q: %w", prefix, err)
	}
	out := make([][]byte, len(rows))
	for i, r := range rows {
		cp := make([]byte, len(r.Value))
		copy(cp, r.Value)
		out[i] = cp
	}
	return out, nil
}
