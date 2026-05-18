package executor

import (
	"errors"
	"sort"
	"sync"
)

// ApprovalStore persists approval decisions for execution gates.
// Approval history must survive restart — it is load-bearing for replay.
type ApprovalStore interface {
	// RecordDecision persists an approval or rejection for a gate.
	RecordDecision(d ApprovalDecision) error
	// GetDecision returns the decision for a gate, or nil if not yet decided.
	GetDecision(executionID, gateID string) (*ApprovalDecision, error)
	// ListDecisions returns all decisions for an execution, ordered by GateID.
	ListDecisions(executionID string) ([]ApprovalDecision, error)
}

// InMemoryApprovalStore is a thread-safe in-memory ApprovalStore.
type InMemoryApprovalStore struct {
	mu        sync.RWMutex
	decisions map[string]map[string]ApprovalDecision // executionID → gateID → decision
}

func NewInMemoryApprovalStore() *InMemoryApprovalStore {
	return &InMemoryApprovalStore{
		decisions: make(map[string]map[string]ApprovalDecision),
	}
}

func (s *InMemoryApprovalStore) RecordDecision(d ApprovalDecision) error {
	if d.ExecutionID == "" || d.GateID == "" {
		return errors.New("approval: decision must have non-empty ExecutionID and GateID")
	}
	if d.OperatorID == "" {
		return errors.New("approval: decision must have non-empty OperatorID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decisions[d.ExecutionID] == nil {
		s.decisions[d.ExecutionID] = make(map[string]ApprovalDecision)
	}
	s.decisions[d.ExecutionID][d.GateID] = d
	return nil
}

func (s *InMemoryApprovalStore) GetDecision(executionID, gateID string) (*ApprovalDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m, ok := s.decisions[executionID]; ok {
		if d, ok := m[gateID]; ok {
			return &d, nil
		}
	}
	return nil, nil
}

func (s *InMemoryApprovalStore) ListDecisions(executionID string) ([]ApprovalDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.decisions[executionID]
	result := make([]ApprovalDecision, 0, len(m))
	for _, d := range m {
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].GateID < result[j].GateID
	})
	return result, nil
}

// IsGateApproved returns true only when the gate has been explicitly approved.
// Returns false if the gate has not been decided or was rejected.
func IsGateApproved(store ApprovalStore, executionID, gateID string) (bool, error) {
	d, err := store.GetDecision(executionID, gateID)
	if err != nil {
		return false, err
	}
	if d == nil {
		return false, nil
	}
	return d.Approved, nil
}

// AllGatesApproved returns true only when every gate in gateIDs has been approved.
// An empty gateIDs list returns true (no gates to satisfy).
func AllGatesApproved(store ApprovalStore, executionID string, gateIDs []string) (bool, error) {
	for _, id := range gateIDs {
		ok, err := IsGateApproved(store, executionID, id)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
