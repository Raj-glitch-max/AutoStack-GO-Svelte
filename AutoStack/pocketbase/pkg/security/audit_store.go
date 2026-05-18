package security

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// PersistentAuditStore is an append-only, hash-chained, restart-safe audit store.
// It backs the AuditTrail with a SQLite WAL database so evidence survives restarts.
//
// Invariants preserved across restarts:
//   - Chain hash continuity: every entry's ChainHash chains from the previous
//   - No entry is ever deleted or updated
//   - Retrieval order is insertion order (ROWID ascending)
type PersistentAuditStore struct {
	mu  sync.Mutex
	db  *sql.DB
	dsn string
}

// NewSQLiteAuditStore opens (or creates) a SQLite WAL audit store at path.
// Use ":memory:" for tests — WAL pragmas are omitted for in-memory DBs.
func NewSQLiteAuditStore(path string) (*PersistentAuditStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open audit store %q: %w", path, err)
	}
	if path != ":memory:" {
		if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;`); err != nil {
			db.Close()
			return nil, fmt.Errorf("set WAL mode: %w", err)
		}
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_entries (
			rowid       INTEGER PRIMARY KEY AUTOINCREMENT,
			entry_id    TEXT NOT NULL UNIQUE,
			tenant_id   TEXT NOT NULL,
			event_type  TEXT NOT NULL,
			request_id  TEXT NOT NULL,
			actor_role  TEXT NOT NULL,
			permission  TEXT,
			outcome     TEXT NOT NULL,
			reason      TEXT,
			metadata    TEXT,
			occurred_at TEXT NOT NULL,
			entry_hash  TEXT NOT NULL,
			chain_hash  TEXT NOT NULL
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create audit table: %w", err)
	}
	return &PersistentAuditStore{db: db, dsn: path}, nil
}

// Append writes a new audit entry. EntryID must be unique.
// The entry must already have EntryHash and ChainHash set (use AuditTrail.Record or compute them directly).
func (s *PersistentAuditStore) Append(entry AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _ := json.Marshal(entry.Metadata)
	_, err := s.db.Exec(`
		INSERT INTO audit_entries
			(entry_id, tenant_id, event_type, request_id, actor_role, permission,
			 outcome, reason, metadata, occurred_at, entry_hash, chain_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.EntryID, entry.TenantID, string(entry.EventType), entry.RequestID,
		entry.ActorRole, entry.Permission, entry.Outcome, entry.Reason,
		string(meta), entry.OccurredAt, entry.EntryHash, entry.ChainHash,
	)
	if err != nil {
		return fmt.Errorf("append audit entry %q: %w", entry.EntryID, err)
	}
	return nil
}

// AllEntries returns all audit entries in insertion order.
func (s *PersistentAuditStore) AllEntries() ([]AuditEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queryEntries(`SELECT entry_id, tenant_id, event_type, request_id, actor_role,
		permission, outcome, reason, metadata, occurred_at, entry_hash, chain_hash
		FROM audit_entries ORDER BY rowid ASC`)
}

// EntriesForTenant returns entries for a specific tenant, in insertion order.
func (s *PersistentAuditStore) EntriesForTenant(tenantID string) ([]AuditEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queryEntries(`SELECT entry_id, tenant_id, event_type, request_id, actor_role,
		permission, outcome, reason, metadata, occurred_at, entry_hash, chain_hash
		FROM audit_entries WHERE tenant_id = ? ORDER BY rowid ASC`, tenantID)
}

// LatestChainHash returns the chain_hash of the most recently inserted entry.
// Returns "" if the store is empty.
func (s *PersistentAuditStore) LatestChainHash() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var h string
	err := s.db.QueryRow(`SELECT chain_hash FROM audit_entries ORDER BY rowid DESC LIMIT 1`).Scan(&h)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return h, err
}

// Count returns the total number of audit entries in the store.
func (s *PersistentAuditStore) Count() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_entries`).Scan(&n)
	return n, err
}

// Close closes the underlying database.
func (s *PersistentAuditStore) Close() error {
	return s.db.Close()
}

// VerifyAuditChain reads all entries from the store and verifies:
//   - every EntryHash matches its recomputed hash
//   - every ChainHash correctly chains from the previous entry
//   - entries are in monotonically non-decreasing OccurredAt order
//
// Returns (true, "") if the chain is intact; (false, reason) on first violation.
func VerifyAuditChain(store *PersistentAuditStore) (bool, string) {
	entries, err := store.AllEntries()
	if err != nil {
		return false, fmt.Sprintf("failed to read entries: %v", err)
	}

	prevHash := ""
	prevOccurredAt := ""

	for i, e := range entries {
		// Verify entry hash.
		expectedEntry := computeAuditEntryHash(e)
		if e.EntryHash != expectedEntry {
			return false, fmt.Sprintf("entry %d (%s): entry hash mismatch", i, e.EntryID)
		}

		// Verify chain hash.
		expectedChain := computeChainHash(prevHash, e.EntryHash)
		if e.ChainHash != expectedChain {
			return false, fmt.Sprintf("entry %d (%s): chain hash broken", i, e.EntryID)
		}

		// Verify OccurredAt is not going backwards (monotonic timestamps).
		if prevOccurredAt != "" && e.OccurredAt < prevOccurredAt {
			return false, fmt.Sprintf("entry %d (%s): timestamp reversal (prev=%s cur=%s)",
				i, e.EntryID, prevOccurredAt, e.OccurredAt)
		}

		prevHash = e.EntryHash
		prevOccurredAt = e.OccurredAt
	}
	return true, ""
}

// DurableAuditRecorder wraps an in-process AuditTrail and a PersistentAuditStore
// so every audit.Record() call is written to both the in-memory trail and the store.
// On startup, call Restore() to replay persisted entries into the trail.
type DurableAuditRecorder struct {
	trail *AuditTrail
	store *PersistentAuditStore
}

// NewDurableAuditRecorder creates a recorder backed by the given store.
// Call Restore() before using to load persisted entries into the in-process trail.
func NewDurableAuditRecorder(store *PersistentAuditStore) *DurableAuditRecorder {
	return &DurableAuditRecorder{
		trail: NewAuditTrail(),
		store: store,
	}
}

// Restore loads all persisted entries from the store into the in-process trail.
// Must be called once on startup before any Record() calls.
func (r *DurableAuditRecorder) Restore() error {
	entries, err := r.store.AllEntries()
	if err != nil {
		return fmt.Errorf("restore audit trail: %w", err)
	}
	for _, e := range entries {
		r.trail.entries = append(r.trail.entries, e)
	}
	return nil
}

// Record writes a new event to both the in-memory trail and the persistent store.
func (r *DurableAuditRecorder) Record(entryID string, eventType AuditEventType, tenantID, requestID, actorRole, outcome string, opts ...AuditOption) (AuditEntry, error) {
	entry := r.trail.Record(entryID, eventType, tenantID, requestID, actorRole, outcome, opts...)
	if err := r.store.Append(entry); err != nil {
		return AuditEntry{}, fmt.Errorf("persist audit entry: %w", err)
	}
	return entry, nil
}

// VerifyIntegrity checks both in-process and persisted chain integrity.
func (r *DurableAuditRecorder) VerifyIntegrity() (bool, string) {
	// Check in-process chain.
	if ok, reason := r.trail.VerifyIntegrity(); !ok {
		return false, "in-process: " + reason
	}
	// Check persisted chain.
	return VerifyAuditChain(r.store)
}

// Entries returns a copy of all in-memory entries.
func (r *DurableAuditRecorder) Entries() []AuditEntry {
	return r.trail.Entries()
}

func (s *PersistentAuditStore) queryEntries(query string, args ...interface{}) ([]AuditEntry, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var metaStr string
		var eventType string
		if err := rows.Scan(&e.EntryID, &e.TenantID, &eventType, &e.RequestID, &e.ActorRole,
			&e.Permission, &e.Outcome, &e.Reason, &metaStr, &e.OccurredAt, &e.EntryHash, &e.ChainHash); err != nil {
			return nil, err
		}
		e.EventType = AuditEventType(eventType)
		if metaStr != "" && metaStr != "null" {
			json.Unmarshal([]byte(metaStr), &e.Metadata)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// exportableEntry is used for JSON export of audit entries — never includes raw secrets.
type exportableEntry struct {
	EntryID    string `json:"entry_id"`
	EventType  string `json:"event_type"`
	TenantID   string `json:"tenant_id"`
	RequestID  string `json:"request_id"`
	ActorRole  string `json:"actor_role"`
	Permission string `json:"permission,omitempty"`
	Outcome    string `json:"outcome"`
	Reason     string `json:"reason,omitempty"`
	OccurredAt string `json:"occurred_at"`
	EntryHash  string `json:"entry_hash"`
	ChainHash  string `json:"chain_hash"`
}

// ExportTenantAudit returns a JSON-serializable slice of audit entries for a tenant.
// Metadata is excluded from exports to avoid potential credential leakage.
func ExportTenantAudit(store *PersistentAuditStore, tenantID string) ([]exportableEntry, error) {
	entries, err := store.EntriesForTenant(tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]exportableEntry, len(entries))
	for i, e := range entries {
		out[i] = exportableEntry{
			EntryID:    e.EntryID,
			EventType:  string(e.EventType),
			TenantID:   e.TenantID,
			RequestID:  e.RequestID,
			ActorRole:  e.ActorRole,
			Permission: e.Permission,
			Outcome:    e.Outcome,
			Reason:     e.Reason,
			OccurredAt: e.OccurredAt,
			EntryHash:  e.EntryHash,
			ChainHash:  e.ChainHash,
		}
	}
	return out, nil
}

// nowRFC3339 returns the current time in RFC3339Nano UTC for test helper use.
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
