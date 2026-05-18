package archive

import (
	"testing"
	"time"
)

// ─── RetentionPolicy ──────────────────────────────────────────────────────────

func TestEvaluateRetention_TooNew_NotEligible(t *testing.T) {
	entry, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"x":1}`))
	policy := RetentionPolicy{MinRetentionDays: 365, ForensicRequired: false}
	result := EvaluateRetention(entry, policy, false, time.Now())
	if result.Eligible {
		t.Fatal("brand-new entry must not be retention eligible")
	}
}

func TestEvaluateRetention_OldEnough_Eligible(t *testing.T) {
	entry, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"x":1}`))
	// Set ArchivedAt to 400 days ago.
	past := time.Now().AddDate(0, 0, -400)
	entry.ArchivedAt = past.UTC().Format(time.RFC3339Nano)

	policy := RetentionPolicy{MinRetentionDays: 365, ForensicRequired: false}
	result := EvaluateRetention(entry, policy, false, time.Now())
	if !result.Eligible {
		t.Fatalf("400-day old entry must be eligible: %s", result.Reason)
	}
}

func TestEvaluateRetention_ForensicRequired_BlocksEligibility(t *testing.T) {
	entry, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"x":1}`))
	past := time.Now().AddDate(0, 0, -400)
	entry.ArchivedAt = past.UTC().Format(time.RFC3339Nano)

	policy := RetentionPolicy{MinRetentionDays: 365, ForensicRequired: true}
	result := EvaluateRetention(entry, policy, false, time.Now()) // forensicExported=false
	if result.Eligible {
		t.Fatal("forensic-required policy must block eligibility when not exported")
	}
}

func TestEvaluateRetention_ForensicRequired_Exported(t *testing.T) {
	entry, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"x":1}`))
	past := time.Now().AddDate(0, 0, -400)
	entry.ArchivedAt = past.UTC().Format(time.RFC3339Nano)

	policy := RetentionPolicy{MinRetentionDays: 365, ForensicRequired: true}
	result := EvaluateRetention(entry, policy, true, time.Now())
	if !result.Eligible {
		t.Fatalf("forensic-exported entry must be eligible: %s", result.Reason)
	}
}

// ─── ExportForensicArchive ────────────────────────────────────────────────────

func TestExportForensicArchive_ValidEntries(t *testing.T) {
	e1, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"step":"start"}`))
	e2, _ := NewArchiveEntry("e-2", ArchiveTypeGovernance, "exec-1", "tenant-1", []byte(`{"policy":"p1"}`))

	manifest, rejections := ExportForensicArchive("export-1", "exec-1", "tenant-1", []ArchiveEntry{e1, e2})
	if len(rejections) != 0 {
		t.Fatalf("expected 0 rejections, got: %v", rejections)
	}
	if manifest.EntryCount != 2 {
		t.Fatalf("expected 2 entries, got %d", manifest.EntryCount)
	}
	if !manifest.IntegrityValid {
		t.Fatal("integrity must be valid")
	}
}

func TestExportForensicArchive_TamperedEntry_Rejected(t *testing.T) {
	e, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"x":1}`))
	e.Payload = []byte(`{"x":99}`) // tamper payload without updating hash

	_, rejections := ExportForensicArchive("export-1", "exec-1", "tenant-1", []ArchiveEntry{e})
	if len(rejections) == 0 {
		t.Fatal("tampered entry must be rejected")
	}
}

func TestExportForensicArchive_WrongTenant_Excluded(t *testing.T) {
	e, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-other", []byte(`{"x":1}`))
	manifest, _ := ExportForensicArchive("export-1", "exec-1", "tenant-1", []ArchiveEntry{e})
	if manifest.EntryCount != 0 {
		t.Fatal("wrong-tenant entry must be excluded")
	}
}

func TestVerifyForensicExport_Valid(t *testing.T) {
	e, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"x":1}`))
	manifest, _ := ExportForensicArchive("export-1", "exec-1", "tenant-1", []ArchiveEntry{e})
	ok, problems := VerifyForensicExport(manifest)
	if !ok {
		t.Fatalf("valid export must verify: %v", problems)
	}
}

func TestVerifyForensicExport_TamperedManifestHash(t *testing.T) {
	e, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"x":1}`))
	manifest, _ := ExportForensicArchive("export-1", "exec-1", "tenant-1", []ArchiveEntry{e})
	manifest.EntryCount = 99 // tamper
	ok, _ := VerifyForensicExport(manifest)
	if ok {
		t.Fatal("tampered manifest must not verify")
	}
}

func TestBuildForensicArchiveEntry_ValidJSON(t *testing.T) {
	type Data struct{ X int }
	entry, err := BuildForensicArchiveEntry("e-1", "exec-1", "tenant-1", ArchiveTypeForensic, Data{X: 42})
	if err != nil {
		t.Fatalf("BuildForensicArchiveEntry: %v", err)
	}
	ok, reason := VerifyArchiveEntry(entry)
	if !ok {
		t.Fatalf("built entry must verify: %s", reason)
	}
}

// ─── VerifyArchiveIntegrity ───────────────────────────────────────────────────

func TestVerifyArchiveIntegrity_FullyIntact(t *testing.T) {
	e1, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"a":1}`))
	e2, _ := NewArchiveEntry("e-2", ArchiveTypeGovernance, "exec-1", "tenant-1", []byte(`{"b":2}`))
	manifest := BuildArchiveManifest("manifest-1", "tenant-1", []ArchiveEntry{e1, e2})

	report := VerifyArchiveIntegrity(manifest)
	if !report.FullyIntact {
		t.Fatalf("fully intact archive must report intact: %v", report.InvalidRefs)
	}
	if report.TotalEntries != 2 {
		t.Fatalf("expected 2 total, got %d", report.TotalEntries)
	}
	if report.ValidEntries != 2 {
		t.Fatalf("expected 2 valid, got %d", report.ValidEntries)
	}
}

func TestVerifyArchiveIntegrity_TamperedEntry(t *testing.T) {
	e, _ := NewArchiveEntry("e-1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"a":1}`))
	// Build a legitimate manifest first...
	manifest := BuildArchiveManifest("manifest-1", "tenant-1", []ArchiveEntry{e})
	// ...then tamper with an entry payload.
	manifest.Entries[0].Payload = []byte(`{"a":999}`)

	report := VerifyArchiveIntegrity(manifest)
	if report.FullyIntact {
		t.Fatal("tampered entry must not be fully intact")
	}
	if report.InvalidCount == 0 {
		t.Fatal("must detect at least one invalid entry")
	}
}
