// Package benchmarks measures the throughput and latency of the core
// AutoStack runtime operations. Run with:
//
//	go test ./pkg/benchmarks/... -bench=. -benchmem -count=3
//
// All benchmarks use deterministic, reproducible fixtures.
// Results are not guarantees — they are measurements at a point in time.
package benchmarks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/janlauber/one-click/pkg/archive"
	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/observability"
	"github.com/janlauber/one-click/pkg/persistence"
	"github.com/janlauber/one-click/pkg/providerops"
	"github.com/janlauber/one-click/pkg/replay"
	"github.com/janlauber/one-click/pkg/streaming"
	"github.com/janlauber/one-click/pkg/workers"
)

var bg = context.Background()

// ─── Persistence: SQLite WAL throughput ────────────────────────────────────

func BenchmarkSQLiteKVStore_Put(b *testing.B) {
	store, err := persistence.NewSQLiteKVStore(":memory:")
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench:key:%d", i)
		if err := store.Put(bg, key, []byte("benchmark-value")); err != nil {
			b.Fatalf("put: %v", err)
		}
	}
}

func BenchmarkSQLiteKVStore_Get(b *testing.B) {
	store, err := persistence.NewSQLiteKVStore(":memory:")
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	defer store.Close()

	for i := 0; i < 1000; i++ {
		_ = store.Put(bg, fmt.Sprintf("bench:key:%d", i), []byte("value"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench:key:%d", i%1000)
		_, _, _ = store.Get(bg, key)
	}
}

func BenchmarkSQLiteKVStore_List(b *testing.B) {
	store, err := persistence.NewSQLiteKVStore(":memory:")
	if err != nil {
		b.Fatalf("open: %v", err)
	}
	defer store.Close()

	for i := 0; i < 500; i++ {
		_ = store.Put(bg, fmt.Sprintf("task:exec-bench:%d", i), []byte("value"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.List(bg, "task:exec-bench:")
	}
}

// ─── Executor: task hash computation ────────────────────────────────────────

func BenchmarkNewExecutionTask(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = executor.NewExecutionTask(
			fmt.Sprintf("t-%d", i), "exec-bench", "wf-1", "step-1", "tenant-bench",
			executor.TaskTypeDeploy, 1, int64(i),
		)
	}
}

func BenchmarkVerifyTaskHash(b *testing.B) {
	task := executor.NewExecutionTask("t-bench", "exec-bench", "wf-1", "step-1", "tenant-bench",
		executor.TaskTypeDeploy, 1, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.VerifyTaskHash(task)
	}
}

// ─── Worker assignment: deterministic throughput ────────────────────────────

func BenchmarkWorkerAssignment(b *testing.B) {
	available := make([]workers.Worker, 10)
	for i := range available {
		available[i] = workers.NewWorker(fmt.Sprintf("w-%d", i), "tenant-bench", []string{"deploy"}, "us-east-1")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taskID := fmt.Sprintf("task-%d", i)
		_, _ = workers.AssignWorker(
			fmt.Sprintf("a-%d", i), taskID, "exec-bench", "tenant-bench",
			1, available, []string{"deploy"},
		)
	}
}

// ─── Streaming: event emit throughput ───────────────────────────────────────

func BenchmarkStreamEmit(b *testing.B) {
	stream := streaming.NewExecutionStream("stream-bench", "tenant-bench", "exec-bench", streaming.StreamTypeExecution)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ev := streaming.NewStreamEvent(
			fmt.Sprintf("ev-%d", i),
			"stream-bench",
			streaming.StreamTypeExecution,
			"exec-bench", "tenant-bench",
			int64(i), int64(i),
			fmt.Sprintf("event-%d", i),
		)
		_ = stream.Emit(ev)
	}
}

// ─── Archive: integrity verification throughput ─────────────────────────────

func BenchmarkArchiveVerification_100Entries(b *testing.B) {
	entries := make([]archive.ArchiveEntry, 100)
	for i := range entries {
		e, _ := archive.NewArchiveEntry(
			fmt.Sprintf("e-%d", i),
			archive.ArchiveTypeExecution,
			"exec-bench", "tenant-bench",
			[]byte(fmt.Sprintf(`{"step":%d}`, i)),
		)
		entries[i] = e
	}
	manifest := archive.BuildArchiveManifest("m-bench", "tenant-bench", entries)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = archive.VerifyArchiveIntegrity(manifest)
	}
}

// ─── Replay: scan throughput ─────────────────────────────────────────────────

func BenchmarkReplayManifestVerify(b *testing.B) {
	cp := replay.ReplayCheckpoint{
		CheckpointID: "cp-bench",
		StreamID:     "stream-bench",
		ExecutionID:  "exec-bench",
		TenantID:     "tenant-bench",
		AtClock:      100,
		AtTimestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		StateHash:    "aabbcc",
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	manifest := replay.BuildReplayManifest(
		"m-bench", "exec-bench", "tenant-bench",
		[]replay.ReplayCheckpoint{cp}, 100,
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = replay.VerifyReplayManifest(manifest)
	}
}

// ─── Contradiction scan throughput ──────────────────────────────────────────

func BenchmarkContradictionDetection(b *testing.B) {
	input := providerops.ContradictionInput{
		OperationID:    "op-bench",
		OperationType:  providerops.OperationTypeDeploy,
		ReportedResult: providerops.ResultStateSuccess,
		ResourceExists: false, // will trigger contradiction
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = providerops.DetectProviderContradiction(
			fmt.Sprintf("c-%d", i),
			"exec-bench", "tenant-bench", "aws",
			input,
		)
	}
}

// ─── Audit: production truth audit throughput ───────────────────────────────

func BenchmarkProductionTruthAudit(b *testing.B) {
	input := audit.AuditInput{
		ExecutionID:         "exec-bench",
		TenantID:            "tenant-bench",
		ReplayDivergences:   0,
		CheckpointHashValid: true,
		PolicyDecisions:     100,
		PolicyDenials:       2,
		ContradictionCount:  1,
		WorkflowDivergences: 0,
		ArchiveHashValid:    true,
		TenantsCrossed:      false,
		ClockMonotonic:      true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = audit.RunProductionTruthAudit(fmt.Sprintf("audit-%d", i), input)
	}
}

// ─── Observability: health assessment throughput ─────────────────────────────

func BenchmarkHealthAssessment(b *testing.B) {
	metrics := observability.ObservabilityMetrics{
		TenantID:           "tenant-bench",
		ExecutionID:        "exec-bench",
		TotalTasks:         1000,
		CompletedTasks:     950,
		FailedTasks:        30,
		ContradictedTasks:  10,
		ContradictionCount: 10,
		ConfidenceScore:    0.95,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = observability.AssessHealth(metrics)
	}
}
