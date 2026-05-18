package governance

import "fmt"

// BuildApprovalHierarchy constructs the ordered set of approval steps required
// for a given risk level and action.
//
// Hierarchy rules (highest priority first):
//   - critical  → two steps: security-lead + admin (multi-approval)
//   - high       → one step:  operator approval
//   - delete action in any environment → one step: operator approval (if not already covered)
//   - low/medium → no approval required (returns a hierarchy with 0 steps)
//
// The returned hierarchy is deterministic for the same inputs.
func BuildApprovalHierarchy(hierarchyID, executionID, stageID, riskLevel, action string) ApprovalHierarchy {
	var steps []ApprovalStep
	var rationale string

	switch riskLevel {
	case "critical":
		rationale = "critical risk level requires multi-stage approval: security lead + admin"
		steps = []ApprovalStep{
			{
				StepID:       fmt.Sprintf("%s-step-1", hierarchyID),
				StepOrder:    1,
				RequiredRole: string(PermissionApproveExecution),
				Reason:       "first approval: operator with execution approval authority",
			},
			{
				StepID:       fmt.Sprintf("%s-step-2", hierarchyID),
				StepOrder:    2,
				RequiredRole: string(PermissionOverride),
				Reason:       "second approval: administrator with override authority (multi-approval requirement)",
			},
		}
	case "high":
		rationale = "high risk level requires operator approval before execution"
		steps = []ApprovalStep{
			{
				StepID:       fmt.Sprintf("%s-step-1", hierarchyID),
				StepOrder:    1,
				RequiredRole: string(PermissionApproveExecution),
				Reason:       "high-risk operation requires explicit operator approval",
			},
		}
	default:
		if action == "delete" {
			rationale = "destructive delete requires operator approval regardless of risk level"
			steps = []ApprovalStep{
				{
					StepID:       fmt.Sprintf("%s-step-1", hierarchyID),
					StepOrder:    1,
					RequiredRole: string(PermissionApproveExecution),
					Reason:       "delete operations require explicit operator confirmation",
				},
			}
		} else {
			rationale = fmt.Sprintf("risk level %q with action %q requires no approval", riskLevel, action)
		}
	}

	return ApprovalHierarchy{
		HierarchyID:  hierarchyID,
		ExecutionID:  executionID,
		StageID:      stageID,
		Steps:        steps,
		Rationale:    rationale,
		AllSatisfied: len(steps) == 0,
	}
}
