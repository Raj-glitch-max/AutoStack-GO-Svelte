// Package backup provides snapshot, archive, and restore primitives for the
// AutoStack platform. Backups are append-only evidence snapshots — never
// in-place overwriting of existing data. Restore operations validate hash
// integrity before applying any state.
//
// Invariants:
//   - Restore is refused if the backup hash does not match the stored content.
//   - Backups contain no credentials or secret values.
//   - All backup manifests are content-identity-hashed (timestamps excluded).
package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// BackupKind classifies what kind of data is captured in a backup.
type BackupKind string

const (
	BackupKindPlatformSnapshot  BackupKind = "platform_snapshot"
	BackupKindArchiveSnapshot   BackupKind = "archive_snapshot"
	BackupKindReplayManifest    BackupKind = "replay_manifest"
	BackupKindGovernanceState   BackupKind = "governance_state"
	BackupKindCertification     BackupKind = "certification"
)

// BackupManifest describes a single backup artifact.
// CreatedAt is excluded from ManifestHash (content-identity).
type BackupManifest struct {
	BackupID      string     `json:"backup_id"`
	Kind          BackupKind `json:"kind"`
	TenantID      string     `json:"tenant_id"`
	ExecutionID   string     `json:"execution_id"`
	EntryCount    int        `json:"entry_count"`
	SizeBytes     int64      `json:"size_bytes"`
	ContentHash   string     `json:"content_hash"`   // hash of the payload bytes
	ManifestHash  string     `json:"manifest_hash"`  // hash of stable manifest fields
	CreatedAt     string     `json:"created_at"`     // excluded from ManifestHash
	SchemaVersion string     `json:"schema_version"`
}

// BackupPayload is the serialized content of a backup.
type BackupPayload struct {
	Manifest BackupManifest `json:"manifest"`
	Data     []byte         `json:"data"` // JSON-encoded subsystem state
}

// RestoreResult describes the outcome of a restore operation.
type RestoreResult struct {
	BackupID        string
	Kind            BackupKind
	Succeeded       bool
	EntriesRestored int
	Reason          string // non-empty on failure
}

// BackupRegistry is an append-only store of backup manifests.
// The actual payload bytes are stored externally (object storage, filesystem).
type BackupRegistry struct {
	manifests []BackupManifest
}

// NewBackupRegistry creates an empty registry.
func NewBackupRegistry() *BackupRegistry {
	return &BackupRegistry{}
}

// Register adds a manifest to the registry. backupID must be unique.
func (r *BackupRegistry) Register(m BackupManifest) error {
	for _, existing := range r.manifests {
		if existing.BackupID == m.BackupID {
			return fmt.Errorf("backup %q already registered (append-only)", m.BackupID)
		}
	}
	r.manifests = append(r.manifests, m)
	return nil
}

// Get returns the manifest for a given backupID.
func (r *BackupRegistry) Get(backupID string) (BackupManifest, bool) {
	for _, m := range r.manifests {
		if m.BackupID == backupID {
			return m, true
		}
	}
	return BackupManifest{}, false
}

// All returns a copy of all registered manifests.
func (r *BackupRegistry) All() []BackupManifest {
	out := make([]BackupManifest, len(r.manifests))
	copy(out, r.manifests)
	return out
}

// LatestForKind returns the most recently created manifest of a given kind.
func (r *BackupRegistry) LatestForKind(kind BackupKind, tenantID string) (BackupManifest, bool) {
	var latest BackupManifest
	found := false
	for _, m := range r.manifests {
		if m.Kind == kind && m.TenantID == tenantID {
			if !found || m.CreatedAt > latest.CreatedAt {
				latest = m
				found = true
			}
		}
	}
	return latest, found
}

// CreateBackupManifest builds and signs a BackupManifest for the given payload.
// The payload bytes are content-hashed; CreatedAt is excluded from ManifestHash.
func CreateBackupManifest(backupID, tenantID, executionID, schemaVersion string,
	kind BackupKind, payload []byte) BackupManifest {

	contentHash := hashBytes(payload)
	m := BackupManifest{
		BackupID:      backupID,
		Kind:          kind,
		TenantID:      tenantID,
		ExecutionID:   executionID,
		EntryCount:    0, // set by caller after parsing payload
		SizeBytes:     int64(len(payload)),
		ContentHash:   contentHash,
		SchemaVersion: schemaVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	m.ManifestHash = computeManifestHash(m)
	return m
}

// VerifyBackupManifest confirms the manifest hash is consistent with its content hash.
// Returns (true, "") if valid; (false, reason) on failure.
func VerifyBackupManifest(m BackupManifest) (bool, string) {
	expected := computeManifestHash(m)
	if m.ManifestHash != expected {
		return false, fmt.Sprintf("backup %q: manifest hash mismatch (stored=%s computed=%s)",
			m.BackupID, m.ManifestHash, expected)
	}
	return true, ""
}

// VerifyPayloadIntegrity checks that the payload bytes match the ContentHash in the manifest.
func VerifyPayloadIntegrity(m BackupManifest, payload []byte) (bool, string) {
	computed := hashBytes(payload)
	if m.ContentHash != computed {
		return false, fmt.Sprintf("backup %q: content hash mismatch (stored=%s computed=%s)",
			m.BackupID, m.ContentHash, computed)
	}
	return true, ""
}

// RestorePlatformSnapshot validates a backup payload and returns a restore result.
// Restoration is refused if either the manifest hash or content hash fails verification.
// No state mutation occurs inside this function — the caller applies the restored data.
func RestorePlatformSnapshot(manifest BackupManifest, payload []byte) RestoreResult {
	result := RestoreResult{
		BackupID: manifest.BackupID,
		Kind:     manifest.Kind,
	}

	// Step 1: verify manifest integrity.
	if ok, reason := VerifyBackupManifest(manifest); !ok {
		result.Reason = "manifest integrity check failed: " + reason
		return result
	}

	// Step 2: verify payload matches content hash.
	if ok, reason := VerifyPayloadIntegrity(manifest, payload); !ok {
		result.Reason = "payload integrity check failed: " + reason
		return result
	}

	// Step 3: verify payload is parseable JSON (not silently corrupt).
	var parsed interface{}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		result.Reason = fmt.Sprintf("payload is not valid JSON: %v", err)
		return result
	}

	result.Succeeded = true
	result.EntriesRestored = manifest.EntryCount
	return result
}

func computeManifestHash(m BackupManifest) string {
	type stableManifest struct {
		BackupID      string     `json:"backup_id"`
		Kind          BackupKind `json:"kind"`
		TenantID      string     `json:"tenant_id"`
		ExecutionID   string     `json:"execution_id"`
		EntryCount    int        `json:"entry_count"`
		SizeBytes     int64      `json:"size_bytes"`
		ContentHash   string     `json:"content_hash"`
		SchemaVersion string     `json:"schema_version"`
	}
	stable := stableManifest{
		BackupID:      m.BackupID,
		Kind:          m.Kind,
		TenantID:      m.TenantID,
		ExecutionID:   m.ExecutionID,
		EntryCount:    m.EntryCount,
		SizeBytes:     m.SizeBytes,
		ContentHash:   m.ContentHash,
		SchemaVersion: m.SchemaVersion,
	}
	b, _ := json.Marshal(stable)
	return hashBytes(b)
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
