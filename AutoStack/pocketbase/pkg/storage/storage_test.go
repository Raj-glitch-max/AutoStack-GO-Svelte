package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/janlauber/one-click/pkg/events"
)

// ─── WAL ──────────────────────────────────────────────────────────────────

func TestWAL_AppendAndRead(t *testing.T) {
	dir := t.TempDir()
	w, err := OpenWAL(filepath.Join(dir, "wal.log"))
	if err != nil {
		t.Fatalf("OpenWAL: %v", err)
	}
	defer w.Close()

	body := []byte(`{"event_id":"e1"}`)
	entry, err := w.Append(WALEntryTypeEvent, "e1", body)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if entry.Sequence != 1 {
		t.Fatalf("first entry sequence must be 1, got %d", entry.Sequence)
	}
	if entry.EntryHash == "" {
		t.Fatal("EntryHash must be set")
	}
	entries := w.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestWAL_MultipleEntries(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer w.Close()
	for i := 0; i < 5; i++ {
		_, err := w.Append(WALEntryTypeEvent, "e"+string(rune('0'+i)), []byte(`{}`))
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	entries := w.Entries()
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
	for i, e := range entries {
		if e.Sequence != int64(i+1) {
			t.Fatalf("entry %d must have sequence %d, got %d", i, i+1, e.Sequence)
		}
	}
}

func TestWAL_SequencesMonotonic(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer w.Close()
	e1, _ := w.Append(WALEntryTypeEvent, "e1", []byte(`{}`))
	e2, _ := w.Append(WALEntryTypeEvent, "e2", []byte(`{}`))
	if e2.Sequence != e1.Sequence+1 {
		t.Fatalf("sequence must be monotonic: %d, %d", e1.Sequence, e2.Sequence)
	}
}

func TestWAL_IntegrityVerify_Fresh(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer w.Close()
	_, _ = w.Append(WALEntryTypeEvent, "e1", []byte(`{"x":1}`))
	_, _ = w.Append(WALEntryTypeEvent, "e2", []byte(`{"x":2}`))
	if bad := w.VerifyIntegrity(); len(bad) != 0 {
		t.Fatalf("fresh WAL must have no integrity failures, got: %v", bad)
	}
}

func TestWAL_IntegrityVerify_Tampered(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer w.Close()
	_, _ = w.Append(WALEntryTypeEvent, "e1", []byte(`{"x":1}`))
	// Tamper with the in-memory entry.
	w.entries[0].Body = []byte(`{"x": tampered}`)
	bad := w.VerifyIntegrity()
	if len(bad) != 1 {
		t.Fatalf("tampered entry must be detected, got %d bad entries", len(bad))
	}
	if bad[0] != 1 {
		t.Fatalf("bad sequence must be 1, got %d", bad[0])
	}
}

func TestWAL_Checkpoint(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer w.Close()
	_, _ = w.Append(WALEntryTypeEvent, "e1", []byte(`{}`))
	_, _ = w.Append(WALEntryTypeEvent, "e2", []byte(`{}`))
	cp, err := w.Checkpoint("cp-1")
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	if cp.LastSequence != 2 {
		t.Fatalf("checkpoint LastSequence must be 2, got %d", cp.LastSequence)
	}
	if cp.StateHash == "" {
		t.Fatal("StateHash must be set")
	}
}

func TestWAL_CheckpointPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")
	w1, _ := OpenWAL(path)
	_, _ = w1.Append(WALEntryTypeEvent, "e1", []byte(`{}`))
	_, _ = w1.Checkpoint("cp-1")
	_ = w1.Close()

	w2, err := OpenWAL(path)
	if err != nil {
		t.Fatalf("reopen WAL: %v", err)
	}
	defer w2.Close()
	cp := w2.LatestCheckpoint()
	if cp == nil {
		t.Fatal("checkpoint must persist across reopen")
	}
	if cp.CheckpointID != "cp-1" {
		t.Fatalf("checkpoint ID mismatch: %s", cp.CheckpointID)
	}
}

func TestWAL_ReplayAfterReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal.log")
	w1, _ := OpenWAL(path)
	for i := 0; i < 3; i++ {
		_, _ = w1.Append(WALEntryTypeEvent, "e"+string(rune('0'+i)), []byte(`{}`))
	}
	_ = w1.Close()

	w2, _ := OpenWAL(path)
	defer w2.Close()
	entries := w2.Entries()
	if len(entries) != 3 {
		t.Fatalf("replayed WAL must have 3 entries, got %d", len(entries))
	}
	// Integrity must pass after replay.
	if bad := w2.VerifyIntegrity(); len(bad) != 0 {
		t.Fatalf("replayed WAL must pass integrity check, bad: %v", bad)
	}
}

func TestWAL_EntriesSince(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer w.Close()
	for i := 0; i < 5; i++ {
		_, _ = w.Append(WALEntryTypeEvent, "e"+string(rune('0'+i)), []byte(`{}`))
	}
	since := w.EntriesSince(3)
	if len(since) != 2 {
		t.Fatalf("EntriesSince(3) must return 2 entries (seq 4,5), got %d", len(since))
	}
}

func TestWAL_Metadata(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer w.Close()
	_, _ = w.Append(WALEntryTypeEvent, "e1", []byte(`{}`))
	meta := w.Metadata()
	if meta.EntryCount != 1 {
		t.Fatalf("EntryCount must be 1, got %d", meta.EntryCount)
	}
	if meta.IntegrityStatus != "ok" {
		t.Fatalf("fresh WAL IntegrityStatus must be 'ok', got %s", meta.IntegrityStatus)
	}
}

// ─── VerifyWALCheckpoint ──────────────────────────────────────────────────

func TestVerifyWALCheckpoint_Valid(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer w.Close()
	_, _ = w.Append(WALEntryTypeEvent, "e1", []byte(`{}`))
	_, _ = w.Append(WALEntryTypeEvent, "e2", []byte(`{}`))
	cp, _ := w.Checkpoint("cp-1")
	ok, reason := VerifyWALCheckpoint(cp, w.Entries())
	if !ok {
		t.Fatalf("valid checkpoint must verify: %s", reason)
	}
}

func TestVerifyWALCheckpoint_Tampered(t *testing.T) {
	dir := t.TempDir()
	w, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer w.Close()
	_, _ = w.Append(WALEntryTypeEvent, "e1", []byte(`{}`))
	cp, _ := w.Checkpoint("cp-1")
	cp.StateHash = "deadbeef"
	ok, _ := VerifyWALCheckpoint(cp, w.Entries())
	if ok {
		t.Fatal("tampered checkpoint must fail verification")
	}
}

// ─── CorruptionFinding ────────────────────────────────────────────────────

func TestNewCorruptionFinding_Fields(t *testing.T) {
	f := NewCorruptionFinding("f1", CorruptionTypeInvalidHash, "evt-x", "hash mismatch")
	if f.FindingID != "f1" || f.Type != CorruptionTypeInvalidHash {
		t.Fatal("fields must match inputs")
	}
	if f.DetectedAt == "" {
		t.Fatal("DetectedAt must be set")
	}
}

func TestBuildCorruptionReport_HashNonEmpty(t *testing.T) {
	r := BuildCorruptionReport("rep-1", "t1", nil)
	if r.ReportHash == "" {
		t.Fatal("ReportHash must be set")
	}
}

func TestVerifyCorruptionReport_Fresh(t *testing.T) {
	f := NewCorruptionFinding("f1", CorruptionTypeClockRegression, "e1", "detail")
	r := BuildCorruptionReport("rep-1", "t1", []CorruptionFinding{f})
	ok, reason := VerifyCorruptionReport(r)
	if !ok {
		t.Fatalf("fresh report must verify: %s", reason)
	}
}

func TestVerifyCorruptionReport_Tampered(t *testing.T) {
	r := BuildCorruptionReport("rep-1", "t1", nil)
	r.TenantID = "evil"
	ok, _ := VerifyCorruptionReport(r)
	if ok {
		t.Fatal("tampered report must fail")
	}
}

func TestCorruptionReport_Severity_Critical(t *testing.T) {
	f := NewCorruptionFinding("f1", CorruptionTypeInvalidHash, "e1", "d")
	r := BuildCorruptionReport("r1", "t1", []CorruptionFinding{f})
	if r.Severity != "critical" {
		t.Fatalf("invalid hash must be critical severity, got %s", r.Severity)
	}
}

func TestCorruptionReport_Severity_None(t *testing.T) {
	r := BuildCorruptionReport("r1", "t1", nil)
	if r.Severity != "none" {
		t.Fatalf("no findings must be 'none' severity, got %s", r.Severity)
	}
}

func TestCorruptionFindingAsEvent(t *testing.T) {
	f := NewCorruptionFinding("f1", CorruptionTypeInvalidHash, "e-bad", "hash mismatch")
	evt := CorruptionFindingAsEvent(f, "t1", "incident-1", 42)
	if evt.EventID != "incident-1" {
		t.Fatalf("event ID mismatch: %s", evt.EventID)
	}
	if evt.TenantID != "t1" {
		t.Fatal("event must carry tenantID")
	}
}

func TestCorruptionFindingsFromWAL(t *testing.T) {
	findings := CorruptionFindingsFromWAL([]int64{3, 7})
	if len(findings) != 2 {
		t.Fatalf("must produce 2 findings, got %d", len(findings))
	}
	for _, f := range findings {
		if f.Type != CorruptionTypeWALEntryHashInvalid {
			t.Fatalf("wrong type: %s", f.Type)
		}
	}
}

// ─── DurabilitySnapshot ───────────────────────────────────────────────────

func TestBuildDurabilitySnapshot_HashNonEmpty(t *testing.T) {
	snap := BuildDurabilitySnapshot("ds-1", "t1", WALMetadata{}, nil, nil, nil, nil,
		LeaseIntegritySummary{}, ArchiveIntegritySummary{})
	if snap.SnapshotHash == "" {
		t.Fatal("SnapshotHash must be set")
	}
}

func TestVerifyDurabilitySnapshot_Fresh(t *testing.T) {
	snap := BuildDurabilitySnapshot("ds-1", "t1",
		WALMetadata{EntryCount: 5, LastSequence: 5, IntegrityStatus: "ok"},
		map[string]string{"s1": "abc"},
		map[string]string{"cp-1": "def"},
		nil, nil,
		LeaseIntegritySummary{TotalLeases: 2, IntegrityOK: true},
		ArchiveIntegritySummary{TotalEntries: 3, ManifestOK: true},
	)
	ok, reason := VerifyDurabilitySnapshot(snap)
	if !ok {
		t.Fatalf("fresh durability snapshot must verify: %s", reason)
	}
}

func TestVerifyDurabilitySnapshot_Tampered(t *testing.T) {
	snap := BuildDurabilitySnapshot("ds-1", "t1", WALMetadata{}, nil, nil, nil, nil,
		LeaseIntegritySummary{}, ArchiveIntegritySummary{})
	snap.TenantID = "evil"
	ok, _ := VerifyDurabilitySnapshot(snap)
	if ok {
		t.Fatal("tampered snapshot must fail")
	}
}

func TestVerifyDurabilitySnapshot_ContentIdentity(t *testing.T) {
	build := func() DurabilitySnapshot {
		return BuildDurabilitySnapshot("ds-1", "t1",
			WALMetadata{EntryCount: 1, LastSequence: 1, IntegrityStatus: "ok"},
			map[string]string{"s1": "hash1"},
			nil, nil, nil,
			LeaseIntegritySummary{}, ArchiveIntegritySummary{},
		)
	}
	s1, s2 := build(), build()
	if s1.SnapshotHash != s2.SnapshotHash {
		t.Fatal("durability snapshot must be content-identity (GeneratedAt excluded)")
	}
}

func TestIsSafeToOperate_OK(t *testing.T) {
	snap := BuildDurabilitySnapshot("ds-1", "t1",
		WALMetadata{IntegrityStatus: "ok"}, nil, nil, nil, nil,
		LeaseIntegritySummary{}, ArchiveIntegritySummary{})
	if !IsSafeToOperate(snap) {
		t.Fatal("clean snapshot must be safe to operate")
	}
}

func TestIsSafeToOperate_CorruptedWAL(t *testing.T) {
	snap := BuildDurabilitySnapshot("ds-1", "t1",
		WALMetadata{IntegrityStatus: "corrupted"}, nil, nil, nil, nil,
		LeaseIntegritySummary{}, ArchiveIntegritySummary{})
	if IsSafeToOperate(snap) {
		t.Fatal("corrupted WAL must not be safe to operate")
	}
}

func TestIsSafeToOperate_CriticalFinding(t *testing.T) {
	f := NewCorruptionFinding("f1", CorruptionTypeInvalidHash, "e1", "d")
	snap := BuildDurabilitySnapshot("ds-1", "t1",
		WALMetadata{IntegrityStatus: "ok"}, nil, nil, []CorruptionFinding{f}, nil,
		LeaseIntegritySummary{}, ArchiveIntegritySummary{})
	if IsSafeToOperate(snap) {
		t.Fatal("critical corruption finding must not be safe to operate")
	}
}

// ─── RecoverDurableRuntimeState ───────────────────────────────────────────

func makeRecoveryEvent(t *testing.T, id string, clock int64) events.PlatformEvent {
	t.Helper()
	return events.NewPlatformEvent(
		id, events.EventTypeExecutionStarted, events.AggregateTypeExecution,
		"agg-1", "exec-1", "tenant-1", clock, "", "", nil, nil,
	)
}

func TestRecoverDurableRuntimeState_Basic(t *testing.T) {
	dir := t.TempDir()
	wal, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer wal.Close()

	store := events.NewInMemoryEventStore()
	ctx := context.Background()

	// Write 3 events to WAL.
	for i := int64(1); i <= 3; i++ {
		evt := makeRecoveryEvent(t, "e"+string(rune('0'+i)), i)
		body, _ := events.CanonicalSerialize(evt)
		_, _ = wal.Append(WALEntryTypeEvent, evt.EventID, body)
		// Also append to store so we can verify duplicate skip.
		if i == 1 {
			_ = store.Append(ctx, "stream-1", evt)
		}
	}

	result, err := RecoverDurableRuntimeState(wal, store, "tenant-1", "stream-1", 100)
	if err != nil {
		t.Fatalf("RecoverDurableRuntimeState: %v", err)
	}
	// e1 was already in store → duplicate skip; e2 and e3 recovered.
	if result.RecoveredEventCount != 2 {
		t.Fatalf("expected 2 recovered events, got %d", result.RecoveredEventCount)
	}
	if result.DuplicatesSkipped != 1 {
		t.Fatalf("expected 1 duplicate skipped, got %d", result.DuplicatesSkipped)
	}
	if !result.SafeToContinue {
		t.Fatal("clean recovery must be safe to continue")
	}
	if len(result.CorruptionFindings) != 0 {
		t.Fatalf("clean recovery must have no corruption findings, got %d", len(result.CorruptionFindings))
	}
}

func TestRecoverDurableRuntimeState_CorruptWALEntry(t *testing.T) {
	dir := t.TempDir()
	wal, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer wal.Close()
	store := events.NewInMemoryEventStore()

	// Write one good event.
	evt := makeRecoveryEvent(t, "e1", 1)
	body, _ := events.CanonicalSerialize(evt)
	_, _ = wal.Append(WALEntryTypeEvent, "e1", body)

	// Tamper with the entry hash to simulate corruption.
	wal.entries[0].EntryHash = "bad-hash"

	result, err := RecoverDurableRuntimeState(wal, store, "tenant-1", "stream-1", 100)
	if err != nil {
		t.Fatalf("RecoverDurableRuntimeState: %v", err)
	}
	if len(result.CorruptionFindings) == 0 {
		t.Fatal("corrupted WAL entry must produce at least one finding")
	}
	if result.SafeToContinue {
		t.Fatal("corrupt WAL must set SafeToContinue=false")
	}
}

func TestRecoverDurableRuntimeState_EmptyWAL(t *testing.T) {
	dir := t.TempDir()
	wal, _ := OpenWAL(filepath.Join(dir, "wal.log"))
	defer wal.Close()
	store := events.NewInMemoryEventStore()
	result, err := RecoverDurableRuntimeState(wal, store, "tenant-1", "stream-1", 0)
	if err != nil {
		t.Fatalf("empty WAL recovery must not error: %v", err)
	}
	if result.RecoveredEventCount != 0 {
		t.Fatal("empty WAL must recover 0 events")
	}
	if !result.SafeToContinue {
		t.Fatal("empty WAL must be safe to continue")
	}
}

// ─── StorageIntegrityReport ───────────────────────────────────────────────

func TestVerifyEventStorageIntegrity_ValidEvents(t *testing.T) {
	store := events.NewPocketBaseEventStore(events.NewInMemoryRecordPersistence())
	ctx := context.Background()
	for i := int64(1); i <= 3; i++ {
		evt := events.NewPlatformEvent(
			"e"+string(rune('0'+i)), events.EventTypeExecutionStarted,
			events.AggregateTypeExecution, "agg", "exec", "t1", i, "", "", nil, nil,
		)
		_ = store.Append(ctx, "s1", evt)
	}
	recs, _ := store.Records().FindRecordsByStream("s1")
	report := VerifyEventStorageIntegrity("s1", recs)
	if report.Status != "ok" {
		t.Fatalf("valid events must produce ok report, got: %s", report.Status)
	}
	if len(report.InvalidHashes) != 0 {
		t.Fatalf("must have no invalid hashes, got %d", len(report.InvalidHashes))
	}
}
