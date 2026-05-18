package workflow

import (
	"testing"
)

// ─── graph helpers ─────────────────────────────────────────────────────────

func simpleGraph(t *testing.T) WorkflowGraph {
	t.Helper()
	nodes := []WorkflowNode{
		{NodeID: "start", NodeType: NodeTypeAction, Label: "Start"},
		{NodeID: "end", NodeType: NodeTypeAction, Label: "End"},
	}
	edges := []WorkflowEdge{
		{EdgeID: "e1", FromNode: "start", ToNode: "end", EdgeType: EdgeTypeSuccess},
		{EdgeID: "e2", FromNode: "start", ToNode: "end", EdgeType: EdgeTypeFailure},
	}
	g, err := NewWorkflowGraph("graph-1", "tenant-1", "Simple Graph", "start", nodes, edges)
	if err != nil {
		t.Fatalf("NewWorkflowGraph: %v", err)
	}
	return g
}

func approvalGraph(t *testing.T) WorkflowGraph {
	t.Helper()
	nodes := []WorkflowNode{
		{NodeID: "start", NodeType: NodeTypeAction, Label: "Start"},
		{NodeID: "gate", NodeType: NodeTypeApproval, Label: "Approve?"},
		{NodeID: "finish", NodeType: NodeTypeAction, Label: "Finish"},
		{NodeID: "abort", NodeType: NodeTypeAction, Label: "Abort"},
	}
	edges := []WorkflowEdge{
		{EdgeID: "e1", FromNode: "start", ToNode: "gate", EdgeType: EdgeTypeSuccess},
		{EdgeID: "e2", FromNode: "gate", ToNode: "finish", EdgeType: EdgeTypeSuccess},
		{EdgeID: "e3", FromNode: "gate", ToNode: "abort", EdgeType: EdgeTypeApprovalDenied},
	}
	g, err := NewWorkflowGraph("graph-approval", "tenant-1", "Approval Graph", "start", nodes, edges)
	if err != nil {
		t.Fatalf("NewWorkflowGraph: %v", err)
	}
	return g
}

func rollbackGraph(t *testing.T) WorkflowGraph {
	t.Helper()
	nodes := []WorkflowNode{
		{NodeID: "start", NodeType: NodeTypeAction, Label: "Start"},
		{NodeID: "work", NodeType: NodeTypeAction, Label: "Work"},
		{NodeID: "rollback", NodeType: NodeTypeRollback, Label: "Rollback"},
	}
	edges := []WorkflowEdge{
		{EdgeID: "e1", FromNode: "start", ToNode: "work", EdgeType: EdgeTypeSuccess},
		{EdgeID: "e2", FromNode: "work", ToNode: "rollback", EdgeType: EdgeTypeRollbackTriggered},
	}
	g, err := NewWorkflowGraph("graph-rollback", "tenant-1", "Rollback Graph", "start", nodes, edges)
	if err != nil {
		t.Fatalf("NewWorkflowGraph: %v", err)
	}
	return g
}

// ─── NewWorkflowGraph ──────────────────────────────────────────────────────

func TestNewWorkflowGraph_Valid(t *testing.T) {
	g := simpleGraph(t)
	if g.GraphID != "graph-1" {
		t.Fatalf("GraphID mismatch: %s", g.GraphID)
	}
	if g.GraphHash == "" {
		t.Fatal("GraphHash must not be empty")
	}
}

func TestNewWorkflowGraph_NoNodes_Error(t *testing.T) {
	_, err := NewWorkflowGraph("g1", "t1", "empty", "n1", nil, nil)
	if err == nil {
		t.Fatal("empty node list must error")
	}
}

func TestNewWorkflowGraph_UnknownEntryNode_Error(t *testing.T) {
	nodes := []WorkflowNode{{NodeID: "n1", NodeType: NodeTypeAction}}
	_, err := NewWorkflowGraph("g1", "t1", "test", "n-does-not-exist", nodes, nil)
	if err == nil {
		t.Fatal("unknown entry node must error")
	}
}

func TestNewWorkflowGraph_EdgeWithUnknownNode_Error(t *testing.T) {
	nodes := []WorkflowNode{{NodeID: "n1", NodeType: NodeTypeAction}}
	edges := []WorkflowEdge{{EdgeID: "e1", FromNode: "n1", ToNode: "n-unknown", EdgeType: EdgeTypeSuccess}}
	_, err := NewWorkflowGraph("g1", "t1", "test", "n1", nodes, edges)
	if err == nil {
		t.Fatal("edge with unknown ToNode must error")
	}
}

func TestVerifyWorkflowGraph_Valid(t *testing.T) {
	g := simpleGraph(t)
	ok, reason := VerifyWorkflowGraph(g)
	if !ok {
		t.Fatalf("valid graph must verify: %s", reason)
	}
}

func TestVerifyWorkflowGraph_TamperedName(t *testing.T) {
	g := simpleGraph(t)
	g.Name = "tampered"
	ok, _ := VerifyWorkflowGraph(g)
	if ok {
		t.Fatal("tampered Name must fail verification")
	}
}

// ─── graph helpers ─────────────────────────────────────────────────────────

func TestFindNode_Found(t *testing.T) {
	g := simpleGraph(t)
	n, ok := FindNode(g, "start")
	if !ok {
		t.Fatal("start node must be found")
	}
	if n.NodeID != "start" {
		t.Fatalf("NodeID mismatch: %s", n.NodeID)
	}
}

func TestFindNode_NotFound(t *testing.T) {
	g := simpleGraph(t)
	_, ok := FindNode(g, "no-such-node")
	if ok {
		t.Fatal("missing node must return false")
	}
}

func TestNextNode_FollowsEdge(t *testing.T) {
	g := simpleGraph(t)
	next, ok := NextNode(g, "start", EdgeTypeSuccess)
	if !ok {
		t.Fatal("success edge from start must lead somewhere")
	}
	if next.NodeID != "end" {
		t.Fatalf("success edge must lead to end, got %s", next.NodeID)
	}
}

func TestReachableNodeIDs(t *testing.T) {
	g := approvalGraph(t)
	ids := ReachableNodeIDs(g)
	if len(ids) != 4 {
		t.Fatalf("expected 4 reachable nodes, got %d: %v", len(ids), ids)
	}
}

// ─── ExecuteWorkflowGraph ──────────────────────────────────────────────────

func TestExecuteWorkflowGraph_Initial(t *testing.T) {
	g := simpleGraph(t)
	exec, tr, err := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	if err != nil {
		t.Fatalf("ExecuteWorkflowGraph: %v", err)
	}
	if exec.ActiveNodeID != "start" {
		t.Fatalf("initial active node must be start, got %s", exec.ActiveNodeID)
	}
	if exec.State != ExecutionStateRunning {
		t.Fatalf("initial state must be running, got %s", exec.State)
	}
	if tr.ToNodeID != "start" {
		t.Fatalf("initial transition must point to start, got %s", tr.ToNodeID)
	}
	if ok, reason := VerifyStepTransition(tr); !ok {
		t.Fatalf("initial transition hash invalid: %s", reason)
	}
}

func TestExecuteWorkflowGraph_WrongTenant_Error(t *testing.T) {
	g := simpleGraph(t)
	_, _, err := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "other-tenant")
	if err == nil {
		t.Fatal("wrong tenant must error")
	}
}

func TestExecuteWorkflowGraph_ApprovalNode_BlockedState(t *testing.T) {
	nodes := []WorkflowNode{{NodeID: "gate", NodeType: NodeTypeApproval}}
	g, _ := NewWorkflowGraph("g1", "t1", "test", "gate", nodes, nil)
	exec, _, err := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "t1")
	if err != nil {
		t.Fatalf("ExecuteWorkflowGraph: %v", err)
	}
	if exec.State != ExecutionStateBlocked {
		t.Fatalf("approval entry node must produce blocked state, got %s", exec.State)
	}
}

// ─── AdvanceWorkflow ───────────────────────────────────────────────────────

func TestAdvanceWorkflow_SuccessEdge(t *testing.T) {
	g := simpleGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")

	newExec, tr, err := AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")
	if err != nil {
		t.Fatalf("AdvanceWorkflow: %v", err)
	}
	if newExec.ActiveNodeID != "end" {
		t.Fatalf("expected active node end, got %s", newExec.ActiveNodeID)
	}
	if newExec.State != ExecutionStateRunning {
		t.Fatalf("expected running state, got %s", newExec.State)
	}
	if ok, reason := VerifyStepTransition(tr); !ok {
		t.Fatalf("transition hash invalid: %s", reason)
	}
}

func TestAdvanceWorkflow_TerminalNode_Completed(t *testing.T) {
	g := simpleGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")

	// end node has no outgoing edges → terminal
	finalExec, _, err := AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "end", Result: EdgeTypeSuccess}, "tr-2")
	if err != nil {
		t.Fatalf("AdvanceWorkflow terminal: %v", err)
	}
	if finalExec.State != ExecutionStateCompleted {
		t.Fatalf("expected completed state, got %s", finalExec.State)
	}
	if finalExec.ActiveNodeID != "" {
		t.Fatalf("terminal execution must have empty ActiveNodeID")
	}
}

func TestAdvanceWorkflow_FailureEdge(t *testing.T) {
	g := simpleGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	newExec, _, err := AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeFailure}, "tr-1")
	if err != nil {
		t.Fatalf("AdvanceWorkflow failure: %v", err)
	}
	if newExec.NodeStates["start"] != NodeStateFailed {
		t.Fatalf("start node must be failed, got %s", newExec.NodeStates["start"])
	}
}

func TestAdvanceWorkflow_CannotAdvanceTerminal(t *testing.T) {
	g := simpleGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")
	exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "end", Result: EdgeTypeSuccess}, "tr-2")

	_, _, err := AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "end", Result: EdgeTypeSuccess}, "tr-3")
	if err == nil {
		t.Fatal("advancing terminal execution must error")
	}
}

func TestAdvanceWorkflow_WrongActiveNode_Error(t *testing.T) {
	g := simpleGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	_, _, err := AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "wrong-node", Result: EdgeTypeSuccess}, "tr-1")
	if err == nil {
		t.Fatal("wrong active node must error")
	}
}

func TestAdvanceWorkflow_OriginalUnchanged(t *testing.T) {
	g := simpleGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	original := exec

	_, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")

	if exec.ActiveNodeID != original.ActiveNodeID {
		t.Fatal("AdvanceWorkflow must not mutate original execution")
	}
}

// ─── InitiateRollback ──────────────────────────────────────────────────────

func TestInitiateRollback_FollowsRollbackEdge(t *testing.T) {
	g := rollbackGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")

	// Now active = work. Initiate rollback.
	newExec, tr, err := InitiateRollback(g, exec, "manual rollback", "tr-rollback")
	if err != nil {
		t.Fatalf("InitiateRollback: %v", err)
	}
	if newExec.State != ExecutionStateRollingBack {
		t.Fatalf("expected rolling_back, got %s", newExec.State)
	}
	if tr.EdgeType != EdgeTypeRollbackTriggered {
		t.Fatalf("expected rollback_triggered edge, got %s", tr.EdgeType)
	}
	// Failed state of work node is preserved.
	if newExec.NodeStates["work"] != NodeStateFailed {
		t.Fatalf("work node must be failed, got %s", newExec.NodeStates["work"])
	}
}

func TestInitiateRollback_PreservesFailedHistory(t *testing.T) {
	g := rollbackGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")
	newExec, _, _ := InitiateRollback(g, exec, "reason", "tr-rb")
	// start was completed before rollback.
	if newExec.NodeStates["start"] != NodeStateCompleted {
		t.Fatalf("start node state must be preserved: got %s", newExec.NodeStates["start"])
	}
}

// ─── StepTransition ────────────────────────────────────────────────────────

func TestNewStepTransition_HashValid(t *testing.T) {
	tr := NewStepTransition("tr-1", "exec-1", "t1", "node-a", "node-b", EdgeTypeSuccess, 1, "")
	ok, reason := VerifyStepTransition(tr)
	if !ok {
		t.Fatalf("valid transition must verify: %s", reason)
	}
}

func TestVerifyStepTransition_TamperedFrom(t *testing.T) {
	tr := NewStepTransition("tr-1", "exec-1", "t1", "node-a", "node-b", EdgeTypeSuccess, 1, "")
	tr.FromNodeID = "tampered"
	ok, _ := VerifyStepTransition(tr)
	if ok {
		t.Fatal("tampered FromNodeID must fail verification")
	}
}

// ─── PlanWorkflowExecution ─────────────────────────────────────────────────

func TestPlanWorkflowExecution_IdentifiesApprovalGates(t *testing.T) {
	g := approvalGraph(t)
	plan, err := PlanWorkflowExecution(g, "plan-1")
	if err != nil {
		t.Fatalf("PlanWorkflowExecution: %v", err)
	}
	if !plan.RequiresApproval {
		t.Fatal("plan must identify approval requirement")
	}
	if len(plan.ApprovalGates) != 1 || plan.ApprovalGates[0] != "gate" {
		t.Fatalf("expected approval gate [gate], got %v", plan.ApprovalGates)
	}
}

func TestPlanWorkflowExecution_IdentifiesRollback(t *testing.T) {
	g := rollbackGraph(t)
	plan, err := PlanWorkflowExecution(g, "plan-1")
	if err != nil {
		t.Fatalf("PlanWorkflowExecution: %v", err)
	}
	if !plan.HasRollback {
		t.Fatal("plan must identify rollback nodes")
	}
}

func TestPlanWorkflowExecution_HashValid(t *testing.T) {
	g := simpleGraph(t)
	plan, _ := PlanWorkflowExecution(g, "plan-1")
	ok, reason := VerifyExecutionPlan(plan)
	if !ok {
		t.Fatalf("plan hash invalid: %s", reason)
	}
}

// ─── ScheduleNext ──────────────────────────────────────────────────────────

func TestScheduleNext_ReturnsActiveNode(t *testing.T) {
	g := simpleGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	nodes, err := ScheduleNext(g, exec)
	if err != nil {
		t.Fatalf("ScheduleNext: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Node.NodeID != "start" {
		t.Fatalf("expected [start], got %v", nodes)
	}
}

func TestScheduleNext_ApprovalIsBlocked(t *testing.T) {
	g := approvalGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")

	nodes, err := ScheduleNext(g, exec)
	if err != nil {
		t.Fatalf("ScheduleNext: %v", err)
	}
	if len(nodes) != 1 || !nodes[0].IsBlocked {
		t.Fatal("approval node must be reported as blocked")
	}
}

func TestScheduleNext_TerminalExecution_Empty(t *testing.T) {
	g := simpleGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")
	exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "end", Result: EdgeTypeSuccess}, "tr-2")

	nodes, err := ScheduleNext(g, exec)
	if err != nil {
		t.Fatalf("ScheduleNext: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("terminal execution must return empty schedule, got %v", nodes)
	}
}

// ─── Constraints ───────────────────────────────────────────────────────────

func TestEvaluateConstraint_Eq(t *testing.T) {
	c := Constraint{Field: "env", Operator: "eq", Value: "prod"}
	if !EvaluateConstraint(c, map[string]string{"env": "prod"}) {
		t.Fatal("eq constraint must match")
	}
	if EvaluateConstraint(c, map[string]string{"env": "staging"}) {
		t.Fatal("eq constraint must not match staging")
	}
}

func TestEvaluateConstraint_Contains(t *testing.T) {
	c := Constraint{Field: "region", Operator: "contains", Value: "us-"}
	if !EvaluateConstraint(c, map[string]string{"region": "us-east-1"}) {
		t.Fatal("contains must match")
	}
}

func TestEvaluateConstraints_AllMustPass(t *testing.T) {
	cs := []Constraint{
		{Field: "a", Operator: "eq", Value: "1"},
		{Field: "b", Operator: "eq", Value: "2"},
	}
	ctx := map[string]string{"a": "1", "b": "2"}
	if !EvaluateConstraints(cs, ctx) {
		t.Fatal("all-matching constraints must return true")
	}
	ctx["b"] = "wrong"
	if EvaluateConstraints(cs, ctx) {
		t.Fatal("one failing constraint must return false")
	}
}

func TestEvaluateConstraints_Empty_ReturnsTrue(t *testing.T) {
	if !EvaluateConstraints(nil, nil) {
		t.Fatal("empty constraint list must return true")
	}
}

// ─── ReplayWorkflowExecution ───────────────────────────────────────────────

func TestReplayWorkflowExecution_CleanReplay(t *testing.T) {
	g := simpleGraph(t)
	exec, tr0, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	exec, tr1, _ := AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")
	exec, tr2, _ := AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "end", Result: EdgeTypeSuccess}, "tr-2")

	transitions := []StepTransition{tr0, tr1, tr2}
	report, err := ReplayWorkflowExecution(g, "exec-1", "tenant-1", transitions)
	if err != nil {
		t.Fatalf("ReplayWorkflowExecution: %v", err)
	}
	if len(report.Divergences) != 0 {
		t.Fatalf("clean replay must have no divergences: %v", report.Divergences)
	}
	if report.FinalState.State != exec.State {
		t.Fatalf("replay final state %q != actual %q", report.FinalState.State, exec.State)
	}
}

func TestReplayWorkflowExecution_AfterRollback(t *testing.T) {
	g := rollbackGraph(t)
	exec, tr0, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	exec, tr1, _ := AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")
	exec, tr2, _ := InitiateRollback(g, exec, "test rollback", "tr-rb")

	transitions := []StepTransition{tr0, tr1, tr2}
	report, err := ReplayWorkflowExecution(g, "exec-1", "tenant-1", transitions)
	if err != nil {
		t.Fatalf("ReplayWorkflowExecution after rollback: %v", err)
	}
	if len(report.Divergences) != 0 {
		t.Fatalf("rollback replay must have no divergences: %v", report.Divergences)
	}
	if report.FinalState.State != ExecutionStateRollingBack {
		t.Fatalf("expected rolling_back, got %s", report.FinalState.State)
	}
}

func TestReplayWorkflowExecution_TamperedTransition_Divergence(t *testing.T) {
	g := simpleGraph(t)
	exec, tr0, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	exec, tr1, _ := AdvanceWorkflow(g, exec, NodeOutcome{NodeID: "start", Result: EdgeTypeSuccess}, "tr-1")
	_ = exec

	tr1.Hash = "bad-hash"
	transitions := []StepTransition{tr0, tr1}
	report, err := ReplayWorkflowExecution(g, "exec-1", "tenant-1", transitions)
	if err != nil {
		t.Fatalf("ReplayWorkflowExecution: %v", err)
	}
	if len(report.Divergences) == 0 {
		t.Fatal("tampered transition must produce divergence")
	}
}

// ─── WorkflowCheckpoint + WorkflowSnapshot ────────────────────────────────

func TestBuildWorkflowCheckpoint_HashValid(t *testing.T) {
	g := simpleGraph(t)
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	cp := BuildWorkflowCheckpoint("cp-1", exec, 0)
	ok, reason := VerifyWorkflowCheckpoint(cp)
	if !ok {
		t.Fatalf("checkpoint hash invalid: %s", reason)
	}
}

func TestBuildWorkflowSnapshot_HashValid(t *testing.T) {
	g := simpleGraph(t)
	exec, tr0, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	snap := BuildWorkflowSnapshot("snap-1", g, exec, []StepTransition{tr0}, nil)
	ok, reason := VerifyWorkflowSnapshot(snap)
	if !ok {
		t.Fatalf("snapshot hash invalid: %s", reason)
	}
}

func TestBuildWorkflowSnapshot_GeneratedAtExcluded(t *testing.T) {
	g := simpleGraph(t)
	exec, tr0, _ := ExecuteWorkflowGraph(g, "exec-1", "tr-0", "tenant-1")
	snap := BuildWorkflowSnapshot("snap-1", g, exec, []StepTransition{tr0}, nil)
	origHash := snap.SnapshotHash
	snap.GeneratedAt = "2099-01-01T00:00:00Z"
	ok, _ := VerifyWorkflowSnapshot(snap)
	if !ok {
		t.Fatal("mutating GeneratedAt must not invalidate hash")
	}
	if snap.SnapshotHash != origHash {
		t.Fatal("hash must remain unchanged when GeneratedAt changes")
	}
}

// ─── traversal determinism ─────────────────────────────────────────────────

func TestWorkflow_TraversalDeterminism(t *testing.T) {
	g := simpleGraph(t)
	outcomes := []EdgeType{EdgeTypeSuccess, EdgeTypeSuccess}

	var states1, states2 []string
	exec, _, _ := ExecuteWorkflowGraph(g, "exec-A", "tr-0", "tenant-1")
	states1 = append(states1, string(exec.State)+"/"+exec.ActiveNodeID)
	for i, o := range outcomes {
		exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: exec.ActiveNodeID, Result: o}, "tr-A-"+string(rune('0'+i)))
		states1 = append(states1, string(exec.State)+"/"+exec.ActiveNodeID)
	}

	exec, _, _ = ExecuteWorkflowGraph(g, "exec-B", "tr-0", "tenant-1")
	states2 = append(states2, string(exec.State)+"/"+exec.ActiveNodeID)
	for i, o := range outcomes {
		exec, _, _ = AdvanceWorkflow(g, exec, NodeOutcome{NodeID: exec.ActiveNodeID, Result: o}, "tr-B-"+string(rune('0'+i)))
		states2 = append(states2, string(exec.State)+"/"+exec.ActiveNodeID)
	}

	if len(states1) != len(states2) {
		t.Fatalf("state sequences have different lengths: %v vs %v", states1, states2)
	}
	for i := range states1 {
		if states1[i] != states2[i] {
			t.Fatalf("traversal diverged at step %d: %q vs %q", i, states1[i], states2[i])
		}
	}
}
