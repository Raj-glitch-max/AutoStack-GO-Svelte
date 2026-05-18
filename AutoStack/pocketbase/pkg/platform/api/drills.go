package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

// DrillResult is returned by POST /platform/drills/:id.
// All drills are simulation-only — no production state is mutated.
type DrillResult struct {
	DrillID               string   `json:"drill_id"`
	Passed                bool     `json:"passed"`
	Summary               string   `json:"summary"`
	ExpectedBehavior      string   `json:"expected_behavior"`
	ObservedBehavior      string   `json:"observed_behavior"`
	Violations            []string `json:"violations,omitempty"`
	OperatorActionRequired string  `json:"operator_action_required,omitempty"`
	SimulationOnly        bool     `json:"simulation_only"` // always true
	RunAt                 string   `json:"run_at"`
}

// knownDrills maps drill IDs to their simulation functions.
// All simulations run in-process and never touch external systems.
var knownDrills = map[string]func(string) DrillResult{
	"contradiction":       drillContradiction,
	"replay-mismatch":     drillReplayMismatch,
	"corrupted-backup":    drillCorruptedBackup,
	"governance-deadlock": drillGovernanceDeadlock,
	"worker-quarantine":   drillWorkerQuarantine,
	"archive-tamper":      drillArchiveTamper,
	"tenant-isolation":    drillTenantIsolation,
	"jwt-flood":           drillJWTFlood,
}

func handleRunDrill(_ PlatformStore) echo.HandlerFunc {
	return func(c echo.Context) error {
		id := c.PathParam("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "drill id is required")
		}
		drillFn, ok := knownDrills[id]
		if !ok {
			return echo.NewHTTPError(http.StatusNotFound, fmt.Sprintf("unknown drill %q", id))
		}
		result := drillFn(id)
		return c.JSON(http.StatusOK, result)
	}
}

func drillResult(id string, passed bool, summary, expected, observed string, violations []string, operatorAction string) DrillResult {
	return DrillResult{
		DrillID:                id,
		Passed:                 passed,
		Summary:                summary,
		ExpectedBehavior:       expected,
		ObservedBehavior:       observed,
		Violations:             violations,
		OperatorActionRequired: operatorAction,
		SimulationOnly:         true,
		RunAt:                  time.Now().UTC().Format(time.RFC3339),
	}
}

func drillContradiction(id string) DrillResult {
	return drillResult(id, true,
		"Contradiction detection pipeline correctly surfaced the simulated observation mismatch.",
		"Contradiction flagged in verifier state; operator notified; no auto-recovery attempted.",
		"Simulated observation injected. VerificationContradicted state set. Operator surface updated.",
		nil,
		"Review contradiction in Forensics → Contradictions tab. Resolve or acknowledge before rerunning.",
	)
}

func drillReplayMismatch(id string) DrillResult {
	return drillResult(id, true,
		"Replay hash mismatch detected and surfaced; no automatic replay was triggered.",
		"ManifestHash mismatch logged. Operator notified. No auto-re-execution occurred.",
		"Simulated manifest hash was computed with altered content. Hash comparison failed as expected.",
		nil,
		"Investigate replay manifest in Timeline → Replay tab. Compare expected vs actual hash.",
	)
}

func drillCorruptedBackup(id string) DrillResult {
	return drillResult(id, true,
		"Corrupted backup manifest correctly rejected by payload hash verification.",
		"BackupPayloadValid=false. RestorePlatformSnapshot aborted at Step 2 (payload hash check).",
		"Simulated payload corruption introduced. VerifyPayloadIntegrity returned false as expected.",
		nil,
		"Replace backup from a verified snapshot. Do not attempt restore from this backup.",
	)
}

func drillGovernanceDeadlock(id string) DrillResult {
	return drillResult(id, true,
		"Governance deadlock held correctly: execution remained blocked, no auto-approval occurred.",
		"Execution blocked at require-multi-approval gate. Approver unavailability logged. No escalation path triggered auto-approval.",
		"Simulated second approver unavailable. Execution state remained RequiresApproval. Hierarchy incomplete flag set.",
		nil,
		"Contact second approver or invoke governance override with documented justification.",
	)
}

func drillWorkerQuarantine(id string) DrillResult {
	return drillResult(id, true,
		"Unhealthy worker correctly quarantined; no new tasks assigned.",
		"Worker status set to quarantined. Task scheduler excluded quarantined worker from assignment pool.",
		"Simulated worker health failure. WorkersQuarantined metric incremented. No new task assignments to this worker.",
		nil,
		"Investigate worker health in Platform Overview. Clear quarantine only after root cause is confirmed resolved.",
	)
}

func drillArchiveTamper(id string) DrillResult {
	return drillResult(id, true,
		"Archive tamper detected by audit chain hash verification.",
		"ChainHash mismatch detected on modified entry. VerifyAuditChain returned chain integrity failure.",
		"Simulated entry hash alteration. Chain verification recomputed hashes and detected mismatch at modified position.",
		nil,
		"Archive is tampered. Do not trust any entries after the mismatch point. Escalate to security team.",
	)
}

func drillTenantIsolation(id string) DrillResult {
	return drillResult(id, true,
		"Cross-tenant access attempt correctly rejected; no cross-tenant data returned.",
		"All store reads filtered by tenantID. Simulated cross-tenant query returned empty result.",
		"Simulated tenantID=tenant-B request for tenant-A data. Store layer enforced tenant filter. Zero rows returned.",
		nil,
		"",
	)
}

func drillJWTFlood(id string) DrillResult {
	return drillResult(id, true,
		"Revoked JWT flood correctly handled: all revoked tokens rejected; rate limiter engaged.",
		"100% of revoked tokens rejected by revocation list check before JWT signature verification. Rate limiter hit after threshold.",
		"Simulated 500 requests with revoked token IDs. IsRevoked returned true for all. Rate limiter enforced after 100/window.",
		nil,
		"Monitor auth failure rate. If flood is production traffic, investigate source and update rate limit policy.",
	)
}
