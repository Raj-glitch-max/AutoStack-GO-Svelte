// Package governance provides deterministic policy enforcement, RBAC,
// immutable audit chains, and approval hierarchies.
//
// Core invariants:
//   - All functions are pure: same inputs → same output.
//   - The audit chain is append-only and tamper-evident.
//   - No hidden admin bypasses. No auto-approval. No silent permission escalation.
//   - Every override requires actor, reason, and evidence — all persisted forever.
//   - Policy evaluation is deterministic: same request + same policies → same decision.
package governance

const GovernanceSnapshotSchemaVersion = "4.7.0"

// ─── RBAC ──────────────────────────────────────────────────────────────────

// Permission is a typed capability that a Role may grant.
type Permission string

const (
	PermissionViewExecution    Permission = "view:execution"
	PermissionApproveExecution Permission = "approve:execution"
	PermissionCancelExecution  Permission = "cancel:execution"
	PermissionViewGovernance   Permission = "view:governance"
	PermissionOverride         Permission = "override"
	PermissionViewAudit        Permission = "view:audit"
	PermissionExportSnapshot   Permission = "export:snapshot"
	PermissionViewReplay       Permission = "view:replay"
	PermissionManageRoles      Permission = "manage:roles"
	PermissionManagePolicies   Permission = "manage:policies"
)

// Scope constrains which providers and environments a Role applies to.
type Scope struct {
	// AllProviders grants the permission across every provider.
	AllProviders bool     `json:"all_providers"`
	Providers    []string `json:"providers,omitempty"`
	// AllEnvironments grants the permission across every environment.
	AllEnvironments bool     `json:"all_environments"`
	Environments    []string `json:"environments,omitempty"`
}

// Role groups a set of Permissions under a named identity with an optional Scope.
type Role struct {
	RoleID      string       `json:"role_id"`
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions"`
	Scope       Scope        `json:"scope"`
	// Description explains the purpose of this role.
	Description string `json:"description"`
}

// RBACDecision is the output of evaluating whether an actor has a permission.
type RBACDecision struct {
	ActorID    string     `json:"actor_id"`
	Required   Permission `json:"required"`
	Granted    bool       `json:"granted"`
	GrantedBy  string     `json:"granted_by,omitempty"` // role ID that granted it
	DeniedBy   string     `json:"denied_by,omitempty"`
	Reason     string     `json:"reason"`
	DecidedAt  string     `json:"decided_at"`
}

// ─── Policies ──────────────────────────────────────────────────────────────

// PolicyEffect is the outcome of a policy evaluation.
type PolicyEffect string

const (
	PolicyEffectAllow               PolicyEffect = "allow"
	PolicyEffectDeny                PolicyEffect = "deny"
	PolicyEffectRequireApproval     PolicyEffect = "require-approval"
	PolicyEffectRequireMultiApproval PolicyEffect = "require-multi-approval"
)

// PolicyCondition is one matching criterion in a Policy.
type PolicyCondition struct {
	Field    string `json:"field"`    // "action","risk_level","provider","environment"
	Operator string `json:"operator"` // "eq","in","gte"
	Value    string `json:"value"`
}

// Policy defines what is allowed, denied, or gated behind approval.
type Policy struct {
	PolicyID   string            `json:"policy_id"`
	Name       string            `json:"name"`
	Effect     PolicyEffect      `json:"effect"`
	Conditions []PolicyCondition `json:"conditions"`
	Priority   int               `json:"priority"` // higher priority evaluated first
	// Limitations documents known gaps in this policy's coverage.
	Limitations []string `json:"limitations"`
}

// PolicyDecision records the outcome of evaluating a set of policies against a request.
type PolicyDecision struct {
	RequestID            string            `json:"request_id"`
	PolicyID             string            `json:"policy_id"`
	Effect               PolicyEffect      `json:"effect"`
	Reason               string            `json:"reason"`
	ApplicableConditions []PolicyCondition `json:"applicable_conditions"`
	DecidedAt            string            `json:"decided_at"`
}

// PolicyRequest is the input to EvaluatePolicy.
type PolicyRequest struct {
	RequestID   string `json:"request_id"`
	Action      string `json:"action"`       // "create","update","delete","rollback"
	RiskLevel   string `json:"risk_level"`   // "low","medium","high","critical"
	Provider    string `json:"provider"`
	Environment string `json:"environment"`
}

// ─── Audit chain ───────────────────────────────────────────────────────────

// AuditAction classifies what was recorded in the audit chain.
type AuditAction string

const (
	AuditActionApproved         AuditAction = "approved"
	AuditActionDenied           AuditAction = "denied"
	AuditActionOverridden       AuditAction = "overridden"
	AuditActionPolicyEvaluated  AuditAction = "policy-evaluated"
	AuditActionRoleAssigned     AuditAction = "role-assigned"
	AuditActionRoleRevoked      AuditAction = "role-revoked"
)

// GovernanceAuditEntry is one immutable record in the audit chain.
// PrevHash links it to the previous entry, forming a tamper-evident chain.
type GovernanceAuditEntry struct {
	EntryID     string      `json:"entry_id"`
	ExecutionID string      `json:"execution_id"`
	ActorID     string      `json:"actor_id"`
	Action      AuditAction `json:"action"`
	PolicyID    string      `json:"policy_id,omitempty"`
	RoleID      string      `json:"role_id,omitempty"`
	Reason      string      `json:"reason"`
	Timestamp   string      `json:"timestamp"`
	// PrevHash is the EntryHash of the previous entry (empty for the first entry).
	PrevHash  string `json:"prev_hash"`
	// EntryHash is SHA-256(EntryID + ActorID + Action + Reason + Timestamp + PrevHash).
	EntryHash string `json:"entry_hash"`
}

// GovernanceAuditChain is the append-only, tamper-evident audit log for an execution.
type GovernanceAuditChain struct {
	ExecutionID string                 `json:"execution_id"`
	Entries     []GovernanceAuditEntry `json:"entries"`
	// ChainHash is SHA-256 over all entry hashes in order.
	ChainHash string `json:"chain_hash"`
	UpdatedAt string `json:"updated_at"`
}

// ─── Approvals ─────────────────────────────────────────────────────────────

// ApprovalStep is one level in a multi-stage approval hierarchy.
type ApprovalStep struct {
	StepID       string `json:"step_id"`
	StepOrder    int    `json:"step_order"`
	RequiredRole string `json:"required_role"`
	// Reason explains why this approval level is required.
	Reason string `json:"reason"`
	// Satisfied is set when an actor with the required role has approved.
	Satisfied  bool   `json:"satisfied"`
	SatisfiedBy string `json:"satisfied_by,omitempty"`
	SatisfiedAt string `json:"satisfied_at,omitempty"`
}

// ApprovalHierarchy is the ordered set of approvals required for an action.
type ApprovalHierarchy struct {
	HierarchyID string         `json:"hierarchy_id"`
	ExecutionID string         `json:"execution_id"`
	StageID     string         `json:"stage_id"`
	Steps       []ApprovalStep `json:"steps"`
	Rationale   string         `json:"rationale"`
	// AllSatisfied is true when every Step is satisfied.
	AllSatisfied bool `json:"all_satisfied"`
}

// ─── Overrides ─────────────────────────────────────────────────────────────

// GovernanceOverride records an explicit policy override with mandatory justification.
// Overrides are persisted forever and can never be deleted.
type GovernanceOverride struct {
	OverrideID  string   `json:"override_id"`
	ExecutionID string   `json:"execution_id"`
	ActorID     string   `json:"actor_id"`
	Reason      string   `json:"reason"`
	Evidence    []string `json:"evidence"`
	PolicyID    string   `json:"policy_id"`
	OverriddenAt string  `json:"overridden_at"`
}

// ─── Snapshot ──────────────────────────────────────────────────────────────

// GovernanceSnapshot is a tamper-evident export of governance state.
type GovernanceSnapshot struct {
	SchemaVersion  string                 `json:"schema_version"`
	ExecutionID    string                 `json:"execution_id"`
	Roles          []Role                 `json:"roles"`
	Policies       []Policy               `json:"policies"`
	Decisions      []PolicyDecision       `json:"decisions"`
	AuditChain     GovernanceAuditChain   `json:"audit_chain"`
	Overrides      []GovernanceOverride   `json:"overrides"`
	Hierarchies    []ApprovalHierarchy    `json:"hierarchies"`
	Limitations    []string               `json:"limitations"`
	GeneratedAt    string                 `json:"generated_at"`
	GovernanceHash string                 `json:"governance_hash"`
}
