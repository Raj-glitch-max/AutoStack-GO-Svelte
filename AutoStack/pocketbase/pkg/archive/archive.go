package archive

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// NewArchiveEntry creates an ArchiveEntry with a pre-computed PayloadHash.
// Payload is deep-copied so the caller cannot mutate the archived data.
func NewArchiveEntry(
	entryID string,
	archiveType ArchiveType,
	executionID, tenantID string,
	payload []byte,
) (ArchiveEntry, error) {
	if entryID == "" {
		return ArchiveEntry{}, fmt.Errorf("entryID must be non-empty")
	}
	if tenantID == "" {
		return ArchiveEntry{}, fmt.Errorf("tenantID must be non-empty")
	}
	if len(payload) == 0 {
		return ArchiveEntry{}, fmt.Errorf("payload must not be empty")
	}
	payloadCopy := append([]byte(nil), payload...)
	hash := computePayloadHash(payloadCopy)
	return ArchiveEntry{
		EntryID:       entryID,
		ArchiveType:   archiveType,
		ExecutionID:   executionID,
		TenantID:      tenantID,
		SchemaVersion: ArchiveSchemaVersion,
		Payload:       payloadCopy,
		PayloadHash:   hash,
		ArchivedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Restorable:    true,
	}, nil
}

// VerifyArchiveEntry recomputes PayloadHash and checks it matches the stored value.
func VerifyArchiveEntry(entry ArchiveEntry) (bool, string) {
	if entry.PayloadHash == "" {
		return false, fmt.Sprintf("entry %q has empty PayloadHash", entry.EntryID)
	}
	expected := computePayloadHash(entry.Payload)
	if entry.PayloadHash != expected {
		return false, fmt.Sprintf("entry %q PayloadHash mismatch: expected %q got %q",
			entry.EntryID, expected, entry.PayloadHash)
	}
	return true, ""
}

// RestoreArchiveEntry verifies the entry checksum then returns the payload.
// Returns an error if the payload is tampered or the entry is not restorable.
func RestoreArchiveEntry(entry ArchiveEntry) ([]byte, error) {
	if !entry.Restorable {
		return nil, fmt.Errorf("entry %q is not marked as restorable", entry.EntryID)
	}
	ok, reason := VerifyArchiveEntry(entry)
	if !ok {
		return nil, fmt.Errorf("restore %q: %s", entry.EntryID, reason)
	}
	return append([]byte(nil), entry.Payload...), nil
}

// BuildArchiveManifest constructs a tamper-evident manifest.
// ManifestHash covers all stable fields (GeneratedAt excluded).
func BuildArchiveManifest(
	manifestID, tenantID string,
	entries []ArchiveEntry,
) ArchiveManifest {
	entriesCopy := append([]ArchiveEntry(nil), entries...)
	m := ArchiveManifest{
		ManifestID:    manifestID,
		TenantID:      tenantID,
		SchemaVersion: ArchiveSchemaVersion,
		Entries:       entriesCopy,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	m.ManifestHash = computeManifestHash(m)
	return m
}

// VerifyArchiveManifest recomputes ManifestHash and checks it matches.
// Also verifies every entry's PayloadHash.
func VerifyArchiveManifest(manifest ArchiveManifest) (bool, string) {
	if manifest.ManifestHash == "" {
		return false, "ManifestHash is empty"
	}
	for _, entry := range manifest.Entries {
		ok, reason := VerifyArchiveEntry(entry)
		if !ok {
			return false, fmt.Sprintf("manifest %q: %s", manifest.ManifestID, reason)
		}
	}
	expected := computeManifestHash(manifest)
	if manifest.ManifestHash != expected {
		return false, fmt.Sprintf("manifest %q ManifestHash mismatch: expected %q got %q",
			manifest.ManifestID, expected, manifest.ManifestHash)
	}
	return true, ""
}

// EntriesForExecution returns entries matching the given executionID.
func EntriesForExecution(manifest ArchiveManifest, executionID string) []ArchiveEntry {
	var result []ArchiveEntry
	for _, e := range manifest.Entries {
		if e.ExecutionID == executionID {
			result = append(result, e)
		}
	}
	return result
}

// EntriesForType returns entries matching the given archive type.
func EntriesForType(manifest ArchiveManifest, archiveType ArchiveType) []ArchiveEntry {
	var result []ArchiveEntry
	for _, e := range manifest.Entries {
		if e.ArchiveType == archiveType {
			result = append(result, e)
		}
	}
	return result
}

func computePayloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%x", sum)
}

func computeManifestHash(m ArchiveManifest) string {
	h := sha256.New()
	fmt.Fprintf(h, "manifest_id:%s|tenant:%s|schema:%s|", m.ManifestID, m.TenantID, m.SchemaVersion)
	for _, e := range m.Entries {
		fmt.Fprintf(h, "entry:%s:%s:%s|", e.EntryID, string(e.ArchiveType), e.PayloadHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
