package platformapi

import (
	"testing"
	"time"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/governance"
	"github.com/janlauber/one-click/pkg/observability"
	"github.com/janlauber/one-click/pkg/providerops"
	"github.com/janlauber/one-click/pkg/replay"
	"github.com/janlauber/one-click/pkg/workflow"
	"github.com/janlauber/one-click/pkg/workers"
)

// ─── Executions ───────────────────────────────────────────────────────────────

func TestQueryExecutionTasks_Filter(t *testing.T) {
	t1 := executor.NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 1)
	t2 := executor.NewExecutionTask("t-2", "exec-1", "wf-1", "step-2", "tenant-1", executor.TaskTypeRollback, 1, 2)

	result := QueryExecutionTasks([]executor.ExecutionTask{t1, t2}, ExecutionQueryFilter{
		ExecutionID: "exec-1",
		TenantID:    "tenant-1",
		Type:        executor.TaskTypeDeploy,
	})
	if result.TotalCount != 1 {
		t.Fatalf("expected 1 task, got %d", result.TotalCount)
	}
	if result.Tasks[0].TaskID != "t-1" {
		t.Fatalf("expected t-1, got %s", result.Tasks[0].TaskID)
	}
}

func TestQueryExecutionTasks_HashValid(t *testing.T) {
	t1 := executor.NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 1)
	result := QueryExecutionTasks([]executor.ExecutionTask{t1}, ExecutionQueryFilter{
		ExecutionID: "exec-1",
		TenantID:    "tenant-1",
	})
	if !result.Tasks[0].HashValid {
		t.Fatal("task hash must be valid")
	}
}

func TestSummarizeExecution_Counts(t *testing.T) {
	t1 := executor.NewExecutionTask("t-1", "exec-1", "wf-1", "step-1", "tenant-1", executor.TaskTypeDeploy, 1, 1)
	t2 := executor.NewExecutionTask("t-2", "exec-1", "wf-1", "step-2", "tenant-1", executor.TaskTypeRollback, 1, 2)
	summary := SummarizeExecution([]executor.ExecutionTask{t1, t2}, "exec-1", "tenant-1")
	if summary.IntegrityIssues != 0 {
		t.Fatalf("expected 0 integrity issues, got %d", summary.IntegrityIssues)
	}
	if summary.StateCounts[string(executor.TaskStatePending)] != 2 {
		t.Fatalf("expected 2 pending, got %v", summary.StateCounts)
	}
}

// ─── Workflows ────────────────────────────────────────────────────────────────

func TestQueryWorkflowExecutions_Filter(t *testing.T) {
	g := workflow.WorkflowGraph{
		GraphID:   "g-1",
		TenantID:  "tenant-1",
		EntryNode: "n-1",
		Nodes: []workflow.WorkflowNode{
			{NodeID: "n-1", NodeType: workflow.NodeTypeAction},
		},
	}
	exec, _, err := workflow.ExecuteWorkflowGraph(g, "exec-1", "tr-1", "tenant-1")
	if err != nil {
		t.Fatalf("ExecuteWorkflowGraph: %v", err)
	}
	result := QueryWorkflowExecutions([]workflow.WorkflowExecution{exec}, "tenant-1", "exec-1")
	if result.TotalCount != 1 {
		t.Fatalf("expected 1 workflow, got %d", result.TotalCount)
	}
	if !result.Workflows[0].HashValid {
		t.Fatal("workflow hash must be valid")
	}
}

func TestSummarizeWorkflowProgress_NodeCounts(t *testing.T) {
	g := workflow.WorkflowGraph{
		GraphID:   "g-1",
		TenantID:  "tenant-1",
		EntryNode: "n-1",
		Nodes: []workflow.WorkflowNode{
			{NodeID: "n-1", NodeType: workflow.NodeTypeAction},
		},
	}
	exec, _, _ := workflow.ExecuteWorkflowGraph(g, "exec-1", "tr-1", "tenant-1")
	summary := SummarizeWorkflowProgress(exec)
	if summary.NodeCounts[string(workflow.NodeStateActive)] == 0 {
		t.Fatal("expected at least one active node")
	}
}

// ─── Governance ───────────────────────────────────────────────────────────────

func TestQueryGovernanceState_NilInputs(t *testing.T) {
	result := QueryGovernanceState(nil, nil)
	if result.View != nil {
		t.Fatal("nil snapshot must produce nil view")
	}
	if len(result.PendingSteps) != 0 {
		t.Fatal("nil hierarchy must produce no pending steps")
	}
}

func TestProjectPolicyDecision_Fields(t *testing.T) {
	d := governance.PolicyDecision{
		RequestID: "req-1",
		PolicyID:  "pol-1",
		Effect:    governance.PolicyEffectAllow,
		Reason:    "allowed by default",
		DecidedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	view := ProjectPolicyDecision(d)
	if view.Effect != "allow" {
		t.Fatalf("expected 'allow', got %s", view.Effect)
	}
	if view.RequestID != "req-1" {
		t.Fatalf("expected req-1, got %s", view.RequestID)
	}
}

// ─── Workers ──────────────────────────────────────────────────────────────────

func TestQueryWorkers_Filter(t *testing.T) {
	w1 := workers.NewWorker("w-1", "tenant-1", []string{"deploy"}, "us-east-1")
	w2 := workers.NewWorker("w-2", "tenant-1", []string{"rollback"}, "us-west-2")

	result := QueryWorkers([]workers.Worker{w1, w2}, WorkerQueryFilter{
		TenantID:   "tenant-1",
		Capability: "deploy",
	})
	if result.TotalCount != 1 {
		t.Fatalf("expected 1 worker, got %d", result.TotalCount)
	}
	if result.Workers[0].WorkerID != "w-1" {
		t.Fatalf("expected w-1, got %s", result.Workers[0].WorkerID)
	}
}

func TestSummarizeWorkerHealth_Counts(t *testing.T) {
	w1 := workers.NewWorker("w-1", "tenant-1", []string{"deploy"}, "")
	w2 := workers.NewWorker("w-2", "tenant-1", []string{"rollback"}, "")
	summary := SummarizeWorkerHealth([]workers.Worker{w1, w2}, "tenant-1")
	if summary.TotalWorkers != 2 {
		t.Fatalf("expected 2 workers, got %d", summary.TotalWorkers)
	}
	if summary.StateCounts[string(workers.WorkerStateOnline)] != 2 {
		t.Fatalf("expected 2 online workers, got %v", summary.StateCounts)
	}
}

// ─── Replay ───────────────────────────────────────────────────────────────────

func TestProjectReplayResult_Fields(t *testing.T) {
	r := replay.ReplayResult{
		ExecutionID: "exec-1",
		TenantID:    "tenant-1",
		EventCount:  5,
		FromClock:   1,
		ToClock:     5,
		Diverged:    false,
		ReplayedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	view := ProjectReplayResult(r)
	if view.EventCount != 5 {
		t.Fatalf("expected 5 events, got %d", view.EventCount)
	}
	if view.Diverged {
		t.Fatal("must not be diverged")
	}
}

func TestQueryReplayState_Filtering(t *testing.T) {
	r := replay.ReplayResult{
		ExecutionID: "exec-1",
		TenantID:    "tenant-1",
		EventCount:  3,
		ReplayedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	q := QueryReplayState([]replay.ReplayResult{r}, nil, "exec-1", "tenant-1")
	if len(q.Views) != 1 {
		t.Fatalf("expected 1 view, got %d", len(q.Views))
	}
}

// ─── Contradictions ───────────────────────────────────────────────────────────

func TestQueryContradictions_Basic(t *testing.T) {
	input := providerops.ContradictionInput{
		OperationID:    "op-1",
		OperationType:  providerops.OperationTypeDeploy,
		ReportedResult: providerops.ResultStateSuccess,
		ResourceExists: false, // deploy reported success but resource missing
	}
	c, err := providerops.DetectProviderContradiction("c-1", "exec-1", "tenant-1", "aws", input)
	if err != nil {
		t.Fatalf("DetectProviderContradiction: %v", err)
	}
	if c == nil {
		t.Fatal("expected contradiction to be detected")
	}
	summary := observability.SummarizeContradictions("exec-1", "tenant-1", []observability.ContradictionInput{
		{Type: string(c.Type), Resolved: false, ConfidenceImpact: 0.9},
	})
	result := QueryContradictions([]providerops.ProviderContradiction{*c}, &summary, "exec-1", "tenant-1")
	if result.TotalCount != 1 {
		t.Fatalf("expected 1 contradiction, got %d", result.TotalCount)
	}
}

// ─── Certifications ───────────────────────────────────────────────────────────

func TestQueryCertifications_Certified(t *testing.T) {
	r := audit.CertifyReplay("cert-1", "exec-1", "tenant-1", true, true, true, true, 10, 0)
	result := QueryCertifications([]audit.ReplayCertificationReport{r}, "exec-1", "tenant-1")
	if result.TotalCount != 1 {
		t.Fatalf("expected 1 cert, got %d", result.TotalCount)
	}
	if result.CertifiedCount != 1 {
		t.Fatal("cert must be certified")
	}
	if !result.Certifications[0].HashValid {
		t.Fatal("cert hash must be valid")
	}
}

func TestLatestCertification_ReturnsNewest(t *testing.T) {
	r1 := audit.CertifyReplay("cert-1", "exec-1", "tenant-1", true, true, true, true, 5, 0)
	r2 := audit.CertifyReplay("cert-2", "exec-1", "tenant-1", false, false, false, false, 5, 2)
	_, ok := LatestCertification([]audit.ReplayCertificationReport{r1, r2}, "exec-1", "tenant-1")
	if !ok {
		t.Fatal("must find latest certification")
	}
}

func TestLatestCertification_NoMatch(t *testing.T) {
	r := audit.CertifyReplay("cert-1", "exec-1", "tenant-1", true, true, true, true, 5, 0)
	_, ok := LatestCertification([]audit.ReplayCertificationReport{r}, "exec-2", "tenant-1")
	if ok {
		t.Fatal("must return not-found for wrong executionID")
	}
}
