package governance

import (
	"strings"
	"testing"
)

// ─── RBAC ──────────────────────────────────────────────────────────────────

func TestHasPermission_Found(t *testing.T) {
	role := Role{
		RoleID:      "test-role",
		Permissions: []Permission{PermissionViewExecution, PermissionApproveExecution},
	}
	if !HasPermission(role, PermissionApproveExecution) {
		t.Fatal("role has PermissionApproveExecution — HasPermission must return true")
	}
}

func TestHasPermission_NotFound(t *testing.T) {
	role := Role{
		RoleID:      "test-role",
		Permissions: []Permission{PermissionViewExecution},
	}
	if HasPermission(role, PermissionOverride) {
		t.Fatal("role does not have PermissionOverride — HasPermission must return false")
	}
}

func TestEvaluateRBAC_EmptyActorID(t *testing.T) {
	decision := EvaluateRBAC("", PermissionApproveExecution, nil, nil)
	if decision.Granted {
		t.Fatal("empty actorID must be denied")
	}
	if decision.Reason == "" {
		t.Fatal("reason must be non-empty for denied decision")
	}
}

func TestEvaluateRBAC_Granted(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"alice": {"operator"}}
	decision := EvaluateRBAC("alice", PermissionApproveExecution, actorRoles, roles)
	if !decision.Granted {
		t.Fatalf("operator role must grant PermissionApproveExecution: %s", decision.Reason)
	}
	if decision.GrantedBy != "operator" {
		t.Fatalf("GrantedBy must be 'operator', got %q", decision.GrantedBy)
	}
}

func TestEvaluateRBAC_Denied_NoMatchingRole(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"bob": {"auditor"}}
	decision := EvaluateRBAC("bob", PermissionOverride, actorRoles, roles)
	if decision.Granted {
		t.Fatal("auditor must not have PermissionOverride")
	}
}

func TestEvaluateRBAC_AdminHasOverride(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"admin-user": {"admin"}}
	decision := EvaluateRBAC("admin-user", PermissionOverride, actorRoles, roles)
	if !decision.Granted {
		t.Fatalf("admin must have PermissionOverride: %s", decision.Reason)
	}
}

func TestEvaluateRBAC_Deterministic(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"alice": {"operator", "auditor"}}
	d1 := EvaluateRBAC("alice", PermissionApproveExecution, actorRoles, roles)
	d2 := EvaluateRBAC("alice", PermissionApproveExecution, actorRoles, roles)
	if d1.Granted != d2.Granted || d1.GrantedBy != d2.GrantedBy {
		t.Fatal("EvaluateRBAC must be deterministic")
	}
}

func TestEvaluateRBAC_UnknownRole(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"eve": {"ghost-role"}}
	decision := EvaluateRBAC("eve", PermissionViewExecution, actorRoles, roles)
	if decision.Granted {
		t.Fatal("unknown role must not grant any permission")
	}
}

// ─── Policies ──────────────────────────────────────────────────────────────

func TestEvaluatePolicy_DenyDestructiveDeleteProd(t *testing.T) {
	policies := DefaultPolicies()
	req := PolicyRequest{
		RequestID:   "req-1",
		Action:      "delete",
		Environment: "production",
		RiskLevel:   "low",
	}
	dec := EvaluatePolicy(policies, req)
	if dec.Effect != PolicyEffectDeny {
		t.Fatalf("delete in production must be denied, got %q", dec.Effect)
	}
	if dec.PolicyID != "deny-destructive-delete-prod" {
		t.Fatalf("unexpected policy matched: %q", dec.PolicyID)
	}
}

func TestEvaluatePolicy_RequireApprovalHighRisk(t *testing.T) {
	policies := DefaultPolicies()
	req := PolicyRequest{
		RequestID:   "req-2",
		Action:      "update",
		Environment: "staging",
		RiskLevel:   "high",
	}
	dec := EvaluatePolicy(policies, req)
	if dec.Effect != PolicyEffectRequireApproval {
		t.Fatalf("high risk must require approval, got %q", dec.Effect)
	}
}

func TestEvaluatePolicy_RequireMultiApprovalCritical(t *testing.T) {
	policies := DefaultPolicies()
	req := PolicyRequest{
		RequestID:   "req-3",
		Action:      "update",
		Environment: "staging",
		RiskLevel:   "critical",
	}
	dec := EvaluatePolicy(policies, req)
	if dec.Effect != PolicyEffectRequireMultiApproval {
		t.Fatalf("critical risk must require multi-approval, got %q", dec.Effect)
	}
}

func TestEvaluatePolicy_DefaultAllow(t *testing.T) {
	policies := DefaultPolicies()
	req := PolicyRequest{
		RequestID:   "req-4",
		Action:      "create",
		Environment: "dev",
		RiskLevel:   "low",
	}
	dec := EvaluatePolicy(policies, req)
	if dec.Effect != PolicyEffectAllow {
		t.Fatalf("low risk create in dev must default allow, got %q", dec.Effect)
	}
	if dec.PolicyID != "default-allow" {
		t.Fatalf("default-allow policy ID expected, got %q", dec.PolicyID)
	}
}

func TestEvaluatePolicy_Deterministic(t *testing.T) {
	policies := DefaultPolicies()
	req := PolicyRequest{RequestID: "r", Action: "delete", Environment: "production", RiskLevel: "low"}
	d1 := EvaluatePolicy(policies, req)
	d2 := EvaluatePolicy(policies, req)
	if d1.Effect != d2.Effect || d1.PolicyID != d2.PolicyID {
		t.Fatal("EvaluatePolicy must be deterministic")
	}
}

func TestEvaluatePolicy_HigherPriorityWins(t *testing.T) {
	// critical delete in production: deny-destructive-delete-prod (100) vs
	// require-multi-approval-critical (95) — deny wins.
	policies := DefaultPolicies()
	req := PolicyRequest{
		RequestID:   "req-5",
		Action:      "delete",
		Environment: "production",
		RiskLevel:   "critical",
	}
	dec := EvaluatePolicy(policies, req)
	if dec.Effect != PolicyEffectDeny {
		t.Fatalf("higher priority deny must win, got %q (policy %q)", dec.Effect, dec.PolicyID)
	}
}

func TestEvaluatePolicy_DecidedAtSet(t *testing.T) {
	dec := EvaluatePolicy(DefaultPolicies(), PolicyRequest{RequestID: "r", Action: "create", RiskLevel: "low", Environment: "dev"})
	if dec.DecidedAt == "" {
		t.Fatal("DecidedAt must be set")
	}
}

// ─── Audit chain ───────────────────────────────────────────────────────────

func TestNewAuditChain_Empty(t *testing.T) {
	chain := NewAuditChain("exec-1")
	if chain.ExecutionID != "exec-1" {
		t.Fatalf("ExecutionID mismatch")
	}
	if len(chain.Entries) != 0 {
		t.Fatal("new chain must have 0 entries")
	}
}

func TestAppendAuditEntry_FirstEntry_NoPrevHash(t *testing.T) {
	chain := NewAuditChain("exec-1")
	entry := NewAuditEntry("e1", "exec-1", "alice", AuditActionApproved, "policy-1", "operator", "approved by alice")
	chain = AppendAuditEntry(chain, entry)
	if len(chain.Entries) != 1 {
		t.Fatal("chain must have 1 entry")
	}
	if chain.Entries[0].PrevHash != "" {
		t.Fatal("first entry PrevHash must be empty")
	}
	if chain.Entries[0].EntryHash == "" {
		t.Fatal("EntryHash must be set")
	}
}

func TestAppendAuditEntry_SecondEntryLinked(t *testing.T) {
	chain := NewAuditChain("exec-1")
	e1 := NewAuditEntry("e1", "exec-1", "alice", AuditActionApproved, "p1", "operator", "r1")
	e2 := NewAuditEntry("e2", "exec-1", "bob", AuditActionDenied, "p1", "admin", "r2")
	chain = AppendAuditEntry(chain, e1)
	firstHash := chain.Entries[0].EntryHash
	chain = AppendAuditEntry(chain, e2)
	if chain.Entries[1].PrevHash != firstHash {
		t.Fatalf("second entry PrevHash must equal first EntryHash")
	}
}

func TestVerifyAuditChainIntegrity_Valid(t *testing.T) {
	chain := NewAuditChain("exec-1")
	chain = AppendAuditEntry(chain, NewAuditEntry("e1", "exec-1", "alice", AuditActionApproved, "p1", "operator", "ok"))
	chain = AppendAuditEntry(chain, NewAuditEntry("e2", "exec-1", "bob", AuditActionPolicyEvaluated, "p2", "", "policy checked"))
	ok, reason := VerifyAuditChainIntegrity(chain)
	if !ok {
		t.Fatalf("valid chain must verify: %s", reason)
	}
}

func TestVerifyAuditChainIntegrity_TamperedReason(t *testing.T) {
	chain := NewAuditChain("exec-1")
	chain = AppendAuditEntry(chain, NewAuditEntry("e1", "exec-1", "alice", AuditActionApproved, "p1", "operator", "ok"))
	chain.Entries[0].Reason = "tampered"
	ok, _ := VerifyAuditChainIntegrity(chain)
	if ok {
		t.Fatal("tampered reason must fail integrity check")
	}
}

func TestVerifyAuditChainIntegrity_TamperedPrevHash(t *testing.T) {
	chain := NewAuditChain("exec-1")
	chain = AppendAuditEntry(chain, NewAuditEntry("e1", "exec-1", "alice", AuditActionApproved, "p1", "operator", "ok"))
	chain = AppendAuditEntry(chain, NewAuditEntry("e2", "exec-1", "bob", AuditActionDenied, "p2", "", "denied"))
	chain.Entries[1].PrevHash = "badhash"
	ok, _ := VerifyAuditChainIntegrity(chain)
	if ok {
		t.Fatal("tampered PrevHash must fail integrity check")
	}
}

func TestVerifyAuditChainIntegrity_EmptyChain(t *testing.T) {
	chain := NewAuditChain("exec-1")
	ok, _ := VerifyAuditChainIntegrity(chain)
	if !ok {
		t.Fatal("empty chain must verify successfully")
	}
}

func TestAuditChainIntegrity_AppendOnlySemantics(t *testing.T) {
	chain1 := NewAuditChain("exec-1")
	chain2 := AppendAuditEntry(chain1, NewAuditEntry("e1", "exec-1", "alice", AuditActionApproved, "p1", "operator", "ok"))
	// Original chain1 must be unchanged.
	if len(chain1.Entries) != 0 {
		t.Fatal("AppendAuditEntry must not mutate original chain")
	}
	if len(chain2.Entries) != 1 {
		t.Fatal("returned chain must have 1 entry")
	}
}

// ─── Approval hierarchy ────────────────────────────────────────────────────

func TestBuildApprovalHierarchy_CriticalTwoSteps(t *testing.T) {
	hier := BuildApprovalHierarchy("h1", "exec-1", "stage-1", "critical", "update")
	if len(hier.Steps) != 2 {
		t.Fatalf("critical risk must produce 2 approval steps, got %d", len(hier.Steps))
	}
	if hier.AllSatisfied {
		t.Fatal("unsatisfied hierarchy must not be AllSatisfied")
	}
}

func TestBuildApprovalHierarchy_HighOneStep(t *testing.T) {
	hier := BuildApprovalHierarchy("h2", "exec-1", "stage-1", "high", "update")
	if len(hier.Steps) != 1 {
		t.Fatalf("high risk must produce 1 approval step, got %d", len(hier.Steps))
	}
}

func TestBuildApprovalHierarchy_LowNoSteps(t *testing.T) {
	hier := BuildApprovalHierarchy("h3", "exec-1", "stage-1", "low", "create")
	if len(hier.Steps) != 0 {
		t.Fatalf("low risk create must produce 0 steps, got %d", len(hier.Steps))
	}
	if !hier.AllSatisfied {
		t.Fatal("zero-step hierarchy must be AllSatisfied")
	}
}

func TestBuildApprovalHierarchy_DeleteAnyRiskRequiresApproval(t *testing.T) {
	hier := BuildApprovalHierarchy("h4", "exec-1", "stage-1", "low", "delete")
	if len(hier.Steps) != 1 {
		t.Fatalf("delete action must require 1 approval step regardless of risk, got %d", len(hier.Steps))
	}
}

func TestSatisfyApprovalStep_Success(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"alice": {"operator"}}
	hier := BuildApprovalHierarchy("h5", "exec-1", "stage-1", "high", "update")
	stepID := hier.Steps[0].StepID

	updated, err := SatisfyApprovalStep(hier, stepID, "alice", actorRoles, roles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated.Steps[0].Satisfied {
		t.Fatal("step must be satisfied after SatisfyApprovalStep")
	}
	if !updated.AllSatisfied {
		t.Fatal("single-step hierarchy must be AllSatisfied after one satisfaction")
	}
}

func TestSatisfyApprovalStep_WrongRole(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"bob": {"auditor"}}
	hier := BuildApprovalHierarchy("h6", "exec-1", "stage-1", "high", "update")
	stepID := hier.Steps[0].StepID

	_, err := SatisfyApprovalStep(hier, stepID, "bob", actorRoles, roles)
	if err == nil {
		t.Fatal("auditor satisfying an operator-required step must return error")
	}
}

func TestSatisfyApprovalStep_AlreadySatisfied(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"alice": {"operator"}}
	hier := BuildApprovalHierarchy("h7", "exec-1", "stage-1", "high", "update")
	stepID := hier.Steps[0].StepID

	hier, _ = SatisfyApprovalStep(hier, stepID, "alice", actorRoles, roles)
	_, err := SatisfyApprovalStep(hier, stepID, "alice", actorRoles, roles)
	if err == nil {
		t.Fatal("satisfying an already-satisfied step must return error")
	}
}

func TestSatisfyApprovalStep_EmptyActorID(t *testing.T) {
	roles := PredefinedRoles()
	hier := BuildApprovalHierarchy("h8", "exec-1", "stage-1", "high", "update")
	stepID := hier.Steps[0].StepID
	_, err := SatisfyApprovalStep(hier, stepID, "", nil, roles)
	if err == nil {
		t.Fatal("empty actorID must return error")
	}
}

func TestIsHierarchySatisfied_AllStepsSatisfied(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"alice": {"operator"}}
	hier := BuildApprovalHierarchy("h9", "exec-1", "stage-1", "high", "update")
	hier, _ = SatisfyApprovalStep(hier, hier.Steps[0].StepID, "alice", actorRoles, roles)
	if !IsHierarchySatisfied(hier) {
		t.Fatal("fully satisfied hierarchy must return true")
	}
}

func TestIsHierarchySatisfied_EmptyHierarchy(t *testing.T) {
	hier := BuildApprovalHierarchy("h10", "exec-1", "stage-1", "low", "create")
	if IsHierarchySatisfied(hier) {
		t.Fatal("zero-step hierarchy must NOT be considered satisfied by IsHierarchySatisfied (use AllSatisfied field for that)")
	}
}

func TestPendingSteps_ReturnsPending(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"alice": {"operator"}}
	hier := BuildApprovalHierarchy("hP", "exec-1", "stage-1", "critical", "update")
	// Satisfy step 1 only.
	hier, _ = SatisfyApprovalStep(hier, hier.Steps[0].StepID, "alice", actorRoles, roles)
	pending := PendingSteps(hier)
	if len(pending) != 1 {
		t.Fatalf("one pending step expected, got %d", len(pending))
	}
}

// ─── Overrides ─────────────────────────────────────────────────────────────

func TestRecordOverride_Success(t *testing.T) {
	ov, err := RecordOverride("ov-1", "exec-1", "alice", "emergency bypass", []string{"incident-123"}, "deny-destructive-delete-prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ov.ActorID != "alice" {
		t.Fatalf("ActorID mismatch")
	}
	if ov.OverriddenAt == "" {
		t.Fatal("OverriddenAt must be set")
	}
}

func TestRecordOverride_MissingActorID(t *testing.T) {
	_, err := RecordOverride("ov-1", "exec-1", "", "reason", []string{"ev"}, "p1")
	if err == nil {
		t.Fatal("missing actorID must return error")
	}
}

func TestRecordOverride_MissingReason(t *testing.T) {
	_, err := RecordOverride("ov-1", "exec-1", "alice", "", []string{"ev"}, "p1")
	if err == nil {
		t.Fatal("missing reason must return error")
	}
}

func TestRecordOverride_MissingEvidence(t *testing.T) {
	_, err := RecordOverride("ov-1", "exec-1", "alice", "reason", nil, "p1")
	if err == nil {
		t.Fatal("missing evidence must return error")
	}
}

func TestRecordOverride_EvidenceDeepCopied(t *testing.T) {
	ev := []string{"incident-1"}
	ov, _ := RecordOverride("ov-1", "exec-1", "alice", "reason", ev, "p1")
	ev[0] = "mutated"
	if ov.Evidence[0] == "mutated" {
		t.Fatal("override evidence must be deep-copied from input slice")
	}
}

// ─── Governance snapshot ───────────────────────────────────────────────────

func TestExportGovernanceSnapshot_NonNil(t *testing.T) {
	chain := NewAuditChain("exec-1")
	snap := ExportGovernanceSnapshot("exec-1", nil, nil, nil, chain, nil, nil, nil)
	if snap == nil {
		t.Fatal("ExportGovernanceSnapshot must return non-nil")
	}
}

func TestExportGovernanceSnapshot_HashIs64Chars(t *testing.T) {
	chain := NewAuditChain("exec-1")
	snap := ExportGovernanceSnapshot("exec-1", nil, nil, nil, chain, nil, nil, nil)
	if len(snap.GovernanceHash) != 64 {
		t.Fatalf("GovernanceHash must be 64 hex chars, got %d", len(snap.GovernanceHash))
	}
}

func TestExportGovernanceSnapshot_SchemaVersion(t *testing.T) {
	chain := NewAuditChain("exec-1")
	snap := ExportGovernanceSnapshot("exec-1", nil, nil, nil, chain, nil, nil, nil)
	if snap.SchemaVersion != GovernanceSnapshotSchemaVersion {
		t.Fatalf("SchemaVersion mismatch: %q", snap.SchemaVersion)
	}
}

func TestExportGovernanceSnapshot_GeneratedAtExcludedFromHash(t *testing.T) {
	chain := NewAuditChain("exec-1")
	roles := PredefinedRoles()
	snap1 := ExportGovernanceSnapshot("exec-1", roles, nil, nil, chain, nil, nil, nil)
	snap2 := ExportGovernanceSnapshot("exec-1", roles, nil, nil, chain, nil, nil, nil)
	if snap1.GovernanceHash != snap2.GovernanceHash {
		t.Fatal("same inputs must produce same GovernanceHash regardless of GeneratedAt")
	}
}

func TestVerifyGovernanceSnapshot_Valid(t *testing.T) {
	chain := NewAuditChain("exec-1")
	snap := ExportGovernanceSnapshot("exec-1", PredefinedRoles(), DefaultPolicies(), nil, chain, nil, nil, nil)
	ok, reason := VerifyGovernanceSnapshot(snap)
	if !ok {
		t.Fatalf("original snapshot must verify: %s", reason)
	}
}

func TestVerifyGovernanceSnapshot_TamperedExecutionID(t *testing.T) {
	chain := NewAuditChain("exec-1")
	snap := ExportGovernanceSnapshot("exec-1", nil, nil, nil, chain, nil, nil, nil)
	snap.ExecutionID = "tampered"
	ok, _ := VerifyGovernanceSnapshot(snap)
	if ok {
		t.Fatal("tampered ExecutionID must fail verification")
	}
}

func TestVerifyGovernanceSnapshot_Nil(t *testing.T) {
	ok, reason := VerifyGovernanceSnapshot(nil)
	if ok || reason == "" {
		t.Fatal("nil snapshot must return false with non-empty reason")
	}
}

func TestVerifyGovernanceSnapshot_EmptyHash(t *testing.T) {
	chain := NewAuditChain("exec-1")
	snap := ExportGovernanceSnapshot("exec-1", nil, nil, nil, chain, nil, nil, nil)
	snap.GovernanceHash = ""
	ok, reason := VerifyGovernanceSnapshot(snap)
	if ok || !strings.Contains(reason, "empty") {
		t.Fatal("empty hash must fail with 'empty' reason")
	}
}

func TestGovernanceSnapshot_WithAuditChainIntact(t *testing.T) {
	chain := NewAuditChain("exec-1")
	chain = AppendAuditEntry(chain, NewAuditEntry("e1", "exec-1", "alice", AuditActionApproved, "p1", "operator", "approved"))
	snap := ExportGovernanceSnapshot("exec-1", nil, nil, nil, chain, nil, nil, nil)
	ok, reason := VerifyGovernanceSnapshot(snap)
	if !ok {
		t.Fatalf("snapshot with valid chain must verify: %s", reason)
	}
	chainOk, chainReason := VerifyAuditChainIntegrity(snap.AuditChain)
	if !chainOk {
		t.Fatalf("audit chain inside snapshot must be intact: %s", chainReason)
	}
}

func TestGovernanceSnapshot_Deterministic(t *testing.T) {
	chain := NewAuditChain("exec-1")
	chain = AppendAuditEntry(chain, NewAuditEntry("e1", "exec-1", "alice", AuditActionApproved, "p1", "operator", "ok"))
	roles := PredefinedRoles()
	policies := DefaultPolicies()
	snap1 := ExportGovernanceSnapshot("exec-1", roles, policies, nil, chain, nil, nil, nil)
	snap2 := ExportGovernanceSnapshot("exec-1", roles, policies, nil, chain, nil, nil, nil)
	if snap1.GovernanceHash != snap2.GovernanceHash {
		t.Fatal("same inputs must produce same GovernanceHash")
	}
}

func TestGovernanceSnapshot_WithOverrides(t *testing.T) {
	chain := NewAuditChain("exec-1")
	ov, _ := RecordOverride("ov-1", "exec-1", "alice", "emergency", []string{"inc-1"}, "deny-destructive-delete-prod")
	snap := ExportGovernanceSnapshot("exec-1", nil, nil, nil, chain, []GovernanceOverride{ov}, nil, nil)
	if len(snap.Overrides) != 1 {
		t.Fatalf("snapshot must carry 1 override, got %d", len(snap.Overrides))
	}
	ok, _ := VerifyGovernanceSnapshot(snap)
	if !ok {
		t.Fatal("snapshot with override must verify")
	}
}

func TestPredefinedRoles_Count(t *testing.T) {
	roles := PredefinedRoles()
	if len(roles) < 3 {
		t.Fatalf("expected at least 3 predefined roles, got %d", len(roles))
	}
}

func TestDefaultPolicies_Count(t *testing.T) {
	policies := DefaultPolicies()
	if len(policies) < 3 {
		t.Fatalf("expected at least 3 default policies, got %d", len(policies))
	}
}

func TestSatisfyApprovalStep_OriginalUnchanged(t *testing.T) {
	roles := PredefinedRoles()
	actorRoles := map[string][]string{"alice": {"operator"}}
	original := BuildApprovalHierarchy("hX", "exec-1", "stage-1", "high", "update")
	stepID := original.Steps[0].StepID
	_, _ = SatisfyApprovalStep(original, stepID, "alice", actorRoles, roles)
	if original.Steps[0].Satisfied {
		t.Fatal("SatisfyApprovalStep must not mutate the original hierarchy")
	}
}
