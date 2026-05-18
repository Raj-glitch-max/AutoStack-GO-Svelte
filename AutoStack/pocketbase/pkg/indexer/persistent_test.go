package indexer

import (
	"testing"

	"github.com/janlauber/one-click/pkg/events"
)

// ─── helpers ───────────────────────────────────────────────────────────────

func makeTestIndex(t *testing.T, indexID, tenantID string, rebuildCount int) Index {
	t.Helper()
	return Index{
		IndexID:      indexID,
		IndexType:    IndexTypeExecution,
		TenantID:     tenantID,
		Entries:      []IndexEntry{},
		RebuildCount: rebuildCount,
		SourceHash:   "sha256-test-hash",
	}
}

func makeTestIndexWithEntries(t *testing.T, indexID, tenantID string, entries []IndexEntry) Index {
	t.Helper()
	return Index{
		IndexID:      indexID,
		IndexType:    IndexTypeExecution,
		TenantID:     tenantID,
		Entries:      entries,
		RebuildCount: 1,
		SourceHash:   "sha256-with-entries",
	}
}

// ─── BuildIndexSnapshot ────────────────────────────────────────────────────

func TestBuildIndexSnapshot_Basic(t *testing.T) {
	idx := makeTestIndex(t, "idx-1", "tenant-1", 3)
	snap := BuildIndexSnapshot("snap-1", idx)

	if snap.SnapshotID != "snap-1" {
		t.Fatalf("SnapshotID mismatch: %s", snap.SnapshotID)
	}
	if snap.IndexID != "idx-1" {
		t.Fatalf("IndexID mismatch: %s", snap.IndexID)
	}
	if snap.TenantID != "tenant-1" {
		t.Fatalf("TenantID mismatch: %s", snap.TenantID)
	}
	if snap.RebuildCount != 3 {
		t.Fatalf("RebuildCount mismatch: %d", snap.RebuildCount)
	}
	if snap.SnapshotHash == "" {
		t.Fatal("SnapshotHash must not be empty")
	}
	if snap.SnapshotAt == "" {
		t.Fatal("SnapshotAt must not be empty")
	}
}

func TestBuildIndexSnapshot_HashCoversContent(t *testing.T) {
	idx := makeTestIndex(t, "idx-1", "tenant-1", 1)
	s1 := BuildIndexSnapshot("snap-1", idx)
	s2 := BuildIndexSnapshot("snap-1", idx)
	// Different SnapshotAt timestamps, but same content → same SnapshotHash.
	if s1.SnapshotHash != s2.SnapshotHash {
		t.Fatalf("same content must produce same hash: %s vs %s", s1.SnapshotHash, s2.SnapshotHash)
	}
}

func TestBuildIndexSnapshot_DifferentIDsProduceDifferentHashes(t *testing.T) {
	idx1 := makeTestIndex(t, "idx-1", "tenant-1", 1)
	idx2 := makeTestIndex(t, "idx-2", "tenant-1", 1)
	s1 := BuildIndexSnapshot("snap-1", idx1)
	s2 := BuildIndexSnapshot("snap-1", idx2)
	if s1.SnapshotHash == s2.SnapshotHash {
		t.Fatal("different indexes must produce different hashes")
	}
}

func TestBuildIndexSnapshot_WithEntries(t *testing.T) {
	entries := []IndexEntry{
		{EntryID: "e1", AggregateID: "agg-1", LogicalClock: 1},
		{EntryID: "e2", AggregateID: "agg-2", LogicalClock: 2},
	}
	idx := makeTestIndexWithEntries(t, "idx-1", "t1", entries)
	snap := BuildIndexSnapshot("snap-1", idx)
	if snap.EntryCount != 2 {
		t.Fatalf("EntryCount must be 2, got %d", snap.EntryCount)
	}
}

// ─── VerifyIndexSnapshot ───────────────────────────────────────────────────

func TestVerifyIndexSnapshot_Valid(t *testing.T) {
	idx := makeTestIndex(t, "idx-1", "tenant-1", 1)
	snap := BuildIndexSnapshot("snap-1", idx)
	ok, reason := VerifyIndexSnapshot(snap)
	if !ok {
		t.Fatalf("valid snapshot must verify: %s", reason)
	}
}

func TestVerifyIndexSnapshot_TamperedSnapshotID(t *testing.T) {
	idx := makeTestIndex(t, "idx-1", "tenant-1", 1)
	snap := BuildIndexSnapshot("snap-1", idx)
	snap.SnapshotID = "snap-tampered"
	ok, _ := VerifyIndexSnapshot(snap)
	if ok {
		t.Fatal("tampered SnapshotID must fail verification")
	}
}

func TestVerifyIndexSnapshot_TamperedTenantID(t *testing.T) {
	idx := makeTestIndex(t, "idx-1", "tenant-1", 1)
	snap := BuildIndexSnapshot("snap-1", idx)
	snap.TenantID = "tenant-evil"
	ok, _ := VerifyIndexSnapshot(snap)
	if ok {
		t.Fatal("tampered TenantID must fail verification")
	}
}

func TestVerifyIndexSnapshot_TamperedRebuildCount(t *testing.T) {
	idx := makeTestIndex(t, "idx-1", "tenant-1", 5)
	snap := BuildIndexSnapshot("snap-1", idx)
	snap.RebuildCount = 99
	ok, _ := VerifyIndexSnapshot(snap)
	if ok {
		t.Fatal("tampered RebuildCount must fail verification")
	}
}

func TestVerifyIndexSnapshot_SnapshotAtExcludedFromHash(t *testing.T) {
	idx := makeTestIndex(t, "idx-1", "tenant-1", 1)
	snap := BuildIndexSnapshot("snap-1", idx)
	originalHash := snap.SnapshotHash
	// Mutating SnapshotAt must NOT affect the hash (it's excluded).
	snap.SnapshotAt = "2099-01-01T00:00:00Z"
	ok, _ := VerifyIndexSnapshot(snap)
	if !ok {
		t.Fatal("mutating SnapshotAt must not invalidate hash")
	}
	if snap.SnapshotHash != originalHash {
		t.Fatal("hash must remain unchanged when SnapshotAt changes")
	}
}

// ─── PersistentIndexStore ──────────────────────────────────────────────────

func TestPersistentIndexStore_SaveAndLoad(t *testing.T) {
	store := NewPersistentIndexStore()
	idx := makeTestIndex(t, "idx-1", "tenant-1", 1)
	snap := BuildIndexSnapshot("snap-1", idx)

	if err := store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	loaded, ok := store.LoadSnapshot("idx-1")
	if !ok {
		t.Fatal("snapshot must be found after save")
	}
	if loaded.SnapshotID != "snap-1" {
		t.Fatalf("SnapshotID mismatch: %s", loaded.SnapshotID)
	}
}

func TestPersistentIndexStore_LoadMissing(t *testing.T) {
	store := NewPersistentIndexStore()
	_, ok := store.LoadSnapshot("no-such-index")
	if ok {
		t.Fatal("missing snapshot must return false")
	}
}

func TestPersistentIndexStore_SaveRejectsInvalidHash(t *testing.T) {
	store := NewPersistentIndexStore()
	idx := makeTestIndex(t, "idx-1", "tenant-1", 1)
	snap := BuildIndexSnapshot("snap-1", idx)
	snap.SnapshotHash = "bad-hash"
	if err := store.SaveSnapshot(snap); err == nil {
		t.Fatal("SaveSnapshot must reject invalid hash")
	}
}

func TestPersistentIndexStore_LatestSnapshotReplacesPrior(t *testing.T) {
	store := NewPersistentIndexStore()
	idx1 := makeTestIndex(t, "idx-1", "tenant-1", 1)
	idx2 := makeTestIndex(t, "idx-1", "tenant-1", 2)
	snap1 := BuildIndexSnapshot("snap-v1", idx1)
	snap2 := BuildIndexSnapshot("snap-v2", idx2)

	_ = store.SaveSnapshot(snap1)
	_ = store.SaveSnapshot(snap2)

	loaded, _ := store.LoadSnapshot("idx-1")
	if loaded.SnapshotID != "snap-v2" {
		t.Fatalf("latest snapshot must win: got %s", loaded.SnapshotID)
	}
}

func TestPersistentIndexStore_IsStale_WhenNoSnapshot(t *testing.T) {
	store := NewPersistentIndexStore()
	if !store.IsStale("idx-nonexistent", "any-hash") {
		t.Fatal("missing index must be considered stale")
	}
}

func TestPersistentIndexStore_IsStale_WhenHashMatches(t *testing.T) {
	store := NewPersistentIndexStore()
	idx := makeTestIndex(t, "idx-1", "tenant-1", 1)
	idx.SourceHash = "source-hash-abc"
	snap := BuildIndexSnapshot("snap-1", idx)
	_ = store.SaveSnapshot(snap)

	if store.IsStale("idx-1", "source-hash-abc") {
		t.Fatal("matching SourceHash must not be stale")
	}
}

func TestPersistentIndexStore_IsStale_WhenHashDiffers(t *testing.T) {
	store := NewPersistentIndexStore()
	idx := makeTestIndex(t, "idx-1", "tenant-1", 1)
	idx.SourceHash = "source-hash-abc"
	snap := BuildIndexSnapshot("snap-1", idx)
	_ = store.SaveSnapshot(snap)

	if !store.IsStale("idx-1", "source-hash-different") {
		t.Fatal("differing SourceHash must be stale")
	}
}

func TestPersistentIndexStore_RecordRebuild(t *testing.T) {
	store := NewPersistentIndexStore()
	n1 := store.RecordRebuild("idx-1")
	n2 := store.RecordRebuild("idx-1")
	n3 := store.RecordRebuild("idx-1")
	if n1 != 1 || n2 != 2 || n3 != 3 {
		t.Fatalf("rebuild counts must be 1,2,3 — got %d,%d,%d", n1, n2, n3)
	}
}

func TestPersistentIndexStore_TotalRebuildCount(t *testing.T) {
	store := NewPersistentIndexStore()
	store.RecordRebuild("idx-a")
	store.RecordRebuild("idx-a")
	store.RecordRebuild("idx-b")

	if store.TotalRebuildCount("idx-a") != 2 {
		t.Fatalf("idx-a must have 2 rebuilds")
	}
	if store.TotalRebuildCount("idx-b") != 1 {
		t.Fatalf("idx-b must have 1 rebuild")
	}
	if store.TotalRebuildCount("idx-unknown") != 0 {
		t.Fatalf("unknown index must have 0 rebuilds")
	}
}

func TestPersistentIndexStore_AllIndexIDs_Sorted(t *testing.T) {
	store := NewPersistentIndexStore()
	for _, id := range []string{"idx-c", "idx-a", "idx-b"} {
		idx := makeTestIndex(t, id, "t1", 1)
		snap := BuildIndexSnapshot("snap-"+id, idx)
		_ = store.SaveSnapshot(snap)
	}
	ids := store.AllIndexIDs()
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "idx-a" || ids[1] != "idx-b" || ids[2] != "idx-c" {
		t.Fatalf("IDs must be sorted: got %v", ids)
	}
}

func TestPersistentIndexStore_MultipleIndexes_Independent(t *testing.T) {
	store := NewPersistentIndexStore()
	idx1 := makeTestIndex(t, "idx-1", "t1", 1)
	idx2 := makeTestIndex(t, "idx-2", "t2", 5)
	_ = store.SaveSnapshot(BuildIndexSnapshot("snap-1", idx1))
	_ = store.SaveSnapshot(BuildIndexSnapshot("snap-2", idx2))

	loaded1, ok1 := store.LoadSnapshot("idx-1")
	loaded2, ok2 := store.LoadSnapshot("idx-2")
	if !ok1 || !ok2 {
		t.Fatal("both snapshots must be found")
	}
	if loaded1.TenantID != "t1" || loaded2.TenantID != "t2" {
		t.Fatal("tenants must be independent")
	}
	if loaded1.RebuildCount != 1 || loaded2.RebuildCount != 5 {
		t.Fatal("rebuild counts must be preserved per index")
	}
}

// ─── integration: BuildIndex → BuildIndexSnapshot → store ─────────────────

func TestPersistentIndexStore_IntegrationWithBuildIndex(t *testing.T) {
	store := NewPersistentIndexStore()

	// Build a real index from events.
	evt := events.NewPlatformEvent(
		"evt-1", events.EventTypeExecutionStarted, events.AggregateTypeExecution,
		"agg-1", "exec-1", "tenant-1", 1,
		"", "", []byte(`{}`), nil,
	)
	idx := BuildIndex("idx-exec", IndexTypeExecution, "tenant-1", []events.PlatformEvent{evt})
	snap := BuildIndexSnapshot("snap-exec-1", idx)

	if err := store.SaveSnapshot(snap); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	loaded, ok := store.LoadSnapshot("idx-exec")
	if !ok {
		t.Fatal("snapshot must be found")
	}
	if loaded.EntryCount != 1 {
		t.Fatalf("expected 1 entry, got %d", loaded.EntryCount)
	}

	// IsStale: same source hash → not stale.
	if store.IsStale("idx-exec", idx.SourceHash) {
		t.Fatal("same SourceHash must not be stale")
	}
}
