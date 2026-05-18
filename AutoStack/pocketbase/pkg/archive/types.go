// Package archive implements the Phase 5.0 archival system.
// All entries are immutable once created — no mutation, no silent garbage
// collection, no rewriting of archived payloads.
//
// Invariants:
//   - PayloadHash is SHA-256 of the raw Payload bytes.
//   - Restoration verifies PayloadHash before returning any data.
//   - ArchiveManifest is tamper-evident via ManifestHash.
package archive

const ArchiveSchemaVersion = "5.0.0"

// ArchiveType classifies what kind of data is archived.
type ArchiveType string

const (
	ArchiveTypeExecution  ArchiveType = "execution"
	ArchiveTypeGovernance ArchiveType = "governance"
	ArchiveTypeTopology   ArchiveType = "topology"
	ArchiveTypeForensic   ArchiveType = "forensic"
	ArchiveTypeReplay     ArchiveType = "replay"
	ArchiveTypeExport     ArchiveType = "export"
	ArchiveTypeEvent      ArchiveType = "event"
)

// ArchiveEntry is one immutable archived payload with checksum verification.
type ArchiveEntry struct {
	EntryID       string      `json:"entry_id"`
	ArchiveType   ArchiveType `json:"archive_type"`
	ExecutionID   string      `json:"execution_id"`
	TenantID      string      `json:"tenant_id"`
	SchemaVersion string      `json:"schema_version"`
	// Payload is the archived bytes (e.g. JSON-encoded snapshot).
	Payload []byte `json:"payload"`
	// PayloadHash is SHA-256 of Payload. Computed at creation time.
	PayloadHash string `json:"payload_hash"`
	ArchivedAt  string `json:"archived_at"`
	// Restorable indicates whether restoration is supported for this entry type.
	Restorable bool `json:"restorable"`
}

// ArchiveManifest is a tamper-evident index of ArchiveEntries for one tenant.
// ManifestHash covers all stable fields — GeneratedAt excluded.
type ArchiveManifest struct {
	ManifestID    string         `json:"manifest_id"`
	TenantID      string         `json:"tenant_id"`
	SchemaVersion string         `json:"schema_version"`
	Entries       []ArchiveEntry `json:"entries"`
	ManifestHash  string         `json:"manifest_hash"`
	GeneratedAt   string         `json:"generated_at"`
}
