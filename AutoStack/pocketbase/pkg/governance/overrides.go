package governance

import (
	"fmt"
	"time"
)

// RecordOverride creates a GovernanceOverride record.
// All three of actorID, reason, and evidence are mandatory — a missing field
// returns an error and no override is recorded.
// Overrides are intended to be persisted forever by the caller and can never
// be deleted.
func RecordOverride(
	overrideID string,
	executionID string,
	actorID string,
	reason string,
	evidence []string,
	policyID string,
) (GovernanceOverride, error) {
	if overrideID == "" {
		return GovernanceOverride{}, fmt.Errorf("overrideID must be non-empty")
	}
	if executionID == "" {
		return GovernanceOverride{}, fmt.Errorf("executionID must be non-empty")
	}
	if actorID == "" {
		return GovernanceOverride{}, fmt.Errorf("actorID is mandatory for overrides")
	}
	if reason == "" {
		return GovernanceOverride{}, fmt.Errorf("reason is mandatory for overrides")
	}
	if len(evidence) == 0 {
		return GovernanceOverride{}, fmt.Errorf("at least one evidence item is mandatory for overrides")
	}
	if policyID == "" {
		return GovernanceOverride{}, fmt.Errorf("policyID must be non-empty")
	}

	// Deep copy evidence so the caller cannot mutate the record.
	ev := append([]string(nil), evidence...)

	return GovernanceOverride{
		OverrideID:   overrideID,
		ExecutionID:  executionID,
		ActorID:      actorID,
		Reason:       reason,
		Evidence:     ev,
		PolicyID:     policyID,
		OverriddenAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
