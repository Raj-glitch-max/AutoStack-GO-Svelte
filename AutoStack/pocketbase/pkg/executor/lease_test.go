package executor

import (
	"testing"
	"time"
)

func TestLeaseStore_Acquire_Valid(t *testing.T) {
	s := NewInMemoryLeaseStore()
	lease, err := s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	if err != nil {
		t.Fatalf("Acquire must succeed: %v", err)
	}
	if lease == nil {
		t.Fatal("Acquire must return non-nil lease")
	}
	if lease.HolderID != "holder-1" {
		t.Fatalf("lease HolderID mismatch: want holder-1, got %s", lease.HolderID)
	}
}

func TestLeaseStore_Acquire_EmptyExecutionID_Error(t *testing.T) {
	s := NewInMemoryLeaseStore()
	_, err := s.Acquire("", "holder-1", DefaultLeaseDuration)
	if err == nil {
		t.Fatal("Acquire with empty executionID must error")
	}
}

func TestLeaseStore_Acquire_AlreadyHeld_Error(t *testing.T) {
	s := NewInMemoryLeaseStore()
	s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	_, err := s.Acquire("exec-1", "holder-2", DefaultLeaseDuration)
	if err == nil {
		t.Fatal("Acquire on already-held execution must error")
	}
}

func TestLeaseStore_Acquire_ExpiredLease_Allowed(t *testing.T) {
	now := time.Now()
	s := NewInMemoryLeaseStore()
	s.withClock(func() time.Time { return now })
	// Acquire a 1-second lease.
	s.Acquire("exec-1", "holder-1", 1*time.Second)

	// Advance time past expiry.
	s.withClock(func() time.Time { return now.Add(2 * time.Second) })
	_, err := s.Acquire("exec-1", "holder-2", DefaultLeaseDuration)
	if err != nil {
		t.Fatalf("Acquire on expired lease must succeed: %v", err)
	}
}

func TestLeaseStore_Renew_Valid(t *testing.T) {
	s := NewInMemoryLeaseStore()
	s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	lease, err := s.Renew("exec-1", "holder-1", DefaultLeaseDuration)
	if err != nil {
		t.Fatalf("Renew must succeed: %v", err)
	}
	if lease.Renewed != 1 {
		t.Fatalf("renewal count must be 1, got %d", lease.Renewed)
	}
}

func TestLeaseStore_Renew_WrongHolder_Error(t *testing.T) {
	s := NewInMemoryLeaseStore()
	s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	_, err := s.Renew("exec-1", "holder-2", DefaultLeaseDuration)
	if err == nil {
		t.Fatal("Renew by wrong holder must error")
	}
}

func TestLeaseStore_Renew_NoLease_Error(t *testing.T) {
	s := NewInMemoryLeaseStore()
	_, err := s.Renew("exec-1", "holder-1", DefaultLeaseDuration)
	if err == nil {
		t.Fatal("Renew with no existing lease must error")
	}
}

func TestLeaseStore_Renew_ExpiredLease_Error(t *testing.T) {
	now := time.Now()
	s := NewInMemoryLeaseStore()
	s.withClock(func() time.Time { return now })
	s.Acquire("exec-1", "holder-1", 1*time.Second)
	// Mark lease as expired by setting Expired=true.
	s.leases["exec-1"].Expired = true
	_, err := s.Renew("exec-1", "holder-1", DefaultLeaseDuration)
	if err == nil {
		t.Fatal("Renew on expired lease must error")
	}
}

func TestLeaseStore_Release_Valid(t *testing.T) {
	s := NewInMemoryLeaseStore()
	s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	if err := s.Release("exec-1", "holder-1"); err != nil {
		t.Fatalf("Release must succeed: %v", err)
	}
	held, _ := s.IsHeld("exec-1")
	if held {
		t.Fatal("after Release, lease must not be held")
	}
}

func TestLeaseStore_Release_WrongHolder_Error(t *testing.T) {
	s := NewInMemoryLeaseStore()
	s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	if err := s.Release("exec-1", "holder-2"); err == nil {
		t.Fatal("Release by wrong holder must error")
	}
}

func TestLeaseStore_Release_NoLease_NoOp(t *testing.T) {
	s := NewInMemoryLeaseStore()
	if err := s.Release("exec-unknown", "holder-1"); err != nil {
		t.Fatalf("Release with no lease must be a no-op: %v", err)
	}
}

func TestLeaseStore_IsHeld_True_AfterAcquire(t *testing.T) {
	s := NewInMemoryLeaseStore()
	s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	held, err := s.IsHeld("exec-1")
	if err != nil || !held {
		t.Fatal("IsHeld must return true after successful Acquire")
	}
}

func TestLeaseStore_IsHeld_False_AfterRelease(t *testing.T) {
	s := NewInMemoryLeaseStore()
	s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	s.Release("exec-1", "holder-1")
	held, _ := s.IsHeld("exec-1")
	if held {
		t.Fatal("IsHeld must return false after Release")
	}
}

func TestLeaseStore_IsHeld_False_WhenExpired(t *testing.T) {
	now := time.Now()
	s := NewInMemoryLeaseStore()
	s.withClock(func() time.Time { return now })
	s.Acquire("exec-1", "holder-1", 1*time.Second)
	s.withClock(func() time.Time { return now.Add(2 * time.Second) })
	held, _ := s.IsHeld("exec-1")
	if held {
		t.Fatal("IsHeld must return false when lease is expired")
	}
}

func TestLeaseStore_IsHeld_False_WhenNotAcquired(t *testing.T) {
	s := NewInMemoryLeaseStore()
	held, err := s.IsHeld("exec-never")
	if err != nil || held {
		t.Fatal("IsHeld must return false when no lease has been acquired")
	}
}

func TestLeaseStore_Get_ReturnsLease(t *testing.T) {
	s := NewInMemoryLeaseStore()
	s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	lease, err := s.Get("exec-1")
	if err != nil || lease == nil {
		t.Fatal("Get must return non-nil lease after Acquire")
	}
}

func TestLeaseStore_Get_NoLease_ReturnsNil(t *testing.T) {
	s := NewInMemoryLeaseStore()
	lease, err := s.Get("exec-unknown")
	if err != nil || lease != nil {
		t.Fatal("Get with no lease must return nil")
	}
}

func TestLeaseStore_DuplicatePrevention_SameExecution(t *testing.T) {
	s := NewInMemoryLeaseStore()
	s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	// A second executor attempting to acquire the same execution must fail.
	_, err := s.Acquire("exec-1", "holder-1", DefaultLeaseDuration)
	if err == nil {
		t.Fatal("same execution may not be acquired twice concurrently")
	}
}
