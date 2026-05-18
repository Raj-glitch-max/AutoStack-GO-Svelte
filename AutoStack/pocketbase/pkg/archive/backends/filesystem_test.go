package backends

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/janlauber/one-click/pkg/archive"
)

func makeEntry(t *testing.T, id, tenantID string) archive.ArchiveEntry {
	t.Helper()
	entry, err := archive.NewArchiveEntry(id, archive.ArchiveTypeExecution, "exec-1", tenantID, []byte(`{"data":"test"}`))
	if err != nil {
		t.Fatalf("NewArchiveEntry: %v", err)
	}
	return entry
}

// ─── FilesystemArchiveBackend ──────────────────────────────────────────────

func TestFilesystemBackend_StoreAndLoad(t *testing.T) {
	dir := t.TempDir()
	b, err := NewFilesystemArchiveBackend(dir)
	if err != nil {
		t.Fatalf("NewFilesystemArchiveBackend: %v", err)
	}
	entry := makeEntry(t, "entry-1", "tenant-1")
	if err := b.Store(entry); err != nil {
		t.Fatalf("Store: %v", err)
	}
	loaded, err := b.Load("entry-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.EntryID != "entry-1" {
		t.Fatalf("EntryID mismatch: %s", loaded.EntryID)
	}
	if loaded.TenantID != "tenant-1" {
		t.Fatalf("TenantID mismatch: %s", loaded.TenantID)
	}
}

func TestFilesystemBackend_LoadNotFound(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFilesystemArchiveBackend(dir)
	_, err := b.Load("no-such-entry")
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound, got: %v", err)
	}
}

func TestFilesystemBackend_HashVerificationOnLoad(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFilesystemArchiveBackend(dir)
	entry := makeEntry(t, "entry-2", "tenant-1")
	_ = b.Store(entry)

	// Tamper with the stored file.
	tampered := filepath.Join(dir, "entries", "entry-2.json")
	_ = os.WriteFile(tampered, []byte(`{"entry_id":"entry-2","tenant_id":"tenant-1","payload":"dGFtcGVyZWQ=","payload_hash":"bad","restorable":true,"archive_type":"execution","schema_version":"5.0.0","archived_at":"x","execution_id":"exec-1"}`), 0600)
	_, err := b.Load("entry-2")
	if err == nil {
		t.Fatal("loading tampered entry must error")
	}
	if !errors.Is(err, ErrPayloadCorrupt) {
		t.Fatalf("expected ErrPayloadCorrupt, got: %v", err)
	}
}

func TestFilesystemBackend_StoreManifestAndLoad(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFilesystemArchiveBackend(dir)
	e1 := makeEntry(t, "e1", "t1")
	e2 := makeEntry(t, "e2", "t1")
	manifest := archive.BuildArchiveManifest("manifest-1", "t1", []archive.ArchiveEntry{e1, e2})
	if err := b.StoreManifest(manifest); err != nil {
		t.Fatalf("StoreManifest: %v", err)
	}
	loaded, err := b.LoadManifest("manifest-1")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if loaded.ManifestID != "manifest-1" {
		t.Fatalf("ManifestID mismatch: %s", loaded.ManifestID)
	}
	if len(loaded.Entries) != 2 {
		t.Fatalf("expected 2 entries in manifest, got %d", len(loaded.Entries))
	}
}

func TestFilesystemBackend_LoadManifestNotFound(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFilesystemArchiveBackend(dir)
	_, err := b.LoadManifest("no-such-manifest")
	if !errors.Is(err, ErrManifestNotFound) {
		t.Fatalf("expected ErrManifestNotFound, got: %v", err)
	}
}

func TestFilesystemBackend_ListEntries(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFilesystemArchiveBackend(dir)
	e1 := makeEntry(t, "e1", "tenant-1")
	e2 := makeEntry(t, "e2", "tenant-2")
	e3 := makeEntry(t, "e3", "tenant-1")
	_ = b.Store(e1)
	_ = b.Store(e2)
	_ = b.Store(e3)

	ids, err := b.ListEntries("tenant-1")
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("tenant-1 must have 2 entries, got %d", len(ids))
	}
	for _, id := range ids {
		if id == "e2" {
			t.Fatal("tenant-2 entry must not appear in tenant-1 list")
		}
	}
}

func TestFilesystemBackend_StoreIdempotent(t *testing.T) {
	dir := t.TempDir()
	b, _ := NewFilesystemArchiveBackend(dir)
	entry := makeEntry(t, "entry-1", "t1")
	_ = b.Store(entry)
	if err := b.Store(entry); err != nil {
		t.Fatalf("re-store same entry must not error: %v", err)
	}
	loaded, err := b.Load("entry-1")
	if err != nil {
		t.Fatalf("load after re-store: %v", err)
	}
	if loaded.PayloadHash != entry.PayloadHash {
		t.Fatal("re-stored entry must have same PayloadHash")
	}
}

func TestS3Backend_AllMethodsReturnNotImplemented(t *testing.T) {
	b := NewS3ArchiveBackend(S3Config{Bucket: "test", Region: "us-east-1"})
	e := makeEntry(t, "e1", "t1")
	if err := b.Store(e); !errors.Is(err, ErrS3NotImplemented) {
		t.Fatalf("Store must return ErrS3NotImplemented, got: %v", err)
	}
	if _, err := b.Load("e1"); !errors.Is(err, ErrS3NotImplemented) {
		t.Fatalf("Load must return ErrS3NotImplemented, got: %v", err)
	}
	if _, err := b.ListEntries("t1"); !errors.Is(err, ErrS3NotImplemented) {
		t.Fatalf("ListEntries must return ErrS3NotImplemented, got: %v", err)
	}
}
