package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// SQLiteKVStore is a file-backed, append-only DurableKVStore backed by SQLite.
// It creates a single table: kv(key TEXT PRIMARY KEY, value BLOB NOT NULL).
// Append-only: Put on an existing key returns StorageError{Duplicate: true}.
// Safe for concurrent use within a single process.
type SQLiteKVStore struct {
	mu  sync.Mutex
	db  *sql.DB
	dsn string
}

// NewSQLiteKVStore opens (or creates) a SQLite database at path and
// initialises the kv table. Pass ":memory:" for an in-process test store.
func NewSQLiteKVStore(path string) (*SQLiteKVStore, error) {
	dsn := path
	if path != ":memory:" {
		// Enable WAL mode and fsync on every commit for durability.
		dsn = path + "?_journal_mode=WAL&_synchronous=FULL"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open %q: %w", path, err)
	}
	// Single connection — SQLite WAL handles concurrent readers.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS kv (
			key   TEXT NOT NULL PRIMARY KEY,
			value BLOB NOT NULL
		);
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite create table: %w", err)
	}
	return &SQLiteKVStore{db: db, dsn: dsn}, nil
}

// Put writes value under key. Returns StorageError on duplicate key.
func (s *SQLiteKVStore) Put(_ context.Context, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO kv (key, value) VALUES (?, ?)`, key, value)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") ||
			strings.Contains(err.Error(), "SQLITE_CONSTRAINT") {
			return StorageError{Op: "put", Key: key,
				Err: fmt.Sprintf("key %q already exists (append-only)", key)}
		}
		return fmt.Errorf("sqlite put %q: %w", key, err)
	}
	return nil
}

// Get retrieves the value for key. Returns (nil, false, nil) when not found.
func (s *SQLiteKVStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var value []byte
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("sqlite get %q: %w", key, err)
	}
	// Deep copy.
	cp := make([]byte, len(value))
	copy(cp, value)
	return cp, true, nil
}

// List returns all values whose keys have the given prefix, in key order.
func (s *SQLiteKVStore) List(_ context.Context, prefix string) ([][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT value FROM kv WHERE key LIKE ? ORDER BY key`,
		prefix+"%",
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite list %q: %w", prefix, err)
	}
	defer rows.Close()

	var out [][]byte
	for rows.Next() {
		var v []byte
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("sqlite scan: %w", err)
		}
		cp := make([]byte, len(v))
		copy(cp, v)
		out = append(out, cp)
	}
	return out, rows.Err()
}

// Close releases the database connection.
func (s *SQLiteKVStore) Close() error {
	return s.db.Close()
}

// Size returns the total number of stored keys.
func (s *SQLiteKVStore) Size() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM kv`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// DSN returns the data source name for diagnostics.
func (s *SQLiteKVStore) DSN() string { return s.dsn }
