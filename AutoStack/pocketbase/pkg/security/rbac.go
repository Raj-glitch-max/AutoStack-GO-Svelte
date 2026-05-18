package security

import "fmt"

// Role represents a named authority level within AutoStack.
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleApprover Role = "approver"
	RoleAuditor  Role = "auditor"
	RoleAdmin    Role = "admin"
)

// Permission is a granular capability grant.
type Permission string

const (
	// Read permissions.
	PermReadExecutions    Permission = "read:executions"
	PermReadWorkflows     Permission = "read:workflows"
	PermReadGovernance    Permission = "read:governance"
	PermReadWorkers       Permission = "read:workers"
	PermReadReplay        Permission = "read:replay"
	PermReadContradictions Permission = "read:contradictions"
	PermReadCertifications Permission = "read:certifications"
	PermReadArchive       Permission = "read:archive"

	// Operator actions.
	PermTriggerExecution  Permission = "exec:trigger"
	PermCancelExecution   Permission = "exec:cancel"
	PermQuarantineWorker  Permission = "workers:quarantine"

	// Approval actions.
	PermApproveGovernance Permission = "governance:approve"
	PermDenyGovernance    Permission = "governance:deny"

	// Auditor-only.
	PermExportForensic    Permission = "archive:export-forensic"
	PermReadAuditTrail    Permission = "audit:read"

	// Admin-only.
	PermManageTokens      Permission = "admin:tokens"
	PermManageRoles       Permission = "admin:roles"
	PermRunCertification  Permission = "admin:certification"
)

// rolePermissions is the explicit, immutable permission matrix.
// No permission is granted unless explicitly listed here.
var rolePermissions = map[Role][]Permission{
	RoleViewer: {
		PermReadExecutions, PermReadWorkflows, PermReadWorkers,
		PermReadContradictions, PermReadCertifications,
	},
	RoleOperator: {
		PermReadExecutions, PermReadWorkflows, PermReadGovernance,
		PermReadWorkers, PermReadReplay, PermReadContradictions,
		PermReadCertifications, PermReadArchive,
		PermTriggerExecution, PermCancelExecution, PermQuarantineWorker,
	},
	RoleApprover: {
		PermReadExecutions, PermReadWorkflows, PermReadGovernance,
		PermReadWorkers, PermReadContradictions,
		PermApproveGovernance, PermDenyGovernance,
	},
	RoleAuditor: {
		PermReadExecutions, PermReadWorkflows, PermReadGovernance,
		PermReadWorkers, PermReadReplay, PermReadContradictions,
		PermReadCertifications, PermReadArchive,
		PermExportForensic, PermReadAuditTrail,
	},
	RoleAdmin: {
		PermReadExecutions, PermReadWorkflows, PermReadGovernance,
		PermReadWorkers, PermReadReplay, PermReadContradictions,
		PermReadCertifications, PermReadArchive,
		PermTriggerExecution, PermCancelExecution, PermQuarantineWorker,
		PermApproveGovernance, PermDenyGovernance,
		PermExportForensic, PermReadAuditTrail,
		PermManageTokens, PermManageRoles, PermRunCertification,
	},
}

// RBACDecision is the result of a permission check.
type RBACDecision struct {
	Granted    bool
	Role       Role
	Permission Permission
	Reason     string
}

// CheckPermission returns whether role has the given permission.
// Default-deny: if role is unknown or permission not listed, denies.
func CheckPermission(role Role, perm Permission) RBACDecision {
	perms, ok := rolePermissions[role]
	if !ok {
		return RBACDecision{
			Granted:    false,
			Role:       role,
			Permission: perm,
			Reason:     fmt.Sprintf("unknown role %q", role),
		}
	}
	for _, p := range perms {
		if p == perm {
			return RBACDecision{Granted: true, Role: role, Permission: perm}
		}
	}
	return RBACDecision{
		Granted:    false,
		Role:       role,
		Permission: perm,
		Reason:     fmt.Sprintf("role %q does not have permission %q", role, perm),
	}
}

// AllPermissionsFor returns a copy of the permission list for a role.
func AllPermissionsFor(role Role) []Permission {
	perms := rolePermissions[role]
	out := make([]Permission, len(perms))
	copy(out, perms)
	return out
}

// ValidRole returns true if the role is a known AutoStack role.
func ValidRole(role Role) bool {
	_, ok := rolePermissions[role]
	return ok
}
