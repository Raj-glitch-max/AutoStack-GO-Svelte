package executor

import (
	"errors"
	"sync"
	"time"
)

const (
	// DefaultLeaseDuration is the default execution lease TTL.
	DefaultLeaseDuration = 5 * time.Minute
)

// LeaseStore manages execution leases to prevent concurrent executor instances.
// Single-node only — no distributed consensus required.
type LeaseStore interface {
	// Acquire attempts to acquire a lease. Returns error if already held and not expired.
	Acquire(executionID, holderID string, duration time.Duration) (*ExecutionLease, error)
	// Renew extends the lease TTL. Returns error if not held by holderID.
	Renew(executionID, holderID string, duration time.Duration) (*ExecutionLease, error)
	// Release surrenders the lease. No-op if lease does not exist.
	Release(executionID, holderID string) error
	// Get returns a copy of the current lease, or nil if none exists.
	Get(executionID string) (*ExecutionLease, error)
	// IsHeld returns true if the lease exists, is held, and has not expired.
	IsHeld(executionID string) (bool, error)
}

// InMemoryLeaseStore is a thread-safe in-memory LeaseStore.
// clock is injectable for deterministic testing.
type InMemoryLeaseStore struct {
	mu     sync.RWMutex
	leases map[string]*ExecutionLease
	clock  func() time.Time
}

func NewInMemoryLeaseStore() *InMemoryLeaseStore {
	return &InMemoryLeaseStore{
		leases: make(map[string]*ExecutionLease),
		clock:  time.Now,
	}
}

// withClock returns a store that uses the given clock function.
// Used in tests to control time.
func (s *InMemoryLeaseStore) withClock(clock func() time.Time) *InMemoryLeaseStore {
	s.clock = clock
	return s
}

func (s *InMemoryLeaseStore) Acquire(executionID, holderID string, duration time.Duration) (*ExecutionLease, error) {
	if executionID == "" || holderID == "" {
		return nil, errors.New("lease: executionID and holderID must be non-empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock()
	if existing, ok := s.leases[executionID]; ok && !existing.Expired {
		if exp, err := time.Parse(time.RFC3339, existing.ExpiresAt); err == nil && now.Before(exp) {
			return nil, errors.New("lease: already held by " + existing.HolderID)
		}
		// Expired — mark it and allow re-acquisition.
		existing.Expired = true
	}

	lease := &ExecutionLease{
		LeaseID:     executionID + ":" + holderID,
		ExecutionID: executionID,
		HolderID:    holderID,
		AcquiredAt:  now.Format(time.RFC3339),
		ExpiresAt:   now.Add(duration).Format(time.RFC3339),
		Renewed:     0,
		Expired:     false,
	}
	s.leases[executionID] = lease
	copy := *lease
	return &copy, nil
}

func (s *InMemoryLeaseStore) Renew(executionID, holderID string, duration time.Duration) (*ExecutionLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lease, ok := s.leases[executionID]
	if !ok {
		return nil, errors.New("lease: no lease found for " + executionID)
	}
	if lease.HolderID != holderID {
		return nil, errors.New("lease: held by different holder: " + lease.HolderID)
	}
	if lease.Expired {
		return nil, errors.New("lease: cannot renew expired lease")
	}
	now := s.clock()
	lease.ExpiresAt = now.Add(duration).Format(time.RFC3339)
	lease.Renewed++
	copy := *lease
	return &copy, nil
}

func (s *InMemoryLeaseStore) Release(executionID, holderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lease, ok := s.leases[executionID]
	if !ok {
		return nil // no-op
	}
	if lease.HolderID != holderID {
		return errors.New("lease: cannot release — held by different holder: " + lease.HolderID)
	}
	lease.Expired = true
	return nil
}

func (s *InMemoryLeaseStore) Get(executionID string) (*ExecutionLease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lease, ok := s.leases[executionID]
	if !ok {
		return nil, nil
	}
	copy := *lease
	return &copy, nil
}

func (s *InMemoryLeaseStore) IsHeld(executionID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lease, ok := s.leases[executionID]
	if !ok || lease.Expired {
		return false, nil
	}
	now := s.clock()
	if exp, err := time.Parse(time.RFC3339, lease.ExpiresAt); err == nil && now.After(exp) {
		return false, nil
	}
	return true, nil
}
