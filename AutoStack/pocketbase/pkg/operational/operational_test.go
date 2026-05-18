package operational

import (
	"testing"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/observability"
	"github.com/janlauber/one-click/pkg/workers"
)

func makeTask(id, execID, tenantID string, state executor.TaskState) executor.ExecutionTask {
	t := executor.NewExecutionTask(id, execID, "wf-1", "step-1", tenantID, executor.TaskTypeDeploy, 1, 1)
	// override state directly for test setup
	switch state {
	case executor.TaskStateCompleted:
		t, _ = executor.TransitionTask(t, executor.TaskStateQueued)
		t, _ = executor.TransitionTask(t, executor.TaskStateDispatched)
		t, _ = executor.MarkTaskStarted(t, "worker-1")
		t, _ = executor.MarkTaskCompleted(t)
	case executor.TaskStateFailed:
		t, _ = executor.TransitionTask(t, executor.TaskStateQueued)
		t, _ = executor.TransitionTask(t, executor.TaskStateDispatched)
		t, _ = executor.MarkTaskStarted(t, "worker-1")
		t, _ = executor.MarkTaskFailed(t)
	}
	return t
}

func TestBuildOperationalSurface_TaskSummary(t *testing.T) {
	t1 := makeTask("t-1", "exec-1", "tenant-1", executor.TaskStatePending)
	t2 := makeTask("t-2", "exec-1", "tenant-1", executor.TaskStateCompleted)
	t3 := makeTask("t-3", "exec-2", "tenant-1", executor.TaskStatePending) // different execution

	surface := BuildOperationalSurface(
		"exec-1", "tenant-1",
		[]executor.ExecutionTask{t1, t2, t3},
		nil, nil, nil, nil, nil,
		observability.ObservabilityTimeline{},
	)

	if surface.TaskSummary.Total != 2 {
		t.Fatalf("expected 2 tasks for exec-1, got %d", surface.TaskSummary.Total)
	}
	if surface.TaskSummary.Terminal != 1 {
		t.Fatalf("expected 1 terminal task, got %d", surface.TaskSummary.Terminal)
	}
}

func TestBuildOperationalSurface_WorkerHealth(t *testing.T) {
	w1 := workers.NewWorker("w-1", "tenant-1", []string{"deploy"}, "us-east-1")
	w2 := workers.TransitionWorker(workers.NewWorker("w-2", "tenant-1", []string{"deploy"}, ""), workers.WorkerStateDegraded)

	surface := BuildOperationalSurface(
		"exec-1", "tenant-1",
		nil,
		[]workers.Worker{w1, w2},
		nil, nil, nil, nil,
		observability.ObservabilityTimeline{},
	)

	if surface.WorkerHealth.Total != 2 {
		t.Fatalf("expected 2 workers, got %d", surface.WorkerHealth.Total)
	}
	if surface.WorkerHealth.Online != 1 {
		t.Fatalf("expected 1 online worker, got %d", surface.WorkerHealth.Online)
	}
	if surface.WorkerHealth.Degraded != 1 {
		t.Fatalf("expected 1 degraded worker, got %d", surface.WorkerHealth.Degraded)
	}
}

func TestBuildOperationalSurface_NoContradictions(t *testing.T) {
	surface := BuildOperationalSurface(
		"exec-1", "tenant-1",
		nil, nil, nil, nil, nil, nil,
		observability.ObservabilityTimeline{},
	)
	if surface.ContradictionAlert.HasContradictions {
		t.Fatal("must not have contradictions with nil input")
	}
}

func TestBuildOperationalSurface_CertificationState(t *testing.T) {
	cert := audit.CertifyReplay("cert-1", "exec-1", "tenant-1", true, true, true, true, 10, 0)

	surface := BuildOperationalSurface(
		"exec-1", "tenant-1",
		nil, nil, nil, nil, nil,
		[]audit.ReplayCertificationReport{cert},
		observability.ObservabilityTimeline{},
	)

	if !surface.CertificationState.HasCertification {
		t.Fatal("must have certification")
	}
	if !surface.CertificationState.Certified {
		t.Fatal("must be certified")
	}
}

func TestBuildOperationalSurface_Timeline(t *testing.T) {
	entries := []observability.TimelineEntry{
		{Sequence: 1, LogicalClock: 1, Category: "task", EventRef: "t-1", Summary: "task started"},
		{Sequence: 2, LogicalClock: 2, Category: "task", EventRef: "t-1", Summary: "task completed"},
	}
	timeline := observability.StreamExecutionTimeline("exec-1", "tenant-1", entries)

	surface := BuildOperationalSurface(
		"exec-1", "tenant-1",
		nil, nil, nil, nil, nil, nil,
		timeline,
	)

	if len(surface.TimelineEntries) != 2 {
		t.Fatalf("expected 2 timeline entries, got %d", len(surface.TimelineEntries))
	}
}

func TestBuildOperationalSurface_WrongTenantWorkerExcluded(t *testing.T) {
	w := workers.NewWorker("w-1", "tenant-other", []string{"deploy"}, "")

	surface := BuildOperationalSurface(
		"exec-1", "tenant-1",
		nil,
		[]workers.Worker{w},
		nil, nil, nil, nil,
		observability.ObservabilityTimeline{},
	)

	if surface.WorkerHealth.Total != 0 {
		t.Fatal("worker from different tenant must be excluded")
	}
}

func TestBuildOperationalSurface_GeneratedAtSet(t *testing.T) {
	surface := BuildOperationalSurface(
		"exec-1", "tenant-1",
		nil, nil, nil, nil, nil, nil,
		observability.ObservabilityTimeline{},
	)
	if surface.GeneratedAt == "" {
		t.Fatal("GeneratedAt must be set")
	}
}
