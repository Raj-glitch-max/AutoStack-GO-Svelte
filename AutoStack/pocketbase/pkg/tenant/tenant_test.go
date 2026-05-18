package tenant

import (
	"testing"

	"github.com/janlauber/one-click/pkg/events"
)

// ─── types ─────────────────────────────────────────────────────────────────

func TestNewTenant_DefaultIsolation(t *testing.T) {
	ten := NewTenant("tenant-1", "Test Tenant")
	if !ten.IsolationScope.RestrictToOwnExecutions {
		t.Fatal("new tenant must default to RestrictToOwnExecutions=true")
	}
	if !ten.IsolationScope.RestrictGovernance {
		t.Fatal("new tenant must default to RestrictGovernance=true")
	}
	if !ten.IsolationScope.RestrictExports {
		t.Fatal("new tenant must default to RestrictExports=true")
	}
	if !ten.IsolationScope.RestrictTopology {
		t.Fatal("new tenant must default to RestrictTopology=true")
	}
}

func TestNewTenant_Fields(t *testing.T) {
	ten := NewTenant("t1", "My Tenant")
	if ten.TenantID != "t1" || ten.Name != "My Tenant" {
		t.Fatal("TenantID and Name must match inputs")
	}
	if ten.CreatedAt == "" {
		t.Fatal("CreatedAt must be set")
	}
}

func TestDefaultIsolationScope_AllRestricted(t *testing.T) {
	scope := DefaultIsolationScope()
	if !scope.RestrictToOwnExecutions || !scope.RestrictGovernance ||
		!scope.RestrictExports || !scope.RestrictTopology {
		t.Fatal("DefaultIsolationScope must set all flags to true")
	}
}

// ─── ValidateTenantAccess ──────────────────────────────────────────────────

func TestValidateTenantAccess_SameTenant(t *testing.T) {
	if !ValidateTenantAccess("tenant-1", "tenant-1") {
		t.Fatal("same tenant must be allowed")
	}
}

func TestValidateTenantAccess_CrossTenant_Denied(t *testing.T) {
	if ValidateTenantAccess("tenant-1", "tenant-2") {
		t.Fatal("cross-tenant access must be denied")
	}
}

func TestValidateTenantAccess_EmptyTenantID_Denied(t *testing.T) {
	if ValidateTenantAccess("", "tenant-1") {
		t.Fatal("empty tenantID must be denied")
	}
}

// ─── FilterEventsByTenant ──────────────────────────────────────────────────

func makeEvent(t *testing.T, id, tenantID string, clock int64) events.PlatformEvent {
	t.Helper()
	return events.NewPlatformEvent(
		id, events.EventTypeExecutionStarted, events.AggregateTypeExecution,
		"agg-1", "exec-1", tenantID, clock, "", "", nil, nil,
	)
}

func TestFilterEventsByTenant_Correct(t *testing.T) {
	e1 := makeEvent(t, "e1", "tenant-1", 1)
	e2 := makeEvent(t, "e2", "tenant-2", 2)
	e3 := makeEvent(t, "e3", "tenant-1", 3)
	result := FilterEventsByTenant([]events.PlatformEvent{e1, e2, e3}, "tenant-1")
	if len(result) != 2 {
		t.Fatalf("must return 2 events for tenant-1, got %d", len(result))
	}
	for _, e := range result {
		if e.TenantID != "tenant-1" {
			t.Fatalf("returned event belongs to wrong tenant: %q", e.TenantID)
		}
	}
}

func TestFilterEventsByTenant_NoMatches(t *testing.T) {
	e1 := makeEvent(t, "e1", "tenant-2", 1)
	result := FilterEventsByTenant([]events.PlatformEvent{e1}, "tenant-1")
	if len(result) != 0 {
		t.Fatal("no events for tenant-1 must return empty slice")
	}
}

// ─── AssertTenantOwnership ─────────────────────────────────────────────────

func TestAssertTenantOwnership_SameTenant(t *testing.T) {
	e := makeEvent(t, "e1", "tenant-1", 1)
	if err := AssertTenantOwnership(e, "tenant-1"); err != nil {
		t.Fatalf("ownership assertion must succeed for same tenant: %v", err)
	}
}

func TestAssertTenantOwnership_CrossTenant(t *testing.T) {
	e := makeEvent(t, "e1", "tenant-2", 1)
	if err := AssertTenantOwnership(e, "tenant-1"); err == nil {
		t.Fatal("ownership assertion must fail for wrong tenant")
	}
}

// ─── ScopedKey ─────────────────────────────────────────────────────────────

func TestScopedKey_Format(t *testing.T) {
	key := ScopedKey("tenant-1", "exec-42")
	if key != "tenant-1:exec-42" {
		t.Fatalf("ScopedKey must be 'tenantID:resourceID', got %q", key)
	}
}

// ─── ScopedReplay / ScopedGovernance / ScopedTopology ─────────────────────

func TestScopedReplay_FiltersToTenant(t *testing.T) {
	e1 := makeEvent(t, "e1", "tenant-1", 1)
	e2 := makeEvent(t, "e2", "tenant-2", 2)
	result := ScopedReplay([]events.PlatformEvent{e1, e2}, "tenant-1")
	if len(result) != 1 || result[0].TenantID != "tenant-1" {
		t.Fatal("ScopedReplay must return only tenant-1 events")
	}
}

func TestScopedGovernance_FiltersToTenantAndAggType(t *testing.T) {
	e1 := events.NewPlatformEvent("e1", events.EventTypeGovernanceDecision,
		events.AggregateTypeGovernance, "agg", "exec", "tenant-1", 1, "", "", nil, nil)
	e2 := events.NewPlatformEvent("e2", events.EventTypeExecutionStarted,
		events.AggregateTypeExecution, "agg", "exec", "tenant-1", 2, "", "", nil, nil)
	result := ScopedGovernance([]events.PlatformEvent{e1, e2}, "tenant-1")
	if len(result) != 1 || string(result[0].AggregateType) != "governance" {
		t.Fatalf("ScopedGovernance must return only governance events, got %d", len(result))
	}
}

// ─── TenantsFromEvents ─────────────────────────────────────────────────────

func TestTenantsFromEvents_Deduplication(t *testing.T) {
	e1 := makeEvent(t, "e1", "tenant-a", 1)
	e2 := makeEvent(t, "e2", "tenant-b", 2)
	e3 := makeEvent(t, "e3", "tenant-a", 3)
	tenants := TenantsFromEvents([]events.PlatformEvent{e1, e2, e3})
	if len(tenants) != 2 {
		t.Fatalf("expected 2 unique tenants, got %d", len(tenants))
	}
}

func TestTenantsFromEvents_Sorted(t *testing.T) {
	e1 := makeEvent(t, "e1", "zzz", 1)
	e2 := makeEvent(t, "e2", "aaa", 2)
	tenants := TenantsFromEvents([]events.PlatformEvent{e1, e2})
	if tenants[0] != "aaa" || tenants[1] != "zzz" {
		t.Fatal("TenantsFromEvents must return tenants in sorted order")
	}
}

// ─── isolation invariant ───────────────────────────────────────────────────

func TestTenantIsolation_CrossTenantEventNeverLeaks(t *testing.T) {
	// Simulate two tenants with interleaved events.
	var allEvts []events.PlatformEvent
	for i := int64(1); i <= 5; i++ {
		allEvts = append(allEvts, makeEvent(t, string(rune('a'+i-1)), "tenant-a", i))
		allEvts = append(allEvts, makeEvent(t, string(rune('A'+i-1)), "tenant-b", i+100))
	}
	resultA := FilterEventsByTenant(allEvts, "tenant-a")
	resultB := FilterEventsByTenant(allEvts, "tenant-b")
	for _, e := range resultA {
		if e.TenantID != "tenant-a" {
			t.Fatal("tenant-a result must not contain tenant-b events")
		}
	}
	for _, e := range resultB {
		if e.TenantID != "tenant-b" {
			t.Fatal("tenant-b result must not contain tenant-a events")
		}
	}
	if len(resultA)+len(resultB) != len(allEvts) {
		t.Fatal("partition must be complete — every event must appear in exactly one tenant's view")
	}
}
