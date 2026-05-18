package archive

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// ForensicExportManifest is a tamper-evident index of all entries in a forensic export.
// ExportedAt is excluded from ManifestHash (content-identity).
type ForensicExportManifest struct {
	ExportID       string         `json:"export_id"`
	ExecutionID    string         `json:"execution_id"`
	TenantID       string         `json:"tenant_id"`
	EntryCount     int            `json:"entry_count"`
	Entries        []ArchiveEntry `json:"entries"`
	IntegrityValid bool           `json:"integrity_valid"`
	ExportedAt     string         `json:"exported_at"` // excluded from hash
	ManifestHash   string         `json:"manifest_hash"`
}

// ExportForensicArchive bundles all entries for an execution into a forensic export.
// It verifies the integrity of every entry before inclusion.
// Entries with invalid hashes are excluded and listed in rejection reasons.
func ExportForensicArchive(exportID, executionID, tenantID string, entries []ArchiveEntry) (ForensicExportManifest, []string) {
	var valid []ArchiveEntry
	var rejections []string

	for _, e := range entries {
		if e.ExecutionID != executionID {
			continue
		}
		if e.TenantID != tenantID {
			rejections = append(rejections, fmt.Sprintf("entry %q: tenant mismatch (%s vs %s)", e.EntryID, e.TenantID, tenantID))
			continue
		}
		if ok, reason := VerifyArchiveEntry(e); !ok {
			rejections = append(rejections, fmt.Sprintf("entry %q: %s", e.EntryID, reason))
			continue
		}
		cp := ArchiveEntry{
			EntryID:       e.EntryID,
			ArchiveType:   e.ArchiveType,
			ExecutionID:   e.ExecutionID,
			TenantID:      e.TenantID,
			SchemaVersion: e.SchemaVersion,
			Payload:       append([]byte(nil), e.Payload...),
			PayloadHash:   e.PayloadHash,
			ArchivedAt:    e.ArchivedAt,
			Restorable:    e.Restorable,
		}
		valid = append(valid, cp)
	}

	manifest := ForensicExportManifest{
		ExportID:       exportID,
		ExecutionID:    executionID,
		TenantID:       tenantID,
		EntryCount:     len(valid),
		Entries:        valid,
		IntegrityValid: len(rejections) == 0,
		ExportedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	manifest.ManifestHash = computeForensicManifestHash(manifest)
	return manifest, rejections
}

// VerifyForensicExport recomputes the manifest hash and verifies every entry.
// Returns (true, nil) only when the manifest and all entries are valid.
func VerifyForensicExport(m ForensicExportManifest) (bool, []string) {
	var problems []string

	// Verify manifest hash.
	stored := m.ManifestHash
	m.ManifestHash = ""
	expected := computeForensicManifestHash(m)
	if stored != expected {
		problems = append(problems, fmt.Sprintf("manifest hash mismatch: stored=%s computed=%s", stored, expected))
	}
	m.ManifestHash = stored

	// Verify each entry.
	for _, e := range m.Entries {
		if ok, reason := VerifyArchiveEntry(e); !ok {
			problems = append(problems, fmt.Sprintf("entry %q: %s", e.EntryID, reason))
		}
	}

	return len(problems) == 0, problems
}

// BuildForensicArchiveEntry wraps a JSON-serializable value as an ArchiveEntry.
func BuildForensicArchiveEntry(entryID, executionID, tenantID string, archiveType ArchiveType, v interface{}) (ArchiveEntry, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return ArchiveEntry{}, fmt.Errorf("forensic entry: marshal: %w", err)
	}
	return NewArchiveEntry(entryID, archiveType, executionID, tenantID, b)
}

func computeForensicManifestHash(m ForensicExportManifest) string {
	h := sha256.New()
	fmt.Fprintf(h, "forensic:%s|exec:%s|tenant:%s|count:%d|integrity:%v",
		m.ExportID, m.ExecutionID, m.TenantID, m.EntryCount, m.IntegrityValid)
	for _, e := range m.Entries {
		fmt.Fprintf(h, "|e:%s/%s", e.EntryID, e.PayloadHash)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
