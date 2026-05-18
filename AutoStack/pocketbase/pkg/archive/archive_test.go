package archive

import (
	"bytes"
	"testing"
)

func TestNewArchiveEntry_Success(t *testing.T) {
	entry, err := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte(`{"state":"ok"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.PayloadHash == "" {
		t.Fatal("PayloadHash must be non-empty")
	}
	if len(entry.PayloadHash) != 64 {
		t.Fatalf("PayloadHash must be 64 hex chars, got %d", len(entry.PayloadHash))
	}
}

func TestNewArchiveEntry_EmptyPayload_Error(t *testing.T) {
	_, err := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", nil)
	if err == nil {
		t.Fatal("empty payload must return error")
	}
}

func TestNewArchiveEntry_EmptyEntryID_Error(t *testing.T) {
	_, err := NewArchiveEntry("", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data"))
	if err == nil {
		t.Fatal("empty entryID must return error")
	}
}

func TestNewArchiveEntry_DeepCopiesPayload(t *testing.T) {
	orig := []byte("original")
	entry, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", orig)
	orig[0] = 'X'
	if entry.Payload[0] == 'X' {
		t.Fatal("payload must be deep-copied — caller mutation must not affect the entry")
	}
}

func TestVerifyArchiveEntry_Valid(t *testing.T) {
	entry, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data"))
	ok, reason := VerifyArchiveEntry(entry)
	if !ok {
		t.Fatalf("valid entry must verify: %s", reason)
	}
}

func TestVerifyArchiveEntry_TamperedPayload(t *testing.T) {
	entry, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data"))
	entry.Payload = []byte("tampered")
	ok, _ := VerifyArchiveEntry(entry)
	if ok {
		t.Fatal("tampered payload must fail verification")
	}
}

func TestVerifyArchiveEntry_EmptyHash(t *testing.T) {
	entry, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data"))
	entry.PayloadHash = ""
	ok, reason := VerifyArchiveEntry(entry)
	if ok || reason == "" {
		t.Fatal("empty hash must fail with reason")
	}
}

func TestRestoreArchiveEntry_Success(t *testing.T) {
	payload := []byte(`{"execution_id":"exec-1"}`)
	entry, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", payload)
	restored, err := RestoreArchiveEntry(entry)
	if err != nil {
		t.Fatalf("restore error: %v", err)
	}
	if !bytes.Equal(restored, payload) {
		t.Fatal("restored payload must equal original")
	}
}

func TestRestoreArchiveEntry_NotRestorable(t *testing.T) {
	entry, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data"))
	entry.Restorable = false
	_, err := RestoreArchiveEntry(entry)
	if err == nil {
		t.Fatal("non-restorable entry must return error")
	}
}

func TestRestoreArchiveEntry_DeepCopiesPayload(t *testing.T) {
	entry, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data"))
	restored, _ := RestoreArchiveEntry(entry)
	restored[0] = 'X'
	if entry.Payload[0] == 'X' {
		t.Fatal("restore must return a deep copy — mutations must not affect the entry")
	}
}

func TestBuildArchiveManifest_HashIs64Chars(t *testing.T) {
	m := BuildArchiveManifest("m1", "tenant-1", nil)
	if len(m.ManifestHash) != 64 {
		t.Fatalf("ManifestHash must be 64 hex chars, got %d", len(m.ManifestHash))
	}
}

func TestVerifyArchiveManifest_Valid(t *testing.T) {
	e1, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data1"))
	e2, _ := NewArchiveEntry("e2", ArchiveTypeGovernance, "exec-1", "tenant-1", []byte("data2"))
	m := BuildArchiveManifest("m1", "tenant-1", []ArchiveEntry{e1, e2})
	ok, reason := VerifyArchiveManifest(m)
	if !ok {
		t.Fatalf("valid manifest must verify: %s", reason)
	}
}

func TestVerifyArchiveManifest_TamperedEntry(t *testing.T) {
	e1, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data"))
	m := BuildArchiveManifest("m1", "tenant-1", []ArchiveEntry{e1})
	m.Entries[0].Payload = []byte("tampered")
	ok, _ := VerifyArchiveManifest(m)
	if ok {
		t.Fatal("tampered entry payload must fail manifest verification")
	}
}

func TestVerifyArchiveManifest_TamperedManifestHash(t *testing.T) {
	m := BuildArchiveManifest("m1", "tenant-1", nil)
	m.ManifestHash = "badhash"
	ok, _ := VerifyArchiveManifest(m)
	if ok {
		t.Fatal("tampered ManifestHash must fail verification")
	}
}

func TestBuildArchiveManifest_Deterministic(t *testing.T) {
	e1, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data"))
	m1 := BuildArchiveManifest("m1", "tenant-1", []ArchiveEntry{e1})
	m2 := BuildArchiveManifest("m1", "tenant-1", []ArchiveEntry{e1})
	if m1.ManifestHash != m2.ManifestHash {
		t.Fatal("same inputs must produce same ManifestHash")
	}
}

func TestEntriesForExecution_Correct(t *testing.T) {
	e1, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("d1"))
	e2, _ := NewArchiveEntry("e2", ArchiveTypeExecution, "exec-2", "tenant-1", []byte("d2"))
	m := BuildArchiveManifest("m1", "tenant-1", []ArchiveEntry{e1, e2})
	result := EntriesForExecution(m, "exec-1")
	if len(result) != 1 || result[0].EntryID != "e1" {
		t.Fatal("EntriesForExecution must return only matching entries")
	}
}

func TestEntriesForType_Correct(t *testing.T) {
	e1, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("d1"))
	e2, _ := NewArchiveEntry("e2", ArchiveTypeGovernance, "exec-1", "tenant-1", []byte("d2"))
	m := BuildArchiveManifest("m1", "tenant-1", []ArchiveEntry{e1, e2})
	result := EntriesForType(m, ArchiveTypeGovernance)
	if len(result) != 1 || result[0].ArchiveType != ArchiveTypeGovernance {
		t.Fatal("EntriesForType must return only matching entries")
	}
}

func TestNewArchiveEntry_SchemaVersion(t *testing.T) {
	entry, _ := NewArchiveEntry("e1", ArchiveTypeExecution, "exec-1", "tenant-1", []byte("data"))
	if entry.SchemaVersion != ArchiveSchemaVersion {
		t.Fatalf("SchemaVersion mismatch: %q", entry.SchemaVersion)
	}
}
