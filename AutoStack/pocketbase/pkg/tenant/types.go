// Package tenant implements Phase 5.0 tenant isolation foundations.
//
// Scope:
//   - Tenant identity and isolation scope definition.
//   - Tenant-scoped event filtering (for replay, governance, exports, topology).
//   - Tenant access validation (deterministic, no side effects).
//
// NOT in scope for Phase 5.0:
//   - Multi-tenant scheduling.
//   - Tenant billing or quotas.
//   - Cross-tenant sharing.
package tenant

import "time"

// TenantID is a stable, opaque identifier for a tenant.
type TenantID string

// TenantIsolationScope defines what a tenant can access.
// All flags default to true (most restrictive) for new tenants.
type TenantIsolationScope struct {
	// RestrictToOwnExecutions: tenant can only see executions they own.
	RestrictToOwnExecutions bool `json:"restrict_to_own_executions"`
	// RestrictGovernance: governance audit chain is isolated per tenant.
	RestrictGovernance bool `json:"restrict_governance"`
	// RestrictExports: exports are scoped to tenant data only.
	RestrictExports bool `json:"restrict_exports"`
	// RestrictTopology: topology view is scoped to tenant executions only.
	RestrictTopology bool `json:"restrict_topology"`
}

// DefaultIsolationScope returns the most restrictive isolation scope.
// This is the safe default for any new tenant.
func DefaultIsolationScope() TenantIsolationScope {
	return TenantIsolationScope{
		RestrictToOwnExecutions: true,
		RestrictGovernance:      true,
		RestrictExports:         true,
		RestrictTopology:        true,
	}
}

// Tenant is the identity and isolation descriptor for one tenant.
type Tenant struct {
	TenantID       TenantID             `json:"tenant_id"`
	Name           string               `json:"name"`
	IsolationScope TenantIsolationScope `json:"isolation_scope"`
	CreatedAt      string               `json:"created_at"`
}

// NewTenant creates a Tenant with the default (most restrictive) isolation scope.
func NewTenant(id TenantID, name string) Tenant {
	return Tenant{
		TenantID:       id,
		Name:           name,
		IsolationScope: DefaultIsolationScope(),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// TenantFilter holds the filtering context for tenant-scoped operations.
type TenantFilter struct {
	TenantID TenantID `json:"tenant_id"`
}
