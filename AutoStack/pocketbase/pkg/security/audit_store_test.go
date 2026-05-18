package security

import (
	"fmt"
	"testing"
	"time"
)

func newTestAuditStore(t *testing.T) *PersistentAuditStore {
	t.Helper()
	store, err := NewSQLiteAuditStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteAuditStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// buildEntry creates a standalone AuditEntry with valid hashes (no trail dependency).
func buildEntry(entryID, tenantID string, prevHash string) AuditEntry {
	e := AuditEntry{
		EntryID:    entryID,
		EventType:  AuditEventAuthSuccess,
		TenantID:   tenantID,
		RequestID:  "req-" + entryID,
		ActorRole:  string(RoleOperator),
		Outcome:    "allowed",
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	e.EntryHash = computeAuditEntryHash(e)
	e.ChainHash = computeChainHash(prevHash, e.EntryHash)
	return e
}

func TestPersistentAuditStore_AppendAndRetrieve(t *testing.T) {
	store := newTestAuditStore(t)
	e1 := buildEntry("e-1", "tenant-a", "")
	if err := store.Append(e1); err != nil {
		t.Fatalf("Append: %v", err)
	}
	entries, err := store.AllEntries()
	if err != nil {
		t.Fatalf("AllEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].EntryID != "e-1" {
		t.Errorf("unexpected entry ID: %s", entries[0].EntryID)
	}
}

func TestPersistentAuditStore_DuplicateEntryRejected(t *testing.T) {
	store := newTestAuditStore(t)
	e := buildEntry("dup", "tenant-a", "")
	store.Append(e)
	if err := store.Append(e); err == nil {
		t.Fatal("duplicate entry ID should be rejected (append-only)")
	}
}

func TestPersistentAuditStore_EntriesForTenant(t *testing.T) {
	store := newTestAuditStore(t)
	e1 := buildEntry("e-1", "tenant-a", "")
	e2 := buildEntry("e-2", "tenant-b", "")
	e3 := buildEntry("e-3", "tenant-a", e1.EntryHash)
	store.Append(e1)
	store.Append(e2)
	store.Append(e3)

	tenantA, err := store.EntriesForTenant("tenant-a")
	if err != nil {
		t.Fatalf("EntriesForTenant: %v", err)
	}
	if len(tenantA) != 2 {
		t.Errorf("expected 2 entries for tenant-a, got %d", len(tenantA))
	}
}

func TestPersistentAuditStore_LatestChainHash(t *testing.T) {
	store := newTestAuditStore(t)

	h, err := store.LatestChainHash()
	if err != nil {
		t.Fatalf("LatestChainHash on empty: %v", err)
	}
	if h != "" {
		t.Errorf("expected empty chain hash for empty store, got %s", h)
	}

	e1 := buildEntry("e-1", "tenant-a", "")
	store.Append(e1)
	h2, _ := store.LatestChainHash()
	if h2 != e1.ChainHash {
		t.Errorf("expected chain hash %s, got %s", e1.ChainHash, h2)
	}
}

func TestPersistentAuditStore_Count(t *testing.T) {
	store := newTestAuditStore(t)
	e1 := buildEntry("e-1", "tenant-a", "")
	e2 := buildEntry("e-2", "tenant-a", e1.EntryHash)
	store.Append(e1)
	store.Append(e2)
	n, err := store.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
}

func TestVerifyAuditChain_Intact(t *testing.T) {
	store := newTestAuditStore(t)
	prev := ""
	for i := 0; i < 10; i++ {
		e := buildEntry(fmt.Sprintf("e-%d", i), "tenant-a", prev)
		store.Append(e)
		prev = e.EntryHash
	}
	ok, reason := VerifyAuditChain(store)
	if !ok {
		t.Fatalf("audit chain should be intact: %s", reason)
	}
}

func TestVerifyAuditChain_TamperedEntryHash(t *testing.T) {
	store := newTestAuditStore(t)
	e1 := buildEntry("e-1", "tenant-a", "")
	store.Append(e1)

	// Tamper entry hash directly in DB.
	store.db.Exec(`UPDATE audit_entries SET entry_hash='tampered' WHERE entry_id='e-1'`)

	ok, _ := VerifyAuditChain(store)
	if ok {
		t.Fatal("tampered entry hash should fail chain verification")
	}
}

func TestVerifyAuditChain_BrokenChainHash(t *testing.T) {
	store := newTestAuditStore(t)
	e1 := buildEntry("e-1", "tenant-a", "")
	e2 := buildEntry("e-2", "tenant-a", e1.EntryHash)
	store.Append(e1)
	store.Append(e2)

	// Tamper chain hash of e2.
	store.db.Exec(`UPDATE audit_entries SET chain_hash='broken' WHERE entry_id='e-2'`)

	ok, reason := VerifyAuditChain(store)
	if ok {
		t.Fatal("broken chain hash should fail verification")
	}
	if reason == "" {
		t.Error("verification should return a reason")
	}
}

func TestVerifyAuditChain_EmptyStore(t *testing.T) {
	store := newTestAuditStore(t)
	ok, _ := VerifyAuditChain(store)
	if !ok {
		t.Error("empty store should pass chain verification")
	}
}

func TestDurableAuditRecorder_RecordAndVerify(t *testing.T) {
	store := newTestAuditStore(t)
	recorder := NewDurableAuditRecorder(store)

	_, err := recorder.Record("e-1", AuditEventAuthSuccess, "tenant-a", "req-1", string(RoleOperator), "allowed")
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	_, err = recorder.Record("e-2", AuditEventPermDenied, "tenant-a", "req-2", string(RoleViewer), "denied",
		WithPermission(PermTriggerExecution))
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	ok, reason := recorder.VerifyIntegrity()
	if !ok {
		t.Fatalf("integrity check failed: %s", reason)
	}

	n, _ := store.Count()
	if n != 2 {
		t.Errorf("expected 2 persisted entries, got %d", n)
	}
}

func TestDurableAuditRecorder_Restore_ContinuesChain(t *testing.T) {
	store := newTestAuditStore(t)

	// Session 1: record some entries.
	rec1 := NewDurableAuditRecorder(store)
	rec1.Record("e-1", AuditEventAuthSuccess, "tenant-a", "r1", string(RoleOperator), "allowed")
	rec1.Record("e-2", AuditEventGovernanceAct, "tenant-a", "r2", string(RoleApprover), "allowed")

	// Session 2: create a new recorder, restore from store, then record more.
	rec2 := NewDurableAuditRecorder(store)
	if err := rec2.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(rec2.Entries()) != 2 {
		t.Errorf("expected 2 restored entries, got %d", len(rec2.Entries()))
	}

	// Record a new entry — it must chain from the restored tail.
	_, err := rec2.Record("e-3", AuditEventRateLimited, "tenant-a", "r3", string(RoleOperator), "denied")
	if err != nil {
		t.Fatalf("Record after restore: %v", err)
	}

	ok, reason := rec2.VerifyIntegrity()
	if !ok {
		t.Fatalf("integrity after restore + append: %s", reason)
	}

	n, _ := store.Count()
	if n != 3 {
		t.Errorf("expected 3 persisted entries, got %d", n)
	}
}

func TestDurableAuditRecorder_VerifyIntegrity_DetectsTamper(t *testing.T) {
	store := newTestAuditStore(t)
	rec := NewDurableAuditRecorder(store)
	rec.Record("e-1", AuditEventAuthSuccess, "tenant-a", "r1", string(RoleAdmin), "allowed")

	// Tamper in DB.
	store.db.Exec(`UPDATE audit_entries SET outcome='denied' WHERE entry_id='e-1'`)
	// NOTE: in-process trail is unchanged, DB is tampered.
	// VerifyIntegrity checks the DB chain.
	ok, _ := rec.VerifyIntegrity()
	// DB tampering breaks the stored entry hash — chain verification will catch it.
	if ok {
		t.Fatal("tampered DB entry should fail integrity check")
	}
}

func TestExportTenantAudit(t *testing.T) {
	store := newTestAuditStore(t)
	rec := NewDurableAuditRecorder(store)
	rec.Record("e-1", AuditEventAuthSuccess, "tenant-a", "r1", string(RoleOperator), "allowed",
		WithMetadata(map[string]string{"ip": "10.0.0.1"}))
	rec.Record("e-2", AuditEventAuthSuccess, "tenant-b", "r2", string(RoleViewer), "allowed")

	exports, err := ExportTenantAudit(store, "tenant-a")
	if err != nil {
		t.Fatalf("ExportTenantAudit: %v", err)
	}
	if len(exports) != 1 {
		t.Fatalf("expected 1 export for tenant-a, got %d", len(exports))
	}
	// Metadata is NOT exported (could contain sensitive values).
	// The export type has no Metadata field — this is structural.
	if exports[0].EntryHash == "" {
		t.Error("export must include entry hash for verification")
	}
}

func TestPersistentAuditStore_TimestampMonotonicity(t *testing.T) {
	store := newTestAuditStore(t)
	// Insert two entries with correct ordering.
	e1 := buildEntry("e-1", "tenant-a", "")
	time.Sleep(time.Millisecond) // ensure different timestamps
	e2 := buildEntry("e-2", "tenant-a", e1.EntryHash)
	store.Append(e1)
	store.Append(e2)

	ok, reason := VerifyAuditChain(store)
	if !ok {
		t.Fatalf("monotonic timestamps should pass: %s", reason)
	}
}

func TestPersistentAuditStore_OrderIsInsertionOrder(t *testing.T) {
	store := newTestAuditStore(t)
	prev := ""
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("e-%d", i)
		e := buildEntry(id, "tenant-a", prev)
		store.Append(e)
		prev = e.EntryHash
	}
	entries, _ := store.AllEntries()
	for i, e := range entries {
		expected := fmt.Sprintf("e-%d", i)
		if e.EntryID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, e.EntryID)
		}
	}
}
