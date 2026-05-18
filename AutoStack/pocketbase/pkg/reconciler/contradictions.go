package reconciler

import (
	"fmt"
	"log"
	"time"

	pb "github.com/pocketbase/pocketbase"
)

// Phase 3.9.6 — Cross-table contradiction detection.
//
// Contradictions are inconsistencies BETWEEN tables that single-table
// invariants (Phase 3.9.5) cannot catch. Examples:
//   - operations.status='succeeded' AND there's still an unresolved
//     incident pointing at the operation
//   - runtime_decisions.operation_id references an operation that
//     retention has deleted (FK guard failed)
//
// When a contradiction is detected, the runtime:
//   1. Opens (or escalates) an IncidentSourceInvariantViolation incident
//   2. Persists a DecisionContradictionDetected runtime decision
//   3. Never auto-resolves or auto-repairs — operator must investigate
//
// The 5 contradictions implemented here are the load-bearing forensic
// checks. More can be added; the bar is "would silent occurrence corrupt
// operator trust?"

// ContradictionCheck — declarative definition similar to Invariant.
type ContradictionCheck struct {
	Name        string
	Severity    InvariantSeverity
	Description string
	Check       func(app *pb.PocketBase) ([]Contradiction, error)
}

// Contradiction — one detected violation.
type Contradiction struct {
	CheckName   string    `json:"check_name"`
	Severity    string    `json:"severity"`
	TargetID    string    `json:"target_id"`
	OperationID string    `json:"operation_id,omitempty"`
	IncidentID  string    `json:"incident_id,omitempty"`
	Detail      string    `json:"detail"`
	DetectedAt  time.Time `json:"detected_at"`
}

// AllContradictions — the canonical registry. Adding a check here is the
// only way to introduce a new cross-table consistency assertion.
var AllContradictions = []ContradictionCheck{
	{
		Name:        "OperationSucceededButIncidentStillOpen",
		Severity:    InvariantSeverityHigh,
		Description: "An operation completed successfully but a non-resolved incident still references it. The operator's view says ongoing failure; the runtime says success.",
		Check:       checkOpSucceededIncidentOpen,
	},
	{
		Name:        "OperationFailedButHistoryShowsSuccess",
		Severity:    InvariantSeverityHigh,
		Description: "An operation row says failed but the deployment_history audit log shows a success entry for the same operation. Terminal disagreement between the two truth sources.",
		Check:       checkOpFailedHistorySuccess,
	},
	{
		Name:        "RetryDecisionReferencesMissingOperation",
		Severity:    InvariantSeverityHigh,
		Description: "A runtime_decisions row references operation_id that no longer exists. Retention violated the FK protection rule.",
		Check:       checkDecisionMissingOp,
	},
	{
		Name:        "TargetRunningWithUnresolvedRetryIncident",
		Severity:    InvariantSeverityWarning,
		Description: "Target status is 'running' but there's an unresolved retry-exhausted incident. The target appears healthy; the operator's incident view is stale or contradicts.",
		Check:       checkTargetRunningRetryIncidentOpen,
	},
	{
		Name:        "RetryExhaustedDecisionWithoutIncident",
		Severity:    InvariantSeverityHigh,
		Description: "A retry_exhausted decision was persisted but no incident exists for the same operation. The retry engine recorded the decision but the incident creation failed silently.",
		Check:       checkRetryExhaustedNoIncident,
	},

	// Phase 3.9.9 — provider silence detection.
	{
		Name:        "ProviderSilenceOnActiveTarget",
		Severity:    InvariantSeverityHigh,
		Description: "An active target (running/updating) has received no authoritative or inferred provider observations in the last 10 minutes, despite no in-flight operation. The provider may be unreachable, rate-limited, or the reconciler may have stopped polling this target. Provider silence is classified separately from provider failure.",
		Check:       checkProviderSilence,
	},

	// Phase 3.9.7 — provider truth contradiction checks.
	{
		Name:        "ProviderDriftExceedingEscalationThreshold",
		Severity:    InvariantSeverityHigh,
		Description: "A provider_drifts record has been open for more than 4 hours. Prolonged drift means either the provider is persistently wrong, the runtime spec is incorrect, or an external actor changed cloud state. Operator investigation required.",
		Check:       checkProviderDriftPersistent,
	},
	{
		Name:        "RepeatedAmbiguousObservationsWithoutResolution",
		Severity:    InvariantSeverityWarning,
		Description: "A target has had 10 or more consecutive ambiguous/stale observations recorded in provider_observations without a single authoritative observation. The provider may be unreachable or consistently returning incomplete results.",
		Check:       checkRepeatedAmbiguousObservations,
	},
}

// RunContradictionAudit runs every check and returns aggregated findings.
// Errors running an individual check are logged but do not abort.
func RunContradictionAudit(app *pb.PocketBase) []Contradiction {
	var all []Contradiction
	for _, check := range AllContradictions {
		findings, err := check.Check(app)
		if err != nil {
			log.Printf("[CONTRADICTION_CHECK_ERR] check=%s err=%v — treating as non-fatal", check.Name, err)
			continue
		}
		for i := range findings {
			findings[i].CheckName = check.Name
			findings[i].Severity = string(check.Severity)
			findings[i].DetectedAt = time.Now().UTC()
		}
		all = append(all, findings...)
	}
	return all
}

// RecordContradictions persists each finding as both an incident AND
// a runtime_decision. Best-effort — errors logged not propagated.
func (r *Reconciler) RecordContradictions(findings []Contradiction) {
	for _, c := range findings {
		if c.TargetID == "" {
			log.Printf("[CONTRADICTION_NOTGT] check=%s detail=%s", c.CheckName, c.Detail)
			continue
		}
		severity := IncidentSeverityHigh
		if c.Severity == "warning" {
			severity = IncidentSeverityMedium
		}
		incID, _ := r.CreateOrEscalateIncident(c.TargetID, "",
			IncidentSourceInvariantViolation, severity,
			c.OperationID,
			"contradiction: "+c.CheckName+" — "+c.Detail,
			IncidentRuntimeState{})
		evRefs := []EvidenceRef{}
		if c.OperationID != "" {
			evRefs = append(evRefs, EvidenceRef{Collection: "operations", ID: c.OperationID})
		}
		if incID != "" {
			evRefs = append(evRefs, EvidenceRef{Collection: "incidents", ID: incID})
		}
		r.RecordDecision(DecisionRecord{
			TargetID:       c.TargetID,
			OperationID:    c.OperationID,
			DecisionType:   DecisionContradictionDetected,
			DecisionReason: c.CheckName,
			HumanSummary:   c.Detail,
			DecisionInputs: map[string]interface{}{
				"check":    c.CheckName,
				"severity": c.Severity,
			},
			EvidenceRefs: evRefs,
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// CHECK IMPLEMENTATIONS
// ─────────────────────────────────────────────────────────────────────────

func checkOpSucceededIncidentOpen(app *pb.PocketBase) ([]Contradiction, error) {
	var rows []struct {
		OpID       string `db:"op_id"`
		IncidentID string `db:"incident_id"`
		Target     string `db:"target"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT o.id AS op_id, i.id AS incident_id, o.target AS target
		 FROM operations o
		 INNER JOIN incidents i ON (i.last_operation_id = o.id OR i.first_operation_id = o.id)
		 WHERE o.status = 'succeeded' AND i.status != 'resolved'`,
	).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]Contradiction, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contradiction{
			TargetID:    r.Target,
			OperationID: r.OpID,
			IncidentID:  r.IncidentID,
			Detail:      fmt.Sprintf("operation %s succeeded but incident %s remains unresolved", r.OpID, r.IncidentID),
		})
	}
	return out, nil
}

func checkOpFailedHistorySuccess(app *pb.PocketBase) ([]Contradiction, error) {
	var rows []struct {
		OpID   string `db:"op_id"`
		Target string `db:"target"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT o.id AS op_id, o.target AS target
		 FROM operations o
		 WHERE o.status = 'failed'
		   AND o.id IN (
		     SELECT operation_id FROM deployment_history
		     WHERE status = 'success' AND operation_id != ''
		   )`,
	).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]Contradiction, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contradiction{
			TargetID:    r.Target,
			OperationID: r.OpID,
			Detail:      fmt.Sprintf("operation %s status=failed but a history row says success for same operation", r.OpID),
		})
	}
	return out, nil
}

func checkDecisionMissingOp(app *pb.PocketBase) ([]Contradiction, error) {
	var rows []struct {
		DecisionID string `db:"decision_id"`
		Target     string `db:"target"`
		OpID       string `db:"operation_id"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT id AS decision_id, target, operation_id
		 FROM runtime_decisions
		 WHERE operation_id != ''
		   AND operation_id IS NOT NULL
		   AND operation_id NOT IN (SELECT id FROM operations)
		   AND decision_timestamp > {:cutoff}`,
	).Bind(map[string]interface{}{
		// Only check decisions newer than the FK-guard retention window (90d).
		// Older decisions may legitimately reference deleted operations.
		"cutoff": time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339),
	}).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]Contradiction, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contradiction{
			TargetID:    r.Target,
			OperationID: r.OpID,
			Detail:      fmt.Sprintf("decision %s references operation %s which no longer exists; retention FK guard violated", r.DecisionID, r.OpID),
		})
	}
	return out, nil
}

func checkTargetRunningRetryIncidentOpen(app *pb.PocketBase) ([]Contradiction, error) {
	var rows []struct {
		Target     string `db:"target"`
		IncidentID string `db:"incident_id"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT t.id AS target, i.id AS incident_id
		 FROM deployment_targets t
		 INNER JOIN incidents i ON i.target = t.id
		 WHERE t.status = 'running'
		   AND i.status NOT IN ('resolved', 'suppressed')
		   AND i.source = 'retry-exhausted'`,
	).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]Contradiction, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contradiction{
			TargetID:   r.Target,
			IncidentID: r.IncidentID,
			Detail:     fmt.Sprintf("target is 'running' but retry-exhausted incident %s is unresolved", r.IncidentID),
		})
	}
	return out, nil
}

func checkRetryExhaustedNoIncident(app *pb.PocketBase) ([]Contradiction, error) {
	var rows []struct {
		DecisionID string `db:"decision_id"`
		Target     string `db:"target"`
		OpID       string `db:"operation_id"`
	}
	// Look at recent retry_exhausted decisions only; older ones may have
	// had their incidents purged by retention.
	cutoff := time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339)
	err := app.Dao().DB().NewQuery(
		`SELECT id AS decision_id, target, operation_id
		 FROM runtime_decisions
		 WHERE decision_type = 'retry_exhausted'
		   AND decision_timestamp > {:cutoff}
		   AND operation_id != ''
		   AND operation_id NOT IN (
		     SELECT first_operation_id FROM incidents
		     WHERE first_operation_id != '' AND first_operation_id IS NOT NULL
		     UNION
		     SELECT last_operation_id FROM incidents
		     WHERE last_operation_id != '' AND last_operation_id IS NOT NULL
		   )`,
	).Bind(map[string]interface{}{"cutoff": cutoff}).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]Contradiction, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contradiction{
			TargetID:    r.Target,
			OperationID: r.OpID,
			Detail:      fmt.Sprintf("decision %s recorded retry_exhausted for op %s but no matching incident exists", r.DecisionID, r.OpID),
		})
	}
	return out, nil
}

// Phase 3.9.9 — Provider silence detection.
// Active targets (running/updating) with no authoritative/inferred observation
// in the last 10 minutes AND no in-flight operation are "silent". Silence ≠
// failure — the provider may be temporarily unreachable. Targets that have
// never been observed (newly deployed, no first poll yet) are excluded.
func checkProviderSilence(app *pb.PocketBase) ([]Contradiction, error) {
	const silenceWindowMinutes = 10
	cutoff := time.Now().UTC().Add(-silenceWindowMinutes * time.Minute).Format(time.RFC3339)
	var rows []struct {
		Target string `db:"target"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT t.id AS target
		 FROM deployment_targets t
		 WHERE t.status IN ('running', 'updating')
		   AND t.id NOT IN (
		     SELECT DISTINCT target FROM provider_observations
		     WHERE observed_at > {:cutoff}
		       AND observation_trust IN ('authoritative', 'inferred')
		   )
		   AND t.id NOT IN (
		     SELECT DISTINCT target FROM operations
		     WHERE status IN ('pending', 'in_progress', 'in-progress')
		   )
		   AND t.id IN (
		     SELECT DISTINCT target FROM provider_observations
		   )`,
	).Bind(map[string]interface{}{"cutoff": cutoff}).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]Contradiction, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contradiction{
			TargetID: r.Target,
			Detail: fmt.Sprintf("no authoritative/inferred provider observation in the last %d minutes for active target — provider may be silent",
				silenceWindowMinutes),
		})
	}
	return out, nil
}

// Phase 3.9.7 — Provider drift escalation check.
// A drift open > 4 hours indicates either persistent provider divergence or
// a runtime belief that no longer reflects reality.
func checkProviderDriftPersistent(app *pb.PocketBase) ([]Contradiction, error) {
	const driftEscalationHours = 4
	cutoff := time.Now().UTC().Add(-driftEscalationHours * time.Hour).Format(time.RFC3339)
	var rows []struct {
		ID          string `db:"id"`
		Target      string `db:"target"`
		Provider    string `db:"provider"`
		FirstSeenAt string `db:"first_seen_at"`
		DriftCount  int    `db:"drift_count"`
	}
	err := app.Dao().DB().NewQuery(
		`SELECT id, target, provider, first_seen_at, drift_count
		 FROM provider_drifts
		 WHERE (resolved_at IS NULL OR resolved_at = '')
		   AND first_seen_at < {:cutoff}`,
	).Bind(map[string]interface{}{"cutoff": cutoff}).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]Contradiction, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contradiction{
			TargetID: r.Target,
			Detail: fmt.Sprintf("provider_drift %s for provider=%s has been open since %s (%d observations) — exceeds %dh escalation threshold",
				r.ID, r.Provider, r.FirstSeenAt, r.DriftCount, driftEscalationHours),
		})
	}
	return out, nil
}

// Phase 3.9.7 — Repeated ambiguous observations check.
// Targets with ≥10 consecutive ambiguous/stale observations have likely
// lost connectivity to the provider or the provider is in a degraded state.
func checkRepeatedAmbiguousObservations(app *pb.PocketBase) ([]Contradiction, error) {
	const ambiguousThreshold = 10
	// Find targets where the last N observations are all ambiguous or stale
	// (no authoritative or inferred observation in the most recent window).
	// We approximate by checking COUNT of recent non-authoritative obs vs total.
	var rows []struct {
		Target       string `db:"target"`
		AmbiguousAmt int    `db:"ambiguous_count"`
	}
	cutoff := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
	err := app.Dao().DB().NewQuery(
		`SELECT target, COUNT(*) AS ambiguous_count
		 FROM provider_observations
		 WHERE observed_at > {:cutoff}
		   AND observation_trust IN ('ambiguous', 'stale')
		   AND target NOT IN (
		     SELECT DISTINCT target FROM provider_observations
		     WHERE observed_at > {:cutoff}
		       AND observation_trust IN ('authoritative', 'inferred')
		   )
		 GROUP BY target
		 HAVING COUNT(*) >= {:threshold}`,
	).Bind(map[string]interface{}{
		"cutoff":    cutoff,
		"threshold": ambiguousThreshold,
	}).All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]Contradiction, 0, len(rows))
	for _, r := range rows {
		out = append(out, Contradiction{
			TargetID: r.Target,
			Detail: fmt.Sprintf("target has %d ambiguous/stale observations in the last 30 minutes with no authoritative observations — provider may be unreachable",
				r.AmbiguousAmt),
		})
	}
	return out, nil
}
