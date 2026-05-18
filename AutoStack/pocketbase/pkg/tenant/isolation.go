package tenant

import (
	"fmt"

	"github.com/janlauber/one-click/pkg/events"
)

// ValidateTenantAccess returns true when tenantID is allowed to access a
// resource owned by resourceTenantID.
// Cross-tenant access is ALWAYS denied — there are no exceptions in Phase 5.0.
func ValidateTenantAccess(tenantID TenantID, resourceTenantID string) bool {
	return string(tenantID) == resourceTenantID
}

// FilterEventsByTenant returns events belonging to the given tenant in canonical order.
// Events from other tenants are excluded entirely.
func FilterEventsByTenant(evts []events.PlatformEvent, tenantID TenantID) []events.PlatformEvent {
	return events.FilterByTenant(evts, string(tenantID))
}

// ScopedKey returns a canonical "{tenantID}:{resourceID}" key for cross-tenant
// disambiguation in shared maps or logs.
func ScopedKey(tenantID TenantID, resourceID string) string {
	return fmt.Sprintf("%s:%s", string(tenantID), resourceID)
}

// AssertTenantOwnership returns an error if the event does not belong to tenantID.
// Used at system boundaries to enforce tenant isolation.
func AssertTenantOwnership(event events.PlatformEvent, tenantID TenantID) error {
	if event.TenantID != string(tenantID) {
		return fmt.Errorf("tenant %q does not own event %q (owned by %q)",
			tenantID, event.EventID, event.TenantID)
	}
	return nil
}

// ScopedReplay filters an event slice to tenant scope before replay.
// Replay operations MUST call this before processing to prevent cross-tenant leakage.
func ScopedReplay(evts []events.PlatformEvent, tenantID TenantID) []events.PlatformEvent {
	return FilterEventsByTenant(evts, tenantID)
}

// ScopedGovernance filters governance-typed events to tenant scope.
func ScopedGovernance(evts []events.PlatformEvent, tenantID TenantID) []events.PlatformEvent {
	tenantEvts := FilterEventsByTenant(evts, tenantID)
	return events.FilterByAggregateType(tenantEvts, events.AggregateTypeGovernance)
}

// ScopedTopology filters topology-related events to tenant scope.
func ScopedTopology(evts []events.PlatformEvent, tenantID TenantID) []events.PlatformEvent {
	tenantEvts := FilterEventsByTenant(evts, tenantID)
	return events.FilterByAggregateType(tenantEvts, events.AggregateTypeExecution)
}

// TenantsFromEvents returns the deduplicated set of tenant IDs present in the event slice.
func TenantsFromEvents(evts []events.PlatformEvent) []string {
	seen := make(map[string]struct{})
	for _, e := range evts {
		seen[e.TenantID] = struct{}{}
	}
	tenants := make([]string, 0, len(seen))
	for t := range seen {
		tenants = append(tenants, t)
	}
	// Sort for determinism.
	sortStrings(tenants)
	return tenants
}

func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
