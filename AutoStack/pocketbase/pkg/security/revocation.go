package security

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// TokenRevocationList is an append-only, restart-safe store of revoked token IDs.
// Once a token ID is revoked, it cannot be un-revoked.
//
// Revocation survives restart when backed by a PersistentRevocationStore.
// In-process revocation uses InProcessRevocationList for tests.
type TokenRevocationList interface {
	// Revoke adds a token ID to the revocation list.
	Revoke(tokenID, tenantID, reason string) error
	// IsRevoked returns true if the token ID has been revoked.
	IsRevoked(tokenID string) (bool, error)
	// Count returns the number of revoked tokens.
	Count() (int64, error)
}

// InProcessRevocationList is an in-memory revocation list for tests.
// Not restart-safe — use PersistentRevocationStore in production.
type InProcessRevocationList struct {
	mu      sync.RWMutex
	revoked map[string]revocationEntry
}

type revocationEntry struct {
	TokenID   string
	TenantID  string
	Reason    string
	RevokedAt string
}

// NewInProcessRevocationList creates an in-memory revocation list.
func NewInProcessRevocationList() *InProcessRevocationList {
	return &InProcessRevocationList{revoked: make(map[string]revocationEntry)}
}

// Revoke adds the token ID to the in-memory revocation list.
func (r *InProcessRevocationList) Revoke(tokenID, tenantID, reason string) error {
	if tokenID == "" {
		return fmt.Errorf("tokenID must not be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.revoked[tokenID]; exists {
		return nil // idempotent — already revoked
	}
	r.revoked[tokenID] = revocationEntry{
		TokenID:   tokenID,
		TenantID:  tenantID,
		Reason:    reason,
		RevokedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	return nil
}

// IsRevoked returns true if the token ID has been revoked.
func (r *InProcessRevocationList) IsRevoked(tokenID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.revoked[tokenID]
	return ok, nil
}

// Count returns the number of revoked tokens.
func (r *InProcessRevocationList) Count() (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.revoked)), nil
}

// PersistentRevocationStore is a SQLite-backed, restart-safe revocation list.
type PersistentRevocationStore struct {
	mu sync.Mutex
	db *sql.DB
}

// NewPersistentRevocationStore opens (or creates) a SQLite revocation store at path.
func NewPersistentRevocationStore(path string) (*PersistentRevocationStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open revocation store %q: %w", path, err)
	}
	if path != ":memory:" {
		if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;`); err != nil {
			db.Close()
			return nil, fmt.Errorf("set WAL mode: %w", err)
		}
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS revoked_tokens (
			token_id   TEXT PRIMARY KEY,
			tenant_id  TEXT NOT NULL,
			reason     TEXT,
			revoked_at TEXT NOT NULL
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create revocation table: %w", err)
	}
	return &PersistentRevocationStore{db: db}, nil
}

// Revoke persists the token ID as revoked. Idempotent — re-revoking the same token is a no-op.
func (s *PersistentRevocationStore) Revoke(tokenID, tenantID, reason string) error {
	if tokenID == "" {
		return fmt.Errorf("tokenID must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO revoked_tokens (token_id, tenant_id, reason, revoked_at)
		VALUES (?, ?, ?, ?)`,
		tokenID, tenantID, reason, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

// IsRevoked returns true if the token ID is in the revocation store.
func (s *PersistentRevocationStore) IsRevoked(tokenID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM revoked_tokens WHERE token_id=?`, tokenID).Scan(&n)
	return n > 0, err
}

// Count returns the number of revoked tokens.
func (s *PersistentRevocationStore) Count() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM revoked_tokens`).Scan(&n)
	return n, err
}

// Close closes the underlying database.
func (s *PersistentRevocationStore) Close() error {
	return s.db.Close()
}

// VerifyJWTWithRevocation validates a JWT and additionally checks the revocation list.
// A valid but revoked token returns AuthResult{Valid: false, Reason: "token revoked"}.
// Credentials are never logged.
func VerifyJWTWithRevocation(tokenStr, secret, expectedTenantID string, rl TokenRevocationList) AuthResult {
	result := VerifyJWT(tokenStr, secret, expectedTenantID)
	if !result.Valid {
		return result
	}
	if result.Claims.TokenID == "" {
		return result // no token ID in claims; cannot check revocation
	}
	revoked, err := rl.IsRevoked(result.Claims.TokenID)
	if err != nil {
		return AuthResult{Valid: false, Reason: "revocation check failed"}
	}
	if revoked {
		return AuthResult{Valid: false, Reason: "token revoked"}
	}
	return result
}

// KeyRotationRecord documents a key rotation event. The old and new secrets are
// never stored — only the rotation metadata is recorded.
type KeyRotationRecord struct {
	RotationID   string
	TenantID     string
	RotatedAt    string
	RevokedCount int // tokens invalidated as part of this rotation
}

// PerformKeyRotation revokes all provided token IDs (representing tokens issued
// under the old secret) and records the rotation event.
// The old and new secrets are not stored here — the caller manages secrets.
func PerformKeyRotation(rotationID, tenantID string, oldTokenIDs []string, rl TokenRevocationList) (KeyRotationRecord, error) {
	revokedCount := 0
	for _, tokenID := range oldTokenIDs {
		if err := rl.Revoke(tokenID, tenantID, fmt.Sprintf("key-rotation:%s", rotationID)); err != nil {
			return KeyRotationRecord{}, fmt.Errorf("revoke token %q during rotation %s: %w", tokenID, rotationID, err)
		}
		revokedCount++
	}
	return KeyRotationRecord{
		RotationID:   rotationID,
		TenantID:     tenantID,
		RotatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		RevokedCount: revokedCount,
	}, nil
}
