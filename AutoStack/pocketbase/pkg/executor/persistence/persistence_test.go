package persistence

import (
	"testing"

	"github.com/janlauber/one-click/pkg/executor"
)

// Static compile-time interface verification — these assignments fail to build
// if any store does not fully implement the interface.
var (
	_ executor.JournalStore   = (*PocketBaseJournalStore)(nil)
	_ executor.ApprovalStore  = (*PocketBaseApprovalStore)(nil)
	_ executor.LeaseStore     = (*PocketBaseLeaseStore)(nil)
	_ executor.AmbiguityStore = (*PocketBaseAmbiguityStore)(nil)
)

// TestPersistence_StaticCompile is a trivial guard ensuring the above vars
// are not dead-code-eliminated. The real check is at compile time.
func TestPersistence_StaticCompile(t *testing.T) {
	// Deliberate: no runtime assertions needed — compile-time is the guard.
}

// TestPersistence_NewJournalStore_NilDao_Panics verifies the nil guard.
func TestPersistence_NewJournalStore_NilDao_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewPocketBaseJournalStore(nil) must panic")
		}
	}()
	NewPocketBaseJournalStore(nil)
}

// TestPersistence_NewApprovalStore_NilDao_Panics verifies the nil guard.
func TestPersistence_NewApprovalStore_NilDao_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewPocketBaseApprovalStore(nil) must panic")
		}
	}()
	NewPocketBaseApprovalStore(nil)
}

// TestPersistence_NewLeaseStore_NilDao_Panics verifies the nil guard.
func TestPersistence_NewLeaseStore_NilDao_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewPocketBaseLeaseStore(nil) must panic")
		}
	}()
	NewPocketBaseLeaseStore(nil)
}

// TestPersistence_NewAmbiguityStore_NilDao_Panics verifies the nil guard.
func TestPersistence_NewAmbiguityStore_NilDao_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewPocketBaseAmbiguityStore(nil) must panic")
		}
	}()
	NewPocketBaseAmbiguityStore(nil)
}

// TestPersistence_IsNotFound_NilError verifies the helper does not panic on nil.
func TestPersistence_IsNotFound_NilError(t *testing.T) {
	if isNotFound(nil) {
		t.Fatal("isNotFound(nil) must return false")
	}
}
