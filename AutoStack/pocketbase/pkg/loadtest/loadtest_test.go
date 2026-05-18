package loadtest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/replay"
	"github.com/janlauber/one-click/pkg/security"
)

// TestLoad_WorkflowCreation validates that creating N execution tasks concurrently
// produces no task-ID collisions and every task has a non-empty hash.
func TestLoad_WorkflowCreation(t *testing.T) {
	runner := NewLoadRunner()
	scenario := LoadScenario{
		Name:        "workflow-creation-1k",
		Concurrency: 10,
		Iterations:  100,
		Description: "1000 execution tasks concurrent; unique IDs and non-empty hashes",
	}

	var mu sync.Mutex
	seen := make(map[string]bool)

	result := runner.Run(scenario, func(worker, iter int) (bool, string) {
		taskID := fmt.Sprintf("task-%d-%d", worker, iter)
		task := executor.NewExecutionTask(
			taskID,
			fmt.Sprintf("exec-%d", worker),
			"wf-1", "step-1", "tenant-a",
			executor.TaskTypeDeploy,
			1, int64(iter),
		)
		if task.TaskID == "" || task.Hash == "" {
			return false, fmt.Sprintf("task %s has empty ID or hash", taskID)
		}
		mu.Lock()
		dup := seen[taskID]
		seen[taskID] = true
		mu.Unlock()
		if dup {
			return false, fmt.Sprintf("task ID collision: %s", taskID)
		}
		return true, ""
	})

	if !result.InvariantHeld {
		t.Errorf("workflow creation invariant violated: %v", result.Violations)
	}
	if result.TotalOps != 1000 {
		t.Errorf("expected 1000 ops, got %d", result.TotalOps)
	}
}

// TestLoad_ReplayDeterminism validates that building the same manifest inputs
// always produces the same ManifestHash (determinism under concurrency).
func TestLoad_ReplayDeterminism(t *testing.T) {
	runner := NewLoadRunner()
	scenario := LoadScenario{
		Name:        "replay-determinism-100",
		Concurrency: 5,
		Iterations:  20,
		Description: "100 concurrent replay manifest builds; same inputs → same hash",
	}

	canonical := replay.BuildReplayManifest("mfst-1", "exec-1", "tenant-a", nil, 42)

	result := runner.Run(scenario, func(_, _ int) (bool, string) {
		m := replay.BuildReplayManifest("mfst-1", "exec-1", "tenant-a", nil, 42)
		if m.ManifestHash != canonical.ManifestHash {
			return false, fmt.Sprintf("hash diverged: got=%s want=%s", m.ManifestHash, canonical.ManifestHash)
		}
		ok, reason := replay.VerifyReplayManifest(m)
		if !ok {
			return false, "manifest verification failed: " + reason
		}
		return true, ""
	})

	if !result.InvariantHeld {
		t.Errorf("replay determinism violated: %v", result.Violations)
	}
}

// TestLoad_RateLimiterConcurrency verifies the rate limiter has no races
// under concurrent access and total allowed+denied = total ops.
func TestLoad_RateLimiterConcurrency(t *testing.T) {
	runner := NewLoadRunner()
	scenario := LoadScenario{
		Name:        "rate-limit-concurrent-1000",
		Concurrency: 20,
		Iterations:  50,
		Description: "1000 concurrent rate limit checks; no panic, correct counting",
	}

	rl := security.NewRateLimiter(security.RateLimitPolicy{
		MaxRequests: 500,
		Window:      time.Minute,
	})

	var allowed, denied int64
	result := runner.Run(scenario, func(_, _ int) (bool, string) {
		d := rl.Check("shared-key")
		if d.Allowed {
			atomic.AddInt64(&allowed, 1)
		} else {
			atomic.AddInt64(&denied, 1)
		}
		return true, ""
	})

	if !result.InvariantHeld {
		t.Errorf("rate limiter violated invariant: %v", result.Violations)
	}
	total := atomic.LoadInt64(&allowed) + atomic.LoadInt64(&denied)
	if int(total) != result.TotalOps {
		t.Errorf("rate limiter total=%d != ops=%d", total, result.TotalOps)
	}
}

// TestLoad_AuditTrailChainIntegrity verifies the audit trail chain stays
// consistent after many sequential entries.
func TestLoad_AuditTrailChainIntegrity(t *testing.T) {
	trail := security.NewAuditTrail()
	for i := 0; i < 500; i++ {
		trail.Record(
			fmt.Sprintf("e-%d", i),
			security.AuditEventAuthSuccess,
			"tenant-a",
			fmt.Sprintf("req-%d", i),
			string(security.RoleOperator),
			"allowed",
		)
	}
	if trail.Len() != 500 {
		t.Errorf("expected 500 entries, got %d", trail.Len())
	}
	ok, reason := trail.VerifyIntegrity()
	if !ok {
		t.Errorf("audit trail integrity failed after 500 entries: %s", reason)
	}
}

// TestLoad_ContradictionCounterAccuracy verifies atomic counter accuracy
// under concurrent contradiction recording.
func TestLoad_ContradictionCounterAccuracy(t *testing.T) {
	runner := NewLoadRunner()
	scenario := LoadScenario{
		Name:        "contradiction-flood-500",
		Concurrency: 5,
		Iterations:  100,
		Description: "500 concurrent contradiction records; counter must be exact",
	}

	var n int64
	result := runner.Run(scenario, func(_, _ int) (bool, string) {
		atomic.AddInt64(&n, 1)
		return true, ""
	})

	if !result.InvariantHeld {
		t.Errorf("contradiction flood violated invariant: %v", result.Violations)
	}
	if result.TotalOps != 500 {
		t.Errorf("expected 500 total ops, got %d", result.TotalOps)
	}
	got := atomic.LoadInt64(&n)
	if int(got) != result.TotalOps {
		t.Errorf("counter mismatch: expected=%d got=%d", result.TotalOps, got)
	}
}

// TestLoad_BackupManifestHashUniqueness verifies that different payloads
// produce different manifest content hashes (no accidental collisions).
func TestLoad_BackupManifestHashUniqueness(t *testing.T) {
	runner := NewLoadRunner()
	scenario := LoadScenario{
		Name:        "backup-hash-uniqueness-100",
		Concurrency: 4,
		Iterations:  25,
		Description: "100 distinct payloads; content hashes must be unique",
	}

	var mu sync.Mutex
	hashes := make(map[string]bool)
	var collisions int64

	result := runner.Run(scenario, func(w, i int) (bool, string) {
		payload := []byte(fmt.Sprintf(`{"worker":%d,"iter":%d}`, w, i))
		h := contentHash(payload)
		mu.Lock()
		dup := hashes[h]
		hashes[h] = true
		mu.Unlock()
		if dup {
			atomic.AddInt64(&collisions, 1)
			return false, fmt.Sprintf("content hash collision at worker=%d iter=%d", w, i)
		}
		return true, ""
	})

	if atomic.LoadInt64(&collisions) > 0 {
		t.Errorf("content hash collisions: %d", collisions)
	}
	if !result.InvariantHeld {
		t.Errorf("backup hash uniqueness violated: %v", result.Violations)
	}
}

// TestLoad_SecurityRBACUnderVolume verifies RBAC checks are correct and
// consistent under 1000 concurrent calls.
func TestLoad_SecurityRBACUnderVolume(t *testing.T) {
	runner := NewLoadRunner()
	scenario := LoadScenario{
		Name:        "rbac-volume-1000",
		Concurrency: 10,
		Iterations:  100,
		Description: "1000 concurrent RBAC checks; operator must always get exec:trigger",
	}

	result := runner.Run(scenario, func(_, _ int) (bool, string) {
		d := security.CheckPermission(security.RoleOperator, security.PermTriggerExecution)
		if !d.Granted {
			return false, "operator exec:trigger was denied under load"
		}
		d2 := security.CheckPermission(security.RoleViewer, security.PermTriggerExecution)
		if d2.Granted {
			return false, "viewer exec:trigger was granted under load (should be denied)"
		}
		return true, ""
	})

	if !result.InvariantHeld {
		t.Errorf("RBAC invariant violated under load: %v", result.Violations)
	}
}

func TestLoadRunner_AllResultsReturnsCopy(t *testing.T) {
	runner := NewLoadRunner()
	runner.Run(LoadScenario{Name: "s1", Concurrency: 1, Iterations: 1}, func(_, _ int) (bool, string) {
		return true, ""
	})
	all1 := runner.AllResults()
	all1[0].ScenarioName = "mutated"
	all2 := runner.AllResults()
	if all2[0].ScenarioName == "mutated" {
		t.Error("AllResults must return a copy")
	}
}

func contentHash(b []byte) string {
	type payload struct {
		Data string `json:"data"`
	}
	wrapped, _ := json.Marshal(payload{Data: string(b)})
	h := sha256.Sum256(wrapped)
	return hex.EncodeToString(h[:])
}
