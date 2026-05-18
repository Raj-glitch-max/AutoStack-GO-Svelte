package archive

import "fmt"

// ArchiveIntegrityReport summarizes the result of verifying a collection of entries.
type ArchiveIntegrityReport struct {
	TenantID      string   `json:"tenant_id"`
	TotalEntries  int      `json:"total_entries"`
	ValidEntries  int      `json:"valid_entries"`
	InvalidCount  int      `json:"invalid_count"`
	InvalidRefs   []string `json:"invalid_refs,omitempty"`
	ManifestValid bool     `json:"manifest_valid"`
	FullyIntact   bool     `json:"fully_intact"`
}

// VerifyArchiveIntegrity checks every entry in a manifest and returns a report.
// It does not halt on the first invalid entry — all violations are collected.
func VerifyArchiveIntegrity(manifest ArchiveManifest) ArchiveIntegrityReport {
	report := ArchiveIntegrityReport{
		TenantID:     manifest.TenantID,
		TotalEntries: len(manifest.Entries),
	}

	// Verify manifest hash.
	ok, reason := BuildAndVerifyManifestHash(manifest)
	report.ManifestValid = ok
	if !ok {
		report.InvalidRefs = append(report.InvalidRefs, fmt.Sprintf("manifest hash: %s", reason))
	}

	// Verify each entry.
	for _, e := range manifest.Entries {
		if entryOK, entryReason := VerifyArchiveEntry(e); !entryOK {
			report.InvalidCount++
			report.InvalidRefs = append(report.InvalidRefs, fmt.Sprintf("entry %q: %s", e.EntryID, entryReason))
		} else {
			report.ValidEntries++
		}
	}

	report.FullyIntact = report.ManifestValid && report.InvalidCount == 0
	return report
}

// BuildAndVerifyManifestHash recomputes the manifest hash and returns (true, "").
func BuildAndVerifyManifestHash(manifest ArchiveManifest) (bool, string) {
	return VerifyArchiveManifest(manifest)
}
