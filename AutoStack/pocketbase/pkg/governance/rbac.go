package governance

import (
	"fmt"
	"sort"
	"time"
)

// HasPermission returns true when role contains the required Permission.
func HasPermission(role Role, required Permission) bool {
	for _, p := range role.Permissions {
		if p == required {
			return true
		}
	}
	return false
}

// EvaluateRBAC determines whether any role granted to actorID carries the required Permission.
//
// actorRoles maps actorID → list of role IDs held by that actor.
// roles is the full role registry.
//
// Returns (granted, decision). The decision carries the reason and the granting role.
// Pure function: same inputs → same output.
func EvaluateRBAC(
	actorID string,
	required Permission,
	actorRoles map[string][]string,
	roles []Role,
) RBACDecision {
	now := time.Now().UTC().Format(time.RFC3339)

	if actorID == "" {
		return RBACDecision{
			ActorID:   actorID,
			Required:  required,
			Granted:   false,
			DeniedBy:  "system",
			Reason:    "actor ID must be non-empty",
			DecidedAt: now,
		}
	}

	// Build role index.
	roleIndex := make(map[string]Role, len(roles))
	for _, r := range roles {
		roleIndex[r.RoleID] = r
	}

	// Check actor's roles in deterministic order (sorted role IDs).
	heldIDs := actorRoles[actorID]
	sorted := append([]string(nil), heldIDs...)
	sort.Strings(sorted)

	for _, roleID := range sorted {
		role, ok := roleIndex[roleID]
		if !ok {
			continue
		}
		if HasPermission(role, required) {
			return RBACDecision{
				ActorID:   actorID,
				Required:  required,
				Granted:   true,
				GrantedBy: roleID,
				Reason:    fmt.Sprintf("role %q grants permission %q", roleID, required),
				DecidedAt: now,
			}
		}
	}

	return RBACDecision{
		ActorID:   actorID,
		Required:  required,
		Granted:   false,
		DeniedBy:  "rbac-engine",
		Reason:    fmt.Sprintf("actor %q holds no role granting %q", actorID, required),
		DecidedAt: now,
	}
}

// PredefinedRoles returns the canonical built-in role set.
// These roles are starting points; operators should extend or restrict them.
func PredefinedRoles() []Role {
	return []Role{
		{
			RoleID: "operator",
			Name:   "Operator",
			Permissions: []Permission{
				PermissionViewExecution,
				PermissionApproveExecution,
				PermissionCancelExecution,
				PermissionViewGovernance,
				PermissionViewAudit,
				PermissionExportSnapshot,
				PermissionViewReplay,
			},
			Scope:       Scope{AllProviders: true, AllEnvironments: true},
			Description: "Standard operator with full execution visibility and approval authority.",
		},
		{
			RoleID: "auditor",
			Name:   "Auditor",
			Permissions: []Permission{
				PermissionViewExecution,
				PermissionViewGovernance,
				PermissionViewAudit,
				PermissionExportSnapshot,
				PermissionViewReplay,
			},
			Scope:       Scope{AllProviders: true, AllEnvironments: true},
			Description: "Read-only access for compliance and audit purposes.",
		},
		{
			RoleID: "admin",
			Name:   "Administrator",
			Permissions: []Permission{
				PermissionViewExecution,
				PermissionApproveExecution,
				PermissionCancelExecution,
				PermissionViewGovernance,
				PermissionOverride,
				PermissionViewAudit,
				PermissionExportSnapshot,
				PermissionViewReplay,
				PermissionManageRoles,
				PermissionManagePolicies,
			},
			Scope:       Scope{AllProviders: true, AllEnvironments: true},
			Description: "Full administrative access including override authority.",
		},
	}
}
