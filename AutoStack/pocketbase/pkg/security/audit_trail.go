package security

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// AuditEventType classifies the kind of security-relevant event.
type AuditEventType string

const (
	AuditEventAuthSuccess     AuditEventType = "auth.success"
	AuditEventAuthFailure     AuditEventType = "auth.failure"
	AuditEventAuthExpired     AuditEventType = "auth.expired"
	AuditEventTokenIssued     AuditEventType = "token.issued"
	AuditEventTokenRevoked    AuditEventType = "token.revoked"
	AuditEventRateLimited     AuditEventType = "rate.limited"
	AuditEventPermDenied      AuditEventType = "perm.denied"
	AuditEventPermGranted     AuditEventType = "perm.granted"
	AuditEventGovernanceAct   AuditEventType = "governance.action"
	AuditEventExportRequested AuditEventType = "export.requested"
	AuditEventReplayRequested AuditEventType = "replay.requested"
	AuditEventCertAccessed    AuditEventType = "certification.accessed"
	AuditEventIsolationCheck  AuditEventType = "tenant.isolation_check"
)

// AuditEntry is an immutable, tamper-evident record of a security event.
// Raw credentials are never included in any field.
type AuditEntry struct {
	EntryID    string         `json:"entry_id"`
	EventType  AuditEventType `json:"event_type"`
	TenantID   string         `json:"tenant_id"`
	RequestID  string         `json:"request_id"`
	ActorRole  string         `json:"actor_role"`
	Permission string         `json:"permission,omitempty"`
	Outcome    string         `json:"outcome"`        // "allowed" | "denied" | "error"
	Reason     string         `json:"reason,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	OccurredAt string         `json:"occurred_at"`    // RFC3339 UTC — excluded from hash
	EntryHash  string         `json:"entry_hash"`     // SHA-256 of stable fields
	ChainHash  string         `json:"chain_hash"`     // SHA-256(prev.EntryHash + this.EntryHash)
}

// AuditTrail is an append-only, hash-chained sequence of AuditEntries.
// Entries are never deleted or modified after insertion.
type AuditTrail struct {
	entries []AuditEntry
}

// NewAuditTrail creates an empty audit trail.
func NewAuditTrail() *AuditTrail {
	return &AuditTrail{}
}

// Record appends a new event to the trail.
// entryID must be unique; tenantID must be non-empty.
// Credentials must never appear in metadata values.
func (t *AuditTrail) Record(entryID string, eventType AuditEventType, tenantID, requestID, actorRole, outcome string, opts ...AuditOption) AuditEntry {
	cfg := &auditConfig{}
	for _, o := range opts {
		o(cfg)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	entry := AuditEntry{
		EntryID:    entryID,
		EventType:  eventType,
		TenantID:   tenantID,
		RequestID:  requestID,
		ActorRole:  actorRole,
		Permission: cfg.permission,
		Outcome:    outcome,
		Reason:     cfg.reason,
		Metadata:   cfg.metadata,
		OccurredAt: now,
	}
	entry.EntryHash = computeAuditEntryHash(entry)
	entry.ChainHash = computeChainHash(t.tailHash(), entry.EntryHash)

	t.entries = append(t.entries, entry)
	return entry
}

// Entries returns a read-only copy of all audit entries in order.
func (t *AuditTrail) Entries() []AuditEntry {
	out := make([]AuditEntry, len(t.entries))
	copy(out, t.entries)
	return out
}

// EntriesFor returns entries for a specific tenant, in order.
func (t *AuditTrail) EntriesFor(tenantID string) []AuditEntry {
	var out []AuditEntry
	for _, e := range t.entries {
		if e.TenantID == tenantID {
			out = append(out, e)
		}
	}
	return out
}

// VerifyIntegrity checks that the chain hashes are consistent across all entries.
// Returns (true, "") if the chain is intact; (false, reason) on first violation.
func (t *AuditTrail) VerifyIntegrity() (bool, string) {
	prevHash := ""
	for i, e := range t.entries {
		expectedEntry := computeAuditEntryHash(e)
		if e.EntryHash != expectedEntry {
			return false, fmt.Sprintf("entry %d (%s): entry hash mismatch", i, e.EntryID)
		}
		expectedChain := computeChainHash(prevHash, e.EntryHash)
		if e.ChainHash != expectedChain {
			return false, fmt.Sprintf("entry %d (%s): chain hash broken", i, e.EntryID)
		}
		prevHash = e.EntryHash
	}
	return true, ""
}

// Len returns the number of audit entries.
func (t *AuditTrail) Len() int {
	return len(t.entries)
}

func (t *AuditTrail) tailHash() string {
	if len(t.entries) == 0 {
		return ""
	}
	return t.entries[len(t.entries)-1].EntryHash
}

// AuditOption configures optional fields on an audit record.
type AuditOption func(*auditConfig)

type auditConfig struct {
	permission string
	reason     string
	metadata   map[string]string
}

// WithPermission attaches a permission name to the audit record.
func WithPermission(perm Permission) AuditOption {
	return func(c *auditConfig) { c.permission = string(perm) }
}

// WithReason attaches a human-readable reason to the audit record.
func WithReason(reason string) AuditOption {
	return func(c *auditConfig) { c.reason = reason }
}

// WithMetadata attaches non-credential metadata to the audit record.
// Never include token values, passwords, or secrets in metadata.
func WithMetadata(kv map[string]string) AuditOption {
	return func(c *auditConfig) {
		m := make(map[string]string, len(kv))
		for k, v := range kv {
			m[k] = v
		}
		c.metadata = m
	}
}

// computeAuditEntryHash produces a deterministic hash of stable entry fields.
// OccurredAt is excluded (timestamp) as is EntryHash and ChainHash (self-referential).
func computeAuditEntryHash(e AuditEntry) string {
	type stableEntry struct {
		EntryID    string            `json:"entry_id"`
		EventType  AuditEventType    `json:"event_type"`
		TenantID   string            `json:"tenant_id"`
		RequestID  string            `json:"request_id"`
		ActorRole  string            `json:"actor_role"`
		Permission string            `json:"permission"`
		Outcome    string            `json:"outcome"`
		Reason     string            `json:"reason"`
		Metadata   map[string]string `json:"metadata"`
	}
	// Sort metadata keys for determinism.
	var sortedMeta map[string]string
	if len(e.Metadata) > 0 {
		sortedMeta = e.Metadata
		keys := make([]string, 0, len(sortedMeta))
		for k := range sortedMeta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		_ = keys // json.Marshal preserves map key order via sorted representation below
	}
	stable := stableEntry{
		EntryID:    e.EntryID,
		EventType:  e.EventType,
		TenantID:   e.TenantID,
		RequestID:  e.RequestID,
		ActorRole:  e.ActorRole,
		Permission: e.Permission,
		Outcome:    e.Outcome,
		Reason:     e.Reason,
		Metadata:   sortedMeta,
	}
	b, _ := json.Marshal(stable)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func computeChainHash(prevHash, entryHash string) string {
	h := sha256.Sum256([]byte(prevHash + "|" + entryHash))
	return hex.EncodeToString(h[:])
}
