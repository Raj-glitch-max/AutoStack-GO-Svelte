package executor

import (
	"context"
	"testing"

	"github.com/janlauber/one-click/pkg/orchestrator"
	"github.com/janlauber/one-click/pkg/planner"
)

func newRevalStage(stageID string, nodeIDs ...string) orchestrator.ExecutionStage {
	nodes := make([]orchestrator.StageNode, len(nodeIDs))
	for i, id := range nodeIDs {
		nodes[i] = orchestrator.StageNode{
			NodeID:   id,
			Provider: "aws-ecs",
			Action:   planner.ActionCreate,
		}
	}
	return orchestrator.ExecutionStage{StageID: stageID, StageOrder: 0, Nodes: nodes}
}

func TestRevalidate_SucceedingProvider_NoDrift(t *testing.T) {
	stage := newRevalStage("stage-0", "svc-a")
	result := RevalidateStage(context.Background(), "exec-1", stage, &SucceedingProviderExecutor{})
	if result.HasDrift {
		t.Fatal("succeeding provider must not produce drift")
	}
	if result.ShouldPause {
		t.Fatal("succeeding provider must not recommend pause")
	}
}

func TestRevalidate_FailingProvider_DriftDetected(t *testing.T) {
	stage := newRevalStage("stage-0", "svc-a")
	result := RevalidateStage(context.Background(), "exec-1", stage, &NoOpProviderExecutor{})
	if !result.HasDrift {
		t.Fatal("failing validation must detect drift")
	}
	if !result.ShouldPause {
		t.Fatal("drift must recommend pause")
	}
}

func TestRevalidate_EmptyStage_NoDrift(t *testing.T) {
	stage := orchestrator.ExecutionStage{StageID: "stage-0"}
	result := RevalidateStage(context.Background(), "exec-1", stage, &SucceedingProviderExecutor{})
	if result.HasDrift {
		t.Fatal("empty stage must not produce drift")
	}
}

func TestRevalidate_LimitationsNonEmpty(t *testing.T) {
	stage := newRevalStage("stage-0", "svc-a")
	result := RevalidateStage(context.Background(), "exec-1", stage, &SucceedingProviderExecutor{})
	if len(result.Limitations) == 0 {
		t.Fatal("RevalidationResult must document limitations")
	}
}

func TestRevalidate_DriftReport_PerNode(t *testing.T) {
	stage := newRevalStage("stage-0", "svc-a", "svc-b")
	result := RevalidateStage(context.Background(), "exec-1", stage, &SucceedingProviderExecutor{})
	if len(result.DriftReports) != 2 {
		t.Fatalf("expected one DriftReport per node, got %d", len(result.DriftReports))
	}
}

func TestRevalidate_ExecutionIDAndStageID_Set(t *testing.T) {
	stage := newRevalStage("stage-42", "svc-a")
	result := RevalidateStage(context.Background(), "exec-99", stage, &SucceedingProviderExecutor{})
	if result.ExecutionID != "exec-99" {
		t.Fatalf("ExecutionID mismatch: want exec-99, got %s", result.ExecutionID)
	}
	if result.StageID != "stage-42" {
		t.Fatalf("StageID mismatch: want stage-42, got %s", result.StageID)
	}
}

func TestRevalidate_GeneratedAtSet(t *testing.T) {
	stage := newRevalStage("stage-0", "svc-a")
	result := RevalidateStage(context.Background(), "exec-1", stage, &SucceedingProviderExecutor{})
	if result.GeneratedAt == "" {
		t.Fatal("RevalidationResult.GeneratedAt must be set")
	}
}

func TestRevalidate_ObservationsPresent_OnDrift(t *testing.T) {
	stage := newRevalStage("stage-0", "svc-a")
	result := RevalidateStage(context.Background(), "exec-1", stage, &NoOpProviderExecutor{})
	if len(result.DriftReports) == 0 {
		t.Fatal("expected drift reports")
	}
	if len(result.DriftReports[0].Observations) == 0 {
		t.Fatal("drift report must include observations explaining the drift")
	}
}
