package governance

import (
	"fmt"
	"sort"
	"time"
)

// SatisfyApprovalStep marks one step in the hierarchy as satisfied by actorID.
// Returns the updated hierarchy (value semantics — original unchanged).
// Returns an error if the step is not found, already satisfied, or the actor
// does not hold the required role.
func SatisfyApprovalStep(
	hier ApprovalHierarchy,
	stepID string,
	actorID string,
	actorRoles map[string][]string,
	roles []Role,
) (ApprovalHierarchy, error) {
	if actorID == "" {
		return hier, fmt.Errorf("actorID must be non-empty")
	}

	// Find the step.
	stepIdx := -1
	for i, s := range hier.Steps {
		if s.StepID == stepID {
			stepIdx = i
			break
		}
	}
	if stepIdx < 0 {
		return hier, fmt.Errorf("step %q not found in hierarchy %q", stepID, hier.HierarchyID)
	}
	if hier.Steps[stepIdx].Satisfied {
		return hier, fmt.Errorf("step %q already satisfied by %q", stepID, hier.Steps[stepIdx].SatisfiedBy)
	}

	// Enforce role requirement.
	required := Permission(hier.Steps[stepIdx].RequiredRole)
	decision := EvaluateRBAC(actorID, required, actorRoles, roles)
	if !decision.Granted {
		return hier, fmt.Errorf("actor %q lacks role %q required by step %q", actorID, hier.Steps[stepIdx].RequiredRole, stepID)
	}

	// Build updated steps slice (copy-on-write).
	newSteps := append([]ApprovalStep(nil), hier.Steps...)
	newSteps[stepIdx].Satisfied = true
	newSteps[stepIdx].SatisfiedBy = actorID
	newSteps[stepIdx].SatisfiedAt = time.Now().UTC().Format(time.RFC3339)

	allSatisfied := true
	for _, s := range newSteps {
		if !s.Satisfied {
			allSatisfied = false
			break
		}
	}

	return ApprovalHierarchy{
		HierarchyID:  hier.HierarchyID,
		ExecutionID:  hier.ExecutionID,
		StageID:      hier.StageID,
		Steps:        newSteps,
		Rationale:    hier.Rationale,
		AllSatisfied: allSatisfied,
	}, nil
}

// IsHierarchySatisfied returns true when every step in the hierarchy is satisfied.
func IsHierarchySatisfied(hier ApprovalHierarchy) bool {
	if len(hier.Steps) == 0 {
		return false
	}
	for _, s := range hier.Steps {
		if !s.Satisfied {
			return false
		}
	}
	return true
}

// PendingSteps returns the ordered list of steps that are not yet satisfied.
func PendingSteps(hier ApprovalHierarchy) []ApprovalStep {
	var pending []ApprovalStep
	for _, s := range hier.Steps {
		if !s.Satisfied {
			pending = append(pending, s)
		}
	}
	// Already in StepOrder; sort defensively.
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].StepOrder < pending[j].StepOrder
	})
	return pending
}
