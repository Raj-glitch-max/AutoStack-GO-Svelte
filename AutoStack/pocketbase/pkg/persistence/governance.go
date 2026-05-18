package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/janlauber/one-click/pkg/policy"
)

// GovernancePersistenceStore persists policy runtime snapshots, decisions, and assessments.
type GovernancePersistenceStore struct {
	kv DurableKVStore
}

// NewGovernancePersistenceStore wraps a KV store for governance persistence.
func NewGovernancePersistenceStore(kv DurableKVStore) *GovernancePersistenceStore {
	return &GovernancePersistenceStore{kv: kv}
}

// SaveGovernanceSnapshot persists a GovernanceRuntimeSnapshot.
func (s *GovernancePersistenceStore) SaveGovernanceSnapshot(ctx context.Context, snap policy.GovernanceRuntimeSnapshot) error {
	if ok, reason := policy.VerifyGovernanceSnapshot(snap); !ok {
		return fmt.Errorf("governance store: invalid snapshot hash: %s", reason)
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("governance store: marshal: %w", err)
	}
	return s.kv.Put(ctx, "govsnap:"+snap.SnapshotID, b)
}

// LoadGovernanceSnapshot retrieves a snapshot by ID.
func (s *GovernancePersistenceStore) LoadGovernanceSnapshot(ctx context.Context, snapshotID string) (policy.GovernanceRuntimeSnapshot, bool, error) {
	b, ok, err := s.kv.Get(ctx, "govsnap:"+snapshotID)
	if err != nil || !ok {
		return policy.GovernanceRuntimeSnapshot{}, ok, err
	}
	var snap policy.GovernanceRuntimeSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return policy.GovernanceRuntimeSnapshot{}, false, err
	}
	return snap, true, nil
}

// SaveDecision persists a PolicyDecision.
func (s *GovernancePersistenceStore) SaveDecision(ctx context.Context, d policy.PolicyDecision) error {
	if ok, reason := policy.VerifyDecisionHash(d); !ok {
		return fmt.Errorf("governance store: invalid decision hash: %s", reason)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("governance store: decision marshal: %w", err)
	}
	return s.kv.Put(ctx, "decision:"+d.DecisionID, b)
}

// AllDecisions returns all persisted decisions for a tenantID.
func (s *GovernancePersistenceStore) AllDecisions(ctx context.Context, tenantID string) ([]policy.PolicyDecision, error) {
	all, err := s.kv.List(ctx, "decision:")
	if err != nil {
		return nil, err
	}
	var out []policy.PolicyDecision
	for _, b := range all {
		var d policy.PolicyDecision
		if err := json.Unmarshal(b, &d); err != nil {
			continue
		}
		if d.TenantID == tenantID {
			out = append(out, d)
		}
	}
	return out, nil
}

// SaveAssessment persists a RuntimeConfidenceAssessment.
func (s *GovernancePersistenceStore) SaveAssessment(ctx context.Context, a policy.RuntimeConfidenceAssessment) error {
	if ok, reason := policy.VerifyAssessmentHash(a); !ok {
		return fmt.Errorf("governance store: invalid assessment hash: %s", reason)
	}
	b, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("governance store: assessment marshal: %w", err)
	}
	return s.kv.Put(ctx, "assessment:"+a.AssessmentID, b)
}
