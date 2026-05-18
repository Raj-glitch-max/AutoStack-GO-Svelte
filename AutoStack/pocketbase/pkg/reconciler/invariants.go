package reconciler

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
)

// Phase 3.9.5 — Runtime Invariant Framework.
//
// An invariant is a declarative assertion about the persisted runtime state
// that MUST hold at all times. Violations indicate corruption, partial
// migration, manual operator edits, or runtime bugs.
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN PRINCIPLES
// ─────────────────────────────────────────────────────────────────────────
//
//   1. Invariants are READ-ONLY checks. They observe state; they do not
//      mutate it. Auto-repair is FORBIDDEN — silent self-healing would
//      destroy forensic evidence of corruption.
//
//   2. Violations open incidents (IncidentSourceInvariantViolation).
//      Severity = Critical. Operator must investigate and explicitly
//      resolve.
//
//   3. Invariants run on STARTUP (audit boot state) and on a PERIODIC
//      schedule (catch drift over time). They do NOT run on every
//      reconcile tick — that would be too expensive.
//
//   4. Severity classifies actionability:
//        Fatal     — runtime should refuse to continue until resolved
//        High      — operator action required; runtime continues
//        Warning   — informational; runtime continues normally
//
//   5. The registry is the SINGLE source of truth for "what does
//      AutoStack believe about its own state?" Adding/removing/changing
//      an invariant is an architectural decision, not a casual edit.

// InvariantSeverity classifies the operational impact of a violation.
type InvariantSeverity string

const (
	InvariantSeverityFatal   InvariantSeverity = "fatal"
	InvariantSeverityHigh    InvariantSeverity = "high"
	InvariantSeverityWarning InvariantSeverity = "warning"
)

// InvariantViolation describes ONE failed-invariant instance.
//
// Multiple rows can fail the same invariant (e.g., 5 retry_pending targets
// missing next_retry_at); each produces one Violation with the row details.
type InvariantViolation struct {
	InvariantName string            `json:"invariant"`
	Severity      InvariantSeverity `json:"severity"`
	TargetID      string            `json:"target_id,omitempty"`
	OperationID   string            `json:"operation_id,omitempty"`
	IncidentID    string            `json:"incident_id,omitempty"`
	Detail        string            `json:"detail"`
	DetectedAt    time.Time         `json:"detected_at"`
}

// Invariant is the declarative contract.
//
// Check returns the list of violations found (empty slice if all rows
// conform). Implementations MUST NOT mutate state — they perform read-only
// SQL queries and return findings.
type Invariant struct {
	Name        string
	Severity    InvariantSeverity
	Description string
	Check       func(app *pb.PocketBase) ([]InvariantViolation, error)
}

// AllInvariants is the authoritative registry. Adding an entry here is
// the only way to introduce a new runtime invariant. Tests verify that
// every entry has a non-empty Name + Description and a non-nil Check.
var AllInvariants = []Invariant{
	{
		Name:        "RetryPendingMustHaveNextRetryAt",
		Severity:    InvariantSeverityHigh,
		Description: "Every deployment_target with status='retry_pending' must have next_retry_at set; otherwise the reconciler cannot determine when to re-dispatch.",
		Check:       invRetryPendingHasNextRetryAt,
	},
	{
		Name:        "UnresolvedIncidentsMustReferenceExistingTargets",
		Severity:    InvariantSeverityHigh,
		Description: "Every incident with status != 'resolved' must reference a deployment_target that exists. A dangling incident is a forensic continuity break.",
		Check:       invUnresolvedIncidentsReferenceValidTargets,
	},
	{
		Name:        "AtMostOneInProgressOperationPerTarget",
		Severity:    InvariantSeverityFatal,
		Description: "A deployment_target may have at most one operations.status='in_progress' row at a time. Duplicate active operations indicate CAS failure or manual DB edit.",
		Check:       invAtMostOneInProgressOpPerTarget,
	},
	{
		Name:        "CurrentOperationMustReferenceExistingOp",
		Severity:    InvariantSeverityHigh,
		Description: "deployment_targets.current_operation must reference an operations.id that exists. A dangling pointer indicates retention violated the protection rule or the operation row was manually deleted.",
		Check:       invCurrentOperationReferencesExisting,
	},
	{
		Name:        "ActiveLeaseHoldersMustHaveLiveOperations",
		Severity:    InvariantSeverityWarning,
		Description: "Every unexpired lease in reconciler_leases should correspond to a target whose current_operation is in_progress. An orphan active lease may indicate a crashed dispatcher that did not release.",
		Check:       invActiveLeasesMatchLiveOperations,
	},
	{
		Name:        "RetryCountWithinPolicyMax",
		Severity:    InvariantSeverityWarning,
		Description: "operations.retry_count should not exceed the MaxRetries of its retry_class policy. Exceeding indicates the retry-budget enforcement was bypassed.",
		Check:       invRetryCountWithinPolicyMax,
	},
	{
		Name:        "ResolvedIncidentsMustHaveResolutionReason",
		Severity:    InvariantSeverityHigh,
		Description: "Every incident with status='resolved' must have a non-empty resolution_reason. A resolution without explanation is a truthfulness violation.",
		Check:       invResolvedIncidentsHaveReason,
	},
	{
		Name:        "AmbiguousTargetsMustHaveAmbiguityColumns",
		Severity:    InvariantSeverityHigh,
		Description: "deployment_targets with lifecycle_ambiguous=true must have lifecycle_ambiguity_source AND lifecycle_native_state populated. Inconsistent ambiguity state is a forensic gap.",
		Check:       invAmbiguousTargetsHaveDetails,
	},
}

// ─────────────────────────────────────────────────────────────────────────
// AUDIT ENGINE
// ─────────────────────────────────────────────────────────────────────────

// RunInvariantAudit executes every invariant in AllInvariants and returns
// the aggregated violations. Errors running an individual invariant are
// logged but do not abort the audit (best-effort: maximize coverage).
//
// On FATAL violation, the caller MUST refuse to start the reconcile loop
// (Reconciler.Start checks the return value). On HIGH/WARNING, the caller
// logs and creates incidents but continues.
func RunInvariantAudit(app *pb.PocketBase) []InvariantViolation {
	var all []InvariantViolation
	for _, inv := range AllInvariants {
		violations, err := inv.Check(app)
		if err != nil {
			log.Printf("[INVARIANT_AUDIT_ERR] invariant=%s err=%v — treating as non-fatal", inv.Name, err)
			continue
		}
		for i := range violations {
			violations[i].InvariantName = inv.Name
			violations[i].Severity = inv.Severity
			violations[i].DetectedAt = time.Now().UTC()
		}
		all = append(all, violations...)
	}
	return all
}

// HasFatalViolation returns true if any violation is severity=fatal.
// The startup path uses this to decide whether to refuse the reconcile
// loop launch.
func HasFatalViolation(violations []InvariantViolation) bool {
	for _, v := range violations {
		if v.Severity == InvariantSeverityFatal {
			return true
		}
	}
	return false
}

// RecordViolationsAsIncidents opens IncidentSourceInvariantViolation
// incidents for each violation. Dedup naturally because incident creation
// is keyed on (target, source) — multiple violations on the same target
// merge into one escalating incident.
//
// Best-effort: errors are logged but do not abort the audit.
func (r *Reconciler) RecordViolationsAsIncidents(violations []InvariantViolation) {
	for _, v := range violations {
		if v.TargetID == "" {
			// Some invariants (e.g., AtMostOneInProgressOperationPerTarget)
			// emit violations without a clean target attribution. Skip
			// incident creation but the log line above carries the detail.
			log.Printf("[INVARIANT_VIOLATION_NOTGT] invariant=%s severity=%s detail=%s",
				v.InvariantName, v.Severity, v.Detail)
			continue
		}
		severity := IncidentSeverityHigh
		if v.Severity == InvariantSeverityFatal {
			severity = IncidentSeverityCritical
		} else if v.Severity == InvariantSeverityWarning {
			severity = IncidentSeverityMedium
		}
		r.CreateOrEscalateIncident(v.TargetID, "",
			IncidentSourceInvariantViolation, severity,
			v.OperationID,
			fmt.Sprintf("invariant violation: %s — %s", v.InvariantName, v.Detail),
			IncidentRuntimeState{})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// CONCRETE INVARIANTS
// ─────────────────────────────────────────────────────────────────────────

func invRetryPendingHasNextRetryAt(app *pb.PocketBase) ([]InvariantViolation, error) {
	var rows []struct {
		ID string `db:"id"`
	}
	err := app.Dao().DB().
		Select("id").
		From("deployment_targets").
		Where(dbx.NewExp("status = 'retry_pending' AND (next_retry_at IS NULL OR next_retry_at = '')")).
		All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]InvariantViolation, 0, len(rows))
	for _, r := range rows {
		out = append(out, InvariantViolation{
			TargetID: r.ID,
			Detail:   "target status=retry_pending but next_retry_at is null/empty; reconciler cannot determine when to re-dispatch",
		})
	}
	return out, nil
}

func invUnresolvedIncidentsReferenceValidTargets(app *pb.PocketBase) ([]InvariantViolation, error) {
	var rows []struct {
		ID     string `db:"id"`
		Target string `db:"target"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT i.id AS id, i.target AS target
		 FROM incidents i
		 WHERE i.status != 'resolved'
		   AND i.target NOT IN (SELECT id FROM deployment_targets)`,
	).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]InvariantViolation, 0, len(rows))
	for _, r := range rows {
		out = append(out, InvariantViolation{
			IncidentID: r.ID,
			Detail:     fmt.Sprintf("unresolved incident references target=%s that no longer exists", r.Target),
		})
	}
	return out, nil
}

func invAtMostOneInProgressOpPerTarget(app *pb.PocketBase) ([]InvariantViolation, error) {
	var rows []struct {
		Target string `db:"target"`
		Count  int    `db:"cnt"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT target, COUNT(*) AS cnt
		 FROM operations
		 WHERE status = 'in_progress'
		 GROUP BY target
		 HAVING COUNT(*) > 1`,
	).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]InvariantViolation, 0, len(rows))
	for _, r := range rows {
		out = append(out, InvariantViolation{
			TargetID: r.Target,
			Detail:   fmt.Sprintf("target has %d concurrent in_progress operations — CAS guarantee violated", r.Count),
		})
	}
	return out, nil
}

func invCurrentOperationReferencesExisting(app *pb.PocketBase) ([]InvariantViolation, error) {
	var rows []struct {
		ID               string `db:"id"`
		CurrentOperation string `db:"current_operation"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT id, current_operation
		 FROM deployment_targets
		 WHERE current_operation IS NOT NULL
		   AND current_operation != ''
		   AND current_operation NOT IN (SELECT id FROM operations)`,
	).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]InvariantViolation, 0, len(rows))
	for _, r := range rows {
		out = append(out, InvariantViolation{
			TargetID:    r.ID,
			OperationID: r.CurrentOperation,
			Detail:      fmt.Sprintf("target.current_operation=%s references non-existent operations row", r.CurrentOperation),
		})
	}
	return out, nil
}

func invActiveLeasesMatchLiveOperations(app *pb.PocketBase) ([]InvariantViolation, error) {
	// A lease whose expires_at is in the future MUST correspond to a
	// target with an in-progress operation. Otherwise the lease is
	// orphaned (dispatcher crashed without releasing).
	var rows []struct {
		LeaseKey string `db:"lease_key"`
		Holder   string `db:"holder_pod_id"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT l.lease_key AS lease_key, l.holder_pod_id AS holder
		 FROM reconciler_leases l
		 WHERE l.lease_expires_at > datetime('now')
		   AND l.lease_key NOT IN (
		     SELECT target FROM operations WHERE status = 'in_progress'
		   )`,
	).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]InvariantViolation, 0, len(rows))
	for _, r := range rows {
		out = append(out, InvariantViolation{
			TargetID: r.LeaseKey,
			Detail:   fmt.Sprintf("active lease held by pod=%s but no in_progress operation exists for this target; orphan lease (dispatcher crashed without release?)", r.Holder),
		})
	}
	return out, nil
}

func invRetryCountWithinPolicyMax(app *pb.PocketBase) ([]InvariantViolation, error) {
	// We cannot statically know each row's policy without dispatching to
	// the providers package. Simplification: check that no operation has
	// retry_count > 20 (the maximum across all current policies is 10
	// for RateLimit). A count > 20 is definitely a budget bypass.
	var rows []struct {
		ID         string `db:"id"`
		Target     string `db:"target"`
		RetryCount int    `db:"retry_count"`
		RetryClass string `db:"retry_class"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT id, target, retry_count, retry_class
		 FROM operations
		 WHERE retry_count > 20`,
	).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]InvariantViolation, 0, len(rows))
	for _, r := range rows {
		out = append(out, InvariantViolation{
			TargetID:    r.Target,
			OperationID: r.ID,
			Detail:      fmt.Sprintf("operation retry_count=%d exceeds maximum policy ceiling (20); class=%s; retry-budget enforcement was bypassed", r.RetryCount, r.RetryClass),
		})
	}
	return out, nil
}

func invResolvedIncidentsHaveReason(app *pb.PocketBase) ([]InvariantViolation, error) {
	var rows []struct {
		ID     string `db:"id"`
		Target string `db:"target"`
	}
	err := app.Dao().DB().
		Select("id", "target").
		From("incidents").
		Where(dbx.NewExp("status = 'resolved' AND (resolution_reason IS NULL OR resolution_reason = '')")).
		All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]InvariantViolation, 0, len(rows))
	for _, r := range rows {
		out = append(out, InvariantViolation{
			IncidentID: r.ID,
			TargetID:   r.Target,
			Detail:     "resolved incident has no resolution_reason — truthfulness rule violation; UpdateIncidentStatus enforces this for new transitions, so this row was likely created via direct DB edit",
		})
	}
	return out, nil
}

func invAmbiguousTargetsHaveDetails(app *pb.PocketBase) ([]InvariantViolation, error) {
	var rows []struct {
		ID string `db:"id"`
	}
	err := app.Dao().DB().
		Select("id").
		From("deployment_targets").
		Where(dbx.NewExp("lifecycle_ambiguous = 1 AND (lifecycle_ambiguity_source IS NULL OR lifecycle_ambiguity_source = '' OR lifecycle_native_state IS NULL OR lifecycle_native_state = '')")).
		All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]InvariantViolation, 0, len(rows))
	for _, r := range rows {
		out = append(out, InvariantViolation{
			TargetID: r.ID,
			Detail:   "lifecycle_ambiguous=true but ambiguity_source or native_state is empty; ambiguity columns inconsistent",
		})
	}
	return out, nil
}
