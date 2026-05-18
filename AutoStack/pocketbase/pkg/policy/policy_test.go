package policy

import (
	"testing"
	"time"
)

// ─── Policy ────────────────────────────────────────────────────────────────

func TestNewPolicy_HashIsSet(t *testing.T) {
	p := NewPolicy("pol-1", "t1", "Test Policy", PolicyScopeTenant)
	if p.PolicyHash == "" {
		t.Fatal("PolicyHash must not be empty")
	}
}

func TestVerifyPolicyHash_Valid(t *testing.T) {
	p := NewPolicy("pol-1", "t1", "Test", PolicyScopeTenant)
	ok, reason := VerifyPolicyHash(p)
	if !ok {
		t.Fatalf("valid policy must verify: %s", reason)
	}
}

func TestVerifyPolicyHash_TamperedName(t *testing.T) {
	p := NewPolicy("pol-1", "t1", "Test", PolicyScopeTenant)
	p.Name = "tampered"
	ok, _ := VerifyPolicyHash(p)
	if ok {
		t.Fatal("tampered Name must fail verification")
	}
}

func TestActionAllowed(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant),
		[]string{"deploy", "rollback"}, nil, []string{"delete"})
	if !ActionAllowed(p, "deploy") {
		t.Fatal("deploy must be allowed")
	}
	if ActionAllowed(p, "delete") {
		t.Fatal("delete must not be allowed (forbidden)")
	}
	if ActionAllowed(p, "unknown") {
		t.Fatal("unknown action must not be allowed")
	}
}

func TestActionRequiresApproval(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant),
		[]string{"deploy"}, []string{"scale"}, nil)
	if !ActionRequiresApproval(p, "scale") {
		t.Fatal("scale must require approval")
	}
	if ActionRequiresApproval(p, "deploy") {
		t.Fatal("deploy must not require approval")
	}
}

func TestActionForbidden(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant),
		nil, nil, []string{"destroy"})
	if !ActionForbidden(p, "destroy") {
		t.Fatal("destroy must be forbidden")
	}
	if ActionForbidden(p, "deploy") {
		t.Fatal("deploy must not be forbidden")
	}
}

// ─── EvaluatePolicy ────────────────────────────────────────────────────────

func TestEvaluatePolicy_Allow(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant),
		[]string{"deploy"}, nil, nil)
	d := EvaluatePolicy(p, "deploy", 0.9, "d1")
	if d.Outcome != PolicyOutcomeAllow {
		t.Fatalf("expected allow, got %s", d.Outcome)
	}
}

func TestEvaluatePolicy_Deny_Forbidden(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant),
		nil, nil, []string{"destroy"})
	d := EvaluatePolicy(p, "destroy", 0.99, "d1")
	if d.Outcome != PolicyOutcomeDeny {
		t.Fatalf("expected deny for forbidden action, got %s", d.Outcome)
	}
}

func TestEvaluatePolicy_RequireApproval(t *testing.T) {
	p2 := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant),
		nil, []string{"scale"}, nil)
	d := EvaluatePolicy(p2, "scale", 0.9, "d1")
	if d.Outcome != PolicyOutcomeRequireApproval {
		t.Fatalf("expected require_approval, got %s", d.Outcome)
	}
	if !d.RequiresApproval {
		t.Fatal("RequiresApproval field must be true")
	}
}

func TestEvaluatePolicy_Escalate_LowConfidence(t *testing.T) {
	p := WithRiskThreshold(
		WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), []string{"deploy"}, nil, nil),
		0.7,
	)
	d := EvaluatePolicy(p, "deploy", 0.5, "d1") // confidence below threshold
	if d.Outcome != PolicyOutcomeEscalate {
		t.Fatalf("expected escalate, got %s", d.Outcome)
	}
	if d.EscalationReason == "" {
		t.Fatal("EscalationReason must be set")
	}
}

func TestEvaluatePolicy_DefaultDeny_UnknownAction(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant),
		[]string{"deploy"}, nil, nil)
	d := EvaluatePolicy(p, "unknown-action", 0.9, "d1")
	if d.Outcome != PolicyOutcomeDeny {
		t.Fatalf("unknown action must be denied by default, got %s", d.Outcome)
	}
}

func TestEvaluatePolicy_ForbiddenBeatsLowConfidence(t *testing.T) {
	// forbidden check happens before risk-threshold check
	p := WithRiskThreshold(
		WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), nil, nil, []string{"destroy"}),
		0.7,
	)
	d := EvaluatePolicy(p, "destroy", 0.1, "d1")
	if d.Outcome != PolicyOutcomeDeny {
		t.Fatalf("forbidden must deny before escalation check, got %s", d.Outcome)
	}
}

func TestVerifyDecisionHash_Valid(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), []string{"deploy"}, nil, nil)
	d := EvaluatePolicy(p, "deploy", 0.9, "d1")
	ok, reason := VerifyDecisionHash(d)
	if !ok {
		t.Fatalf("valid decision must verify: %s", reason)
	}
}

func TestVerifyDecisionHash_TamperedOutcome(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), []string{"deploy"}, nil, nil)
	d := EvaluatePolicy(p, "deploy", 0.9, "d1")
	d.Outcome = PolicyOutcomeDeny
	ok, _ := VerifyDecisionHash(d)
	if ok {
		t.Fatal("tampered Outcome must fail verification")
	}
}

// ─── ApprovalQueue ─────────────────────────────────────────────────────────

func TestApprovalQueue_RequestAndFind(t *testing.T) {
	q := NewApprovalQueue()
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	req := NewApprovalRequest("req-1", "exec-1", "t1", "deploy", "system", expires)
	if err := q.Request(req); err != nil {
		t.Fatalf("Request: %v", err)
	}
	found, ok := q.FindRequest("req-1")
	if !ok {
		t.Fatal("request must be found after submit")
	}
	if found.RequestID != "req-1" {
		t.Fatalf("RequestID mismatch: %s", found.RequestID)
	}
}

func TestApprovalQueue_RejectsDuplicateRequest(t *testing.T) {
	q := NewApprovalQueue()
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	req := NewApprovalRequest("req-1", "exec-1", "t1", "deploy", "system", expires)
	_ = q.Request(req)
	if err := q.Request(req); err == nil {
		t.Fatal("duplicate request must be rejected")
	}
}

func TestApprovalQueue_DecideGranted(t *testing.T) {
	q := NewApprovalQueue()
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	req := NewApprovalRequest("req-1", "exec-1", "t1", "deploy", "system", expires)
	_ = q.Request(req)

	dec := NewApprovalDecision("dec-1", "req-1", "operator-1", ApprovalStatusGranted, "looks good")
	if err := q.Decide(dec); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	found, ok := q.DecisionFor("req-1")
	if !ok {
		t.Fatal("decision must be found")
	}
	if found.Status != ApprovalStatusGranted {
		t.Fatalf("expected granted, got %s", found.Status)
	}
}

func TestApprovalQueue_DenialPersisted(t *testing.T) {
	q := NewApprovalQueue()
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	req := NewApprovalRequest("req-1", "exec-1", "t1", "scale", "system", expires)
	_ = q.Request(req)

	dec := NewApprovalDecision("dec-1", "req-1", "operator-1", ApprovalStatusDenied, "risk too high")
	_ = q.Decide(dec)

	found, ok := q.DecisionFor("req-1")
	if !ok {
		t.Fatal("denial must be persisted")
	}
	if found.Status != ApprovalStatusDenied {
		t.Fatalf("expected denied, got %s", found.Status)
	}
	if found.Reason == "" {
		t.Fatal("denial reason must be persisted")
	}
}

func TestApprovalQueue_CannotDecideTwice(t *testing.T) {
	q := NewApprovalQueue()
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	req := NewApprovalRequest("req-1", "exec-1", "t1", "deploy", "system", expires)
	_ = q.Request(req)
	_ = q.Decide(NewApprovalDecision("dec-1", "req-1", "op1", ApprovalStatusGranted, ""))
	if err := q.Decide(NewApprovalDecision("dec-2", "req-1", "op2", ApprovalStatusDenied, "")); err == nil {
		t.Fatal("second decision on same request must be rejected")
	}
}

func TestApprovalQueue_PendingRequests_ExcludesExpiredAndDecided(t *testing.T) {
	q := NewApprovalQueue()
	pastExpiry := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	futureExpiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)

	expired := NewApprovalRequest("req-expired", "exec-1", "t1", "deploy", "sys", pastExpiry)
	active := NewApprovalRequest("req-active", "exec-1", "t1", "deploy", "sys", futureExpiry)
	decided := NewApprovalRequest("req-decided", "exec-1", "t1", "deploy", "sys", futureExpiry)

	_ = q.Request(expired)
	_ = q.Request(active)
	_ = q.Request(decided)
	_ = q.Decide(NewApprovalDecision("d1", "req-decided", "op1", ApprovalStatusGranted, ""))

	now := time.Now().UTC().Format(time.RFC3339Nano)
	pending := q.PendingRequests(now)
	if len(pending) != 1 || pending[0].RequestID != "req-active" {
		t.Fatalf("expected [req-active] as pending, got %v", requestIDs(pending))
	}
}

func TestApprovalQueue_RetainsStateAcrossRebuild(t *testing.T) {
	// Simulate "restart during approval wait": rebuild queue from AllRequests.
	q := NewApprovalQueue()
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	req := NewApprovalRequest("req-1", "exec-1", "t1", "deploy", "sys", expires)
	_ = q.Request(req)

	allReqs := q.AllRequests()

	// Rebuild a new queue from the persisted requests.
	q2 := NewApprovalQueue()
	for _, r := range allReqs {
		_ = q2.Request(r)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	pending := q2.PendingRequests(now)
	if len(pending) != 1 {
		t.Fatalf("rebuilt queue must retain pending request, got %d pending", len(pending))
	}
}

// ─── EnforcePolicy ─────────────────────────────────────────────────────────

func TestEnforcePolicy_AllowedAction(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), []string{"deploy"}, nil, nil)
	result, _ := EnforcePolicy(p, "deploy", "d1", 0.9)
	if !result.Allowed {
		t.Fatal("allowed action must be permitted")
	}
	if !IsAutonomyPermitted(result) {
		t.Fatal("autonomy must be permitted for allowed actions")
	}
}

func TestEnforcePolicy_ForbiddenAction(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), nil, nil, []string{"destroy"})
	result, _ := EnforcePolicy(p, "destroy", "d1", 0.9)
	if !result.Denied {
		t.Fatal("forbidden action must be denied")
	}
	if IsAutonomyPermitted(result) {
		t.Fatal("autonomy must not be permitted for denied actions")
	}
}

func TestEnforcePolicy_RequiresApproval_BlocksAutonomy(t *testing.T) {
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), nil, []string{"scale"}, nil)
	result, _ := EnforcePolicy(p, "scale", "d1", 0.9)
	if !result.Blocked || !result.RequiresApproval {
		t.Fatal("approval-required action must be blocked")
	}
	if IsAutonomyPermitted(result) {
		t.Fatal("autonomy must not be permitted when approval required")
	}
}

func TestEnforcePolicy_Escalated_BlocksAutonomy(t *testing.T) {
	p := WithRiskThreshold(
		WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), []string{"deploy"}, nil, nil),
		0.8,
	)
	result, _ := EnforcePolicy(p, "deploy", "d1", 0.3)
	if !result.Escalated {
		t.Fatal("low confidence must escalate")
	}
	if IsAutonomyPermitted(result) {
		t.Fatal("autonomy must not be permitted when escalated")
	}
}

// ─── EscalationRecord ──────────────────────────────────────────────────────

func TestNewEscalationRecord_HashValid(t *testing.T) {
	rec := NewEscalationRecord("esc-1", "exec-1", "t1", EscalationTriggerTimeout, "approval expired")
	ok, reason := VerifyEscalationRecord(rec)
	if !ok {
		t.Fatalf("valid escalation hash invalid: %s", reason)
	}
}

func TestResolveEscalation_ValueSemantics(t *testing.T) {
	rec := NewEscalationRecord("esc-1", "exec-1", "t1", EscalationTriggerTimeout, "reason")
	resolved := ResolveEscalation(rec)

	if rec.Resolved {
		t.Fatal("original must remain unresolved")
	}
	if !resolved.Resolved {
		t.Fatal("resolved copy must be marked resolved")
	}
	if resolved.ResolvedAt == "" {
		t.Fatal("resolved copy must have ResolvedAt set")
	}
}

func TestResolveEscalation_NewHashValid(t *testing.T) {
	rec := NewEscalationRecord("esc-1", "exec-1", "t1", EscalationTriggerContradiction, "reason")
	resolved := ResolveEscalation(rec)
	ok, reason := VerifyEscalationRecord(resolved)
	if !ok {
		t.Fatalf("resolved escalation hash invalid: %s", reason)
	}
}

func TestApprovalTimeoutEscalation(t *testing.T) {
	expires := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	req := NewApprovalRequest("req-1", "exec-1", "t1", "deploy", "sys", expires)
	esc := ApprovalTimeoutEscalation("esc-1", req)
	if esc.Trigger != EscalationTriggerApprovalExpired {
		t.Fatalf("expected approval_expired trigger, got %s", esc.Trigger)
	}
	ok, reason := VerifyEscalationRecord(esc)
	if !ok {
		t.Fatalf("timeout escalation hash invalid: %s", reason)
	}
}

// ─── RuntimeConfidenceAssessment ───────────────────────────────────────────

func TestComputeConfidence_HighScore(t *testing.T) {
	a := ComputeConfidence("a1", "exec-1", "t1", 1.0, 1.0, 1.0, 0, 0, "granted")
	if a.CompositeScore != 1.0 {
		t.Fatalf("perfect inputs must produce 1.0 score, got %.4f", a.CompositeScore)
	}
	if a.Level != ConfidenceLevelHigh {
		t.Fatalf("expected high, got %s", a.Level)
	}
}

func TestComputeConfidence_ContradictionDegrades(t *testing.T) {
	a := ComputeConfidence("a1", "exec-1", "t1", 1.0, 1.0, 1.0, 3, 0, "pending")
	// base=1.0, penalty=3*0.1=0.3, score=0.7
	if a.CompositeScore > 0.75 {
		t.Fatalf("3 contradictions must degrade score, got %.4f", a.CompositeScore)
	}
	if a.Level != ConfidenceLevelMedium {
		t.Fatalf("expected medium, got %s", a.Level)
	}
}

func TestComputeConfidence_ReplayDivergenceDegrades(t *testing.T) {
	a := ComputeConfidence("a1", "exec-1", "t1", 1.0, 1.0, 1.0, 0, 5, "")
	// penalty = 5*0.15=0.75, score=0.25
	if a.CompositeScore > 0.30 {
		t.Fatalf("5 replay divergences must degrade significantly, got %.4f", a.CompositeScore)
	}
}

func TestComputeConfidence_CriticalLevel(t *testing.T) {
	a := ComputeConfidence("a1", "exec-1", "t1", 0.0, 0.0, 0.0, 10, 10, "denied")
	if a.Level != ConfidenceLevelCritical {
		t.Fatalf("expected critical, got %s", a.Level)
	}
	if !BelowOperatingThreshold(a) {
		t.Fatal("critical assessment must be below operating threshold")
	}
}

func TestComputeConfidence_ClampedAt1(t *testing.T) {
	a := ComputeConfidence("a1", "exec-1", "t1", 2.0, 2.0, 2.0, 0, 0, "")
	if a.CompositeScore > 1.0 {
		t.Fatalf("score must be clamped to 1.0, got %.4f", a.CompositeScore)
	}
}

func TestComputeConfidence_ClampedAt0(t *testing.T) {
	a := ComputeConfidence("a1", "exec-1", "t1", 0.0, 0.0, 0.0, 100, 100, "")
	if a.CompositeScore < 0.0 {
		t.Fatalf("score must not go below 0.0, got %.4f", a.CompositeScore)
	}
}

func TestConfidenceLevelFromScore_Boundaries(t *testing.T) {
	cases := []struct {
		score float64
		want  ConfidenceLevel
	}{
		{1.0, ConfidenceLevelHigh},
		{0.80, ConfidenceLevelHigh},
		{0.79, ConfidenceLevelMedium},
		{0.50, ConfidenceLevelMedium},
		{0.49, ConfidenceLevelLow},
		{0.20, ConfidenceLevelLow},
		{0.19, ConfidenceLevelCritical},
		{0.0, ConfidenceLevelCritical},
	}
	for _, c := range cases {
		got := ConfidenceLevelFromScore(c.score)
		if got != c.want {
			t.Errorf("score=%.2f: want %s, got %s", c.score, c.want, got)
		}
	}
}

func TestVerifyAssessmentHash_AssessedAtExcluded(t *testing.T) {
	a := ComputeConfidence("a1", "exec-1", "t1", 0.9, 0.8, 0.95, 0, 0, "pending")
	origHash := a.AssessmentHash
	a.AssessedAt = "2099-01-01T00:00:00Z"
	ok, _ := VerifyAssessmentHash(a)
	if !ok {
		t.Fatal("mutating AssessedAt must not invalidate hash")
	}
	if a.AssessmentHash != origHash {
		t.Fatal("hash must remain unchanged when AssessedAt changes")
	}
}

// ─── PolicyRuntime ─────────────────────────────────────────────────────────

func TestPolicyRuntime_AddPolicyAndEnforce(t *testing.T) {
	rt := NewPolicyRuntime("tenant-1")
	p := WithActions(NewPolicy("p1", "tenant-1", "Main", PolicyScopeTenant), []string{"deploy"}, nil, nil)
	if err := rt.AddPolicy(p); err != nil {
		t.Fatalf("AddPolicy: %v", err)
	}
	result, err := rt.Enforce("deploy", "d1", 0.9)
	if err != nil {
		t.Fatalf("Enforce: %v", err)
	}
	if !result.Allowed {
		t.Fatal("deploy must be allowed")
	}
}

func TestPolicyRuntime_AddPolicy_WrongTenant(t *testing.T) {
	rt := NewPolicyRuntime("tenant-1")
	p := NewPolicy("p1", "other-tenant", "Bad", PolicyScopeTenant)
	if err := rt.AddPolicy(p); err == nil {
		t.Fatal("wrong tenant policy must be rejected")
	}
}

func TestPolicyRuntime_AllDecisions_Recorded(t *testing.T) {
	rt := NewPolicyRuntime("t1")
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), []string{"deploy"}, nil, nil)
	_ = rt.AddPolicy(p)
	_, _ = rt.Enforce("deploy", "d1", 0.9)
	_, _ = rt.Enforce("deploy", "d2", 0.9)

	decisions := rt.AllDecisions()
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
}

func TestPolicyRuntime_Escalations(t *testing.T) {
	rt := NewPolicyRuntime("t1")
	rec := NewEscalationRecord("esc-1", "exec-1", "t1", EscalationTriggerTimeout, "reason")
	if err := rt.AddEscalation(rec); err != nil {
		t.Fatalf("AddEscalation: %v", err)
	}
	escs := rt.AllEscalations()
	if len(escs) != 1 {
		t.Fatalf("expected 1 escalation, got %d", len(escs))
	}
}

func TestPolicyRuntime_ApprovalFlow(t *testing.T) {
	rt := NewPolicyRuntime("t1")
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	req := NewApprovalRequest("req-1", "exec-1", "t1", "scale", "system", expires)
	if err := rt.RecordApprovalRequest(req); err != nil {
		t.Fatalf("RecordApprovalRequest: %v", err)
	}
	dec := NewApprovalDecision("dec-1", "req-1", "operator", ApprovalStatusGranted, "approved")
	if err := rt.RecordApprovalDecision(dec); err != nil {
		t.Fatalf("RecordApprovalDecision: %v", err)
	}
	d, ok := rt.ApprovalQueue().DecisionFor("req-1")
	if !ok {
		t.Fatal("decision must be found via approval queue")
	}
	if d.Status != ApprovalStatusGranted {
		t.Fatalf("expected granted, got %s", d.Status)
	}
}

func TestPolicyRuntime_ExportSnapshot_HashValid(t *testing.T) {
	rt := NewPolicyRuntime("t1")
	p := WithActions(NewPolicy("p1", "t1", "p", PolicyScopeTenant), []string{"deploy"}, nil, nil)
	_ = rt.AddPolicy(p)
	_, _ = rt.Enforce("deploy", "d1", 0.9)

	snap := rt.ExportSnapshot("snap-1")
	ok, reason := VerifyGovernanceSnapshot(snap)
	if !ok {
		t.Fatalf("snapshot hash invalid: %s", reason)
	}
}

func TestPolicyRuntime_ExportSnapshot_GeneratedAtExcluded(t *testing.T) {
	rt := NewPolicyRuntime("t1")
	snap := rt.ExportSnapshot("snap-1")
	origHash := snap.SnapshotHash
	snap.GeneratedAt = "2099-01-01T00:00:00Z"
	ok, _ := VerifyGovernanceSnapshot(snap)
	if !ok {
		t.Fatal("mutating GeneratedAt must not invalidate hash")
	}
	if snap.SnapshotHash != origHash {
		t.Fatal("hash must remain unchanged when GeneratedAt changes")
	}
}

// ─── autonomy restriction tests ────────────────────────────────────────────

func TestIsAutonomyPermitted_AllCases(t *testing.T) {
	tests := []struct {
		name    string
		result  EnforcementResult
		allowed bool
	}{
		{"allowed", EnforcementResult{Allowed: true}, true},
		{"denied", EnforcementResult{Denied: true}, false},
		{"escalated", EnforcementResult{Escalated: true}, false},
		{"requires_approval", EnforcementResult{Blocked: true, RequiresApproval: true}, false},
		{"allowed+requires_approval", EnforcementResult{Allowed: true, RequiresApproval: true}, false},
	}
	for _, tt := range tests {
		got := IsAutonomyPermitted(tt.result)
		if got != tt.allowed {
			t.Errorf("%s: IsAutonomyPermitted()=%v, want %v", tt.name, got, tt.allowed)
		}
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────

func requestIDs(reqs []ApprovalRequest) []string {
	ids := make([]string, len(reqs))
	for i, r := range reqs {
		ids[i] = r.RequestID
	}
	return ids
}
