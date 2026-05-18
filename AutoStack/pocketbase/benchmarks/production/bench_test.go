// Package production provides production-realistic benchmarks for AutoStack.
//
// These benchmarks measure actual observed performance — not theoretical limits.
// Run with: go test ./benchmarks/production/... -bench=. -benchmem -benchtime=3s
//
// Results are in ns/op and allocations. Use these as baselines for regression detection.
package production

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/backup"
	"github.com/janlauber/one-click/pkg/certification"
	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/observability"
	"github.com/janlauber/one-click/pkg/persistence"
	"github.com/janlauber/one-click/pkg/replay"
	"github.com/janlauber/one-click/pkg/security"
)

// BenchmarkReplayManifestBuild measures replay manifest construction latency.
// Input: fixed execution parameters, 1000 events.
func BenchmarkReplayManifestBuild(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = replay.BuildReplayManifest("mfst-bench", "exec-bench", "tenant-bench", nil, 1000)
	}
}

// BenchmarkReplayManifestVerify measures manifest hash verification latency.
func BenchmarkReplayManifestVerify(b *testing.B) {
	m := replay.BuildReplayManifest("mfst-bench", "exec-bench", "tenant-bench", nil, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = replay.VerifyReplayManifest(m)
	}
}

// BenchmarkContradictionScan measures contradiction detection latency per observation.
func BenchmarkContradictionScan(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = observability.ComputeMetrics(
			"tenant-bench", "exec-bench",
			1000, 990, 5, 3, 0, // tasks
			2, 1, 2, 0, // retry, approval, contradiction, divergence
			0.99,  // confidence
			5000, 4, 4, // events, workers
		)
	}
}

// BenchmarkCertificationGeneration measures end-to-end certification latency (all 5 gates).
func BenchmarkCertificationGeneration(b *testing.B) {
	auditRes := audit.RunProductionTruthAudit("a-bench", audit.AuditInput{
		ExecutionID:         "exec-bench",
		TenantID:            "tenant-bench",
		CheckpointHashValid: true,
		ArchiveHashValid:    true,
		ClockMonotonic:      true,
	})
	safetyRes := audit.AssessOperatorSafety("s-bench", audit.OperatorSafetyInput{
		ExecutionID:           "exec-bench",
		TenantID:              "tenant-bench",
		AutomatedActionsCount: 10,
		ApprovedActionsCount:  10,
	})
	replayCert := audit.CertifyReplay("rc-bench", "exec-bench", "tenant-bench", true, true, true, true, 1000, 0)
	health := observability.AssessHealth(observability.ObservabilityMetrics{
		TenantID:        "tenant-bench",
		ExecutionID:     "exec-bench",
		TotalTasks:      1000,
		CompletedTasks:  1000,
		ConfidenceScore: 1.0,
	})
	input := certification.PlatformReadinessInput{
		ExecutionID:         "exec-bench",
		TenantID:            "tenant-bench",
		AuditResult:         auditRes,
		SafetyAssessment:    safetyRes,
		ReplayCertification: &replayCert,
		HealthAssessment:    health,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = certification.RunPlatformReadinessAudit("cert-bench", input)
	}
}

// BenchmarkAuditPersistenceThroughput measures audit entry persistence throughput.
func BenchmarkAuditPersistenceThroughput(b *testing.B) {
	store, _ := security.NewSQLiteAuditStore(":memory:")
	defer store.Close()
	rec := security.NewDurableAuditRecorder(store)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Record(
			fmt.Sprintf("bench-%d", i),
			security.AuditEventAuthSuccess,
			"tenant-bench",
			fmt.Sprintf("req-%d", i),
			string(security.RoleOperator),
			"allowed",
		)
	}
}

// BenchmarkSQLiteKVPut measures SQLite WAL write throughput.
func BenchmarkSQLiteKVPut(b *testing.B) {
	store, _ := persistence.NewSQLiteKVStore(":memory:")
	defer store.Close()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		store.Put(ctx, key, []byte(`{"value":"bench"}`))
	}
}

// BenchmarkSQLiteKVGet measures SQLite WAL read throughput.
func BenchmarkSQLiteKVGet(b *testing.B) {
	store, _ := persistence.NewSQLiteKVStore(":memory:")
	defer store.Close()
	ctx := context.Background()

	// Pre-populate 1000 keys.
	for i := 0; i < 1000; i++ {
		store.Put(ctx, fmt.Sprintf("get-key-%d", i), []byte(`{"v":1}`))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("get-key-%d", i%1000)
		store.Get(ctx, key)
	}
}

// BenchmarkJWTIssue measures JWT issuance latency.
func BenchmarkJWTIssue(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		security.IssueJWT(fmt.Sprintf("tok-%d", i), "tenant-bench", security.RoleOperator, time.Hour, "bench-secret-value")
	}
}

// BenchmarkJWTVerify measures JWT verification latency.
func BenchmarkJWTVerify(b *testing.B) {
	tok, _ := security.IssueJWT("bench-tok", "tenant-bench", security.RoleOperator, time.Hour, "bench-secret")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		security.VerifyJWT(tok, "bench-secret", "tenant-bench")
	}
}

// BenchmarkRateLimiterCheck measures rate limit check latency under no contention.
func BenchmarkRateLimiterCheck(b *testing.B) {
	rl := security.NewRateLimiter(security.RateLimitPolicy{MaxRequests: 1_000_000, Window: time.Hour})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Check("bench-key")
	}
}

// BenchmarkBackupManifestCreate measures backup manifest creation latency.
func BenchmarkBackupManifestCreate(b *testing.B) {
	payload := []byte(`{"entries":1000,"version":"7.0.0","state":"certified"}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backup.CreateBackupManifest(
			fmt.Sprintf("bk-%d", i), "tenant-bench", "exec-bench", "7.0.0",
			backup.BackupKindPlatformSnapshot, payload,
		)
	}
}

// BenchmarkProductionReadinessAudit measures 10-gate readiness audit latency.
func BenchmarkProductionReadinessAudit(b *testing.B) {
	input := backup.ReadinessInput{
		TenantID:                  "tenant-bench",
		ExecutionID:               "exec-bench",
		StorageReadsHealthy:       true,
		StorageWritesHealthy:      true,
		ReplayDivergenceCount:     0,
		ReplayCertified:           true,
		ContradictionScanOK:       true,
		GovernanceDecisionCount:   100,
		GovernanceHashValid:       true,
		ArchiveEntryCount:         10000,
		ArchiveHashValid:          true,
		ActiveWorkerCount:         4,
		QuarantinedWorkers:        0,
		APIEndpointsHealthy:       true,
		RBACRulesValid:            true,
		NoCredentialLeak:          true,
		LatestBackupManifestValid: true,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backup.RunProductionReadinessAudit(fmt.Sprintf("rpt-%d", i), input)
	}
}

// BenchmarkTaskCreation measures ExecutionTask constructor latency.
func BenchmarkTaskCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.NewExecutionTask(
			fmt.Sprintf("task-%d", i), "exec-bench", "wf-1", "step-1", "tenant-bench",
			executor.TaskTypeDeploy, 1, int64(i),
		)
	}
}
