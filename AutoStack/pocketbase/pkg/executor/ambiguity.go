package executor

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// AmbiguityStore persists ExecutionAmbiguity records.
// Ambiguities are first-class — they pause execution and require operator resolution.
// NO silent retries after an ambiguity is recorded.
type AmbiguityStore interface {
	// Record persists a new ambiguity. Returns error if AmbiguityID or ExecutionID is empty.
	Record(a ExecutionAmbiguity) error
	// Get returns the ambiguity by ID, or nil if not found.
	Get(ambiguityID string) (*ExecutionAmbiguity, error)
	// ListUnresolved returns all unresolved ambiguities for an execution, ordered by AmbiguityID.
	ListUnresolved(executionID string) ([]ExecutionAmbiguity, error)
	// Resolve marks an ambiguity as resolved with the operator's identity and resolution text.
	// Returns error if already resolved or operatorID is empty.
	Resolve(ambiguityID, operatorID, resolution string) error
}

// InMemoryAmbiguityStore is a thread-safe in-memory AmbiguityStore.
type InMemoryAmbiguityStore struct {
	mu          sync.RWMutex
	ambiguities map[string]ExecutionAmbiguity
}

func NewInMemoryAmbiguityStore() *InMemoryAmbiguityStore {
	return &InMemoryAmbiguityStore{
		ambiguities: make(map[string]ExecutionAmbiguity),
	}
}

func (s *InMemoryAmbiguityStore) Record(a ExecutionAmbiguity) error {
	if a.AmbiguityID == "" {
		return errors.New("ambiguity: AmbiguityID must be non-empty")
	}
	if a.ExecutionID == "" {
		return errors.New("ambiguity: ExecutionID must be non-empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ambiguities[a.AmbiguityID] = a
	return nil
}

func (s *InMemoryAmbiguityStore) Get(ambiguityID string) (*ExecutionAmbiguity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.ambiguities[ambiguityID]
	if !ok {
		return nil, nil
	}
	copy := a
	return &copy, nil
}

func (s *InMemoryAmbiguityStore) ListUnresolved(executionID string) ([]ExecutionAmbiguity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ExecutionAmbiguity
	for _, a := range s.ambiguities {
		if a.ExecutionID == executionID && !a.Resolved {
			result = append(result, a)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].AmbiguityID < result[j].AmbiguityID
	})
	return result, nil
}

func (s *InMemoryAmbiguityStore) Resolve(ambiguityID, operatorID, resolution string) error {
	if operatorID == "" {
		return errors.New("ambiguity: operatorID must be non-empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.ambiguities[ambiguityID]
	if !ok {
		return errors.New("ambiguity: not found: " + ambiguityID)
	}
	if a.Resolved {
		return errors.New("ambiguity: already resolved: " + ambiguityID)
	}
	a.Resolved = true
	a.Resolution = resolution
	a.OperatorID = operatorID
	a.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
	s.ambiguities[ambiguityID] = a
	return nil
}

// NewAmbiguityRecord constructs an ExecutionAmbiguity with a deterministic ID.
// The ID is stable given the same inputs so duplicate ambiguities can be detected.
func NewAmbiguityRecord(executionID, stageID, nodeID, kind, description string) ExecutionAmbiguity {
	return ExecutionAmbiguity{
		AmbiguityID: "ambiguity:" + executionID + ":" + stageID + ":" + nodeID + ":" + kind,
		ExecutionID: executionID,
		StageID:     stageID,
		NodeID:      nodeID,
		Kind:        kind,
		Description: description,
		DetectedAt:  time.Now().UTC().Format(time.RFC3339),
		Resolved:    false,
	}
}
