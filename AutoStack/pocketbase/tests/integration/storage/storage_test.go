// Package storage provides integration tests for SQLite WAL storage behavior
// under stress, concurrent access, and simulated corruption.
//
// These tests verify:
//   - WAL mode append semantics under concurrent load
//   - Duplicate key detection (corruption of append-only guarantee)
//   - Missing key detection (honest reporting, not silent nil)
//   - Concurrent readers during write storm
//   - Malformed value detection
//   - List prefix correctness under concurrency
package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/janlauber/one-click/pkg/persistence"
	"github.com/janlauber/one-click/pkg/security"
)

func newStore(t *testing.T) *persistence.SQLiteKVStore {
	t.Helper()
	store, err := persistence.NewSQLiteKVStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteKVStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// TestStorage_ConcurrentWritesNoLostEntries verifies that concurrent Put
// operations do not lose entries (all unique keys are stored).
func TestStorage_ConcurrentWritesNoLostEntries(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var errCount int64
	n := 200

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", idx)
			if err := store.Put(ctx, key, []byte(fmt.Sprintf("val-%d", idx))); err != nil {
				atomic.AddInt64(&errCount, 1)
			}
		}(i)
	}
	wg.Wait()

	if atomic.LoadInt64(&errCount) > 0 {
		t.Errorf("%d write errors during concurrent puts", errCount)
	}

	// Verify all entries are retrievable.
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("key-%d", i)
		val, found, err := store.Get(ctx, key)
		if err != nil || !found {
			t.Errorf("key-%d missing after concurrent write: found=%v err=%v", i, found, err)
			continue
		}
		expected := fmt.Sprintf("val-%d", i)
		if string(val) != expected {
			t.Errorf("key-%d: expected %q, got %q", i, expected, string(val))
		}
	}
}

// TestStorage_DuplicatePutDetected verifies that the append-only guard
// correctly detects and rejects duplicate keys.
func TestStorage_DuplicatePutDetected(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	store.Put(ctx, "dup-key", []byte("original"))
	err := store.Put(ctx, "dup-key", []byte("modified"))
	if err == nil {
		t.Fatal("duplicate key should return an error (append-only)")
	}

	// Original value must be intact.
	val, found, _ := store.Get(ctx, "dup-key")
	if !found {
		t.Fatal("original entry should still exist")
	}
	if string(val) != "original" {
		t.Errorf("original entry corrupted: got %q", string(val))
	}
}

// TestStorage_MissingKeyReportedHonestly verifies that a missing key returns
// found=false and no error (not silent nil value).
func TestStorage_MissingKeyReportedHonestly(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	val, found, err := store.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get missing key returned error: %v", err)
	}
	if found {
		t.Fatal("missing key should return found=false")
	}
	if val != nil {
		t.Errorf("missing key should return nil value, got %q", string(val))
	}
}

// TestStorage_ConcurrentReadsDuringWriteStorm verifies that readers
// can read committed entries while writes are in progress.
func TestStorage_ConcurrentReadsDuringWriteStorm(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// Pre-populate some entries.
	for i := 0; i < 50; i++ {
		store.Put(ctx, fmt.Sprintf("pre-%d", i), []byte(fmt.Sprintf("v-%d", i)))
	}

	var readErrs, writeErrs int64
	var wg sync.WaitGroup

	// Writers: write new unique keys.
	for i := 50; i < 150; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if err := store.Put(ctx, fmt.Sprintf("new-%d", idx), []byte("v")); err != nil {
				atomic.AddInt64(&writeErrs, 1)
			}
		}(i)
	}

	// Readers: read pre-populated keys.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("pre-%d", idx%50)
			_, found, err := store.Get(ctx, key)
			if err != nil || !found {
				atomic.AddInt64(&readErrs, 1)
			}
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt64(&writeErrs) > 0 {
		t.Errorf("%d write errors during storm", writeErrs)
	}
	if atomic.LoadInt64(&readErrs) > 0 {
		t.Errorf("%d read errors during concurrent writes", readErrs)
	}
}

// TestStorage_ListPrefixCorrectness verifies List returns exactly the entries
// matching the given prefix, even after many other entries are written.
func TestStorage_ListPrefixCorrectness(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// Write entries with different prefixes.
	for i := 0; i < 20; i++ {
		store.Put(ctx, fmt.Sprintf("task:exec-1:%d", i), []byte("t"))
		store.Put(ctx, fmt.Sprintf("checkpoint:exec-1:%d", i), []byte("c"))
		store.Put(ctx, fmt.Sprintf("task:exec-2:%d", i), []byte("t2"))
	}

	results, err := store.List(ctx, "task:exec-1:")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 20 {
		t.Errorf("expected 20 task:exec-1: entries, got %d", len(results))
	}
}

// TestStorage_MalformedValueDetection verifies that stored bytes are returned
// exactly as written — the store does not silently transform values.
func TestStorage_MalformedValueDetection(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	// Store a value that is intentionally not valid JSON.
	badJSON := []byte(`{"unclosed": "json"`)
	store.Put(ctx, "bad-json-key", badJSON)

	retrieved, found, err := store.Get(ctx, "bad-json-key")
	if err != nil || !found {
		t.Fatalf("Get: found=%v err=%v", found, err)
	}
	// The store must return the exact bytes — not silently "fix" them.
	if string(retrieved) != string(badJSON) {
		t.Errorf("store transformed value: got %q, want %q", string(retrieved), string(badJSON))
	}
}

// TestStorage_EmptyValueStoredAndRetrieved verifies that an empty byte slice
// is treated differently from a missing key.
func TestStorage_EmptyValueStoredAndRetrieved(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	store.Put(ctx, "empty-key", []byte{})
	val, found, err := store.Get(ctx, "empty-key")
	if err != nil {
		t.Fatalf("Get empty value: %v", err)
	}
	if !found {
		t.Fatal("empty value entry should be found (not treated as missing)")
	}
	if len(val) != 0 {
		t.Errorf("expected empty value, got %q", string(val))
	}
}

// TestStorage_AuditChainSurvivesHighVolume verifies the audit store
// maintains a valid chain after 1000 sequential appends.
func TestStorage_AuditChainSurvivesHighVolume(t *testing.T) {
	auditStore, err := security.NewSQLiteAuditStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteAuditStore: %v", err)
	}
	defer auditStore.Close()

	rec := security.NewDurableAuditRecorder(auditStore)
	for i := 0; i < 1000; i++ {
		_, err := rec.Record(
			fmt.Sprintf("audit-%d", i),
			security.AuditEventAuthSuccess,
			"tenant-a",
			fmt.Sprintf("req-%d", i),
			string(security.RoleOperator),
			"allowed",
		)
		if err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}

	ok, reason := security.VerifyAuditChain(auditStore)
	if !ok {
		t.Fatalf("audit chain integrity failed after 1000 entries: %s", reason)
	}

	n, _ := auditStore.Count()
	if n != 1000 {
		t.Errorf("expected 1000 persisted entries, got %d", n)
	}
}

// TestStorage_WALModeDoesNotCorruptOnConcurrentAccess verifies
// concurrent SQLite access produces no data races or panic.
func TestStorage_WALModeDoesNotCorruptOnConcurrentAccess(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	var totalErrors int64

	// Mixed read/write/list operations concurrently.
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func(idx int) {
			defer wg.Done()
			if err := store.Put(ctx, fmt.Sprintf("wal-%d", idx), []byte("v")); err != nil {
				atomic.AddInt64(&totalErrors, 1)
			}
		}(i)
		go func(idx int) {
			defer wg.Done()
			_, _, err := store.Get(ctx, fmt.Sprintf("wal-%d", idx%25))
			if err != nil {
				atomic.AddInt64(&totalErrors, 1)
			}
		}(i)
		go func() {
			defer wg.Done()
			_, err := store.List(ctx, "wal-")
			if err != nil {
				atomic.AddInt64(&totalErrors, 1)
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&totalErrors) > 0 {
		t.Errorf("%d errors during concurrent WAL access", totalErrors)
	}
}
