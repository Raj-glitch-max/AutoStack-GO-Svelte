package governance

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

// ExportGovernanceSnapshot produces a tamper-evident export of the full
// governance state for an execution.
//
// GovernanceHash is computed over all stable fields — GeneratedAt and
// GovernanceHash itself are excluded from the hash input so that two exports
// of the same state produce the same hash (content-identity).
func ExportGovernanceSnapshot(
	executionID string,
	roles []Role,
	policies []Policy,
	decisions []PolicyDecision,
	chain GovernanceAuditChain,
	overrides []GovernanceOverride,
	hierarchies []ApprovalHierarchy,
	limitations []string,
) *GovernanceSnapshot {
	// Deep copy slices.
	rolesCopy := append([]Role(nil), roles...)
	policiesCopy := append([]Policy(nil), policies...)
	decisionsCopy := append([]PolicyDecision(nil), decisions...)
	overridesCopy := append([]GovernanceOverride(nil), overrides...)
	hierarchiesCopy := append([]ApprovalHierarchy(nil), hierarchies...)
	limitationsCopy := append([]string(nil), limitations...)

	// Sort for determinism.
	sort.SliceStable(rolesCopy, func(i, j int) bool { return rolesCopy[i].RoleID < rolesCopy[j].RoleID })
	sort.SliceStable(policiesCopy, func(i, j int) bool { return policiesCopy[i].PolicyID < policiesCopy[j].PolicyID })
	sort.SliceStable(decisionsCopy, func(i, j int) bool { return decisionsCopy[i].RequestID < decisionsCopy[j].RequestID })
	sort.SliceStable(overridesCopy, func(i, j int) bool { return overridesCopy[i].OverrideID < overridesCopy[j].OverrideID })
	sort.SliceStable(hierarchiesCopy, func(i, j int) bool { return hierarchiesCopy[i].HierarchyID < hierarchiesCopy[j].HierarchyID })

	snap := &GovernanceSnapshot{
		SchemaVersion: GovernanceSnapshotSchemaVersion,
		ExecutionID:   executionID,
		Roles:         rolesCopy,
		Policies:      policiesCopy,
		Decisions:     decisionsCopy,
		AuditChain:    chain,
		Overrides:     overridesCopy,
		Hierarchies:   hierarchiesCopy,
		Limitations:   limitationsCopy,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	snap.GovernanceHash = computeGovernanceHash(snap)
	return snap
}

// VerifyGovernanceSnapshot recomputes the GovernanceHash and checks that it
// matches the stored hash.
// Returns (true, "") if the snapshot is intact, or (false, reason) if tampered.
func VerifyGovernanceSnapshot(snap *GovernanceSnapshot) (bool, string) {
	if snap == nil {
		return false, "snapshot is nil"
	}
	if snap.GovernanceHash == "" {
		return false, "GovernanceHash is empty"
	}
	expected := computeGovernanceHash(snap)
	if snap.GovernanceHash != expected {
		return false, fmt.Sprintf("GovernanceHash mismatch: expected %q got %q", expected, snap.GovernanceHash)
	}
	return true, ""
}

// computeGovernanceHash derives SHA-256 over all stable snapshot fields.
// GeneratedAt and GovernanceHash are excluded for content-identity semantics.
func computeGovernanceHash(snap *GovernanceSnapshot) string {
	h := sha256.New()

	// Schema + execution identity.
	fmt.Fprintf(h, "schema:%s|exec:%s|", snap.SchemaVersion, snap.ExecutionID)

	// Roles.
	for _, r := range snap.Roles {
		perms := ""
		for _, p := range r.Permissions {
			perms += string(p) + ","
		}
		fmt.Fprintf(h, "role:%s:%s|", r.RoleID, perms)
	}

	// Policies.
	for _, p := range snap.Policies {
		fmt.Fprintf(h, "policy:%s:%s:%d|", p.PolicyID, string(p.Effect), p.Priority)
	}

	// Decisions.
	for _, d := range snap.Decisions {
		fmt.Fprintf(h, "decision:%s:%s:%s|", d.RequestID, d.PolicyID, string(d.Effect))
	}

	// Audit chain.
	fmt.Fprintf(h, "chainhash:%s|", snap.AuditChain.ChainHash)
	fmt.Fprintf(h, "chainentries:%d|", len(snap.AuditChain.Entries))

	// Overrides.
	for _, o := range snap.Overrides {
		fmt.Fprintf(h, "override:%s:%s:%s|", o.OverrideID, o.ActorID, o.PolicyID)
	}

	// Hierarchies.
	for _, hr := range snap.Hierarchies {
		fmt.Fprintf(h, "hierarchy:%s:%v:%d|", hr.HierarchyID, hr.AllSatisfied, len(hr.Steps))
	}

	// Limitations.
	for _, lim := range snap.Limitations {
		fmt.Fprintf(h, "lim:%s|", lim)
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}
