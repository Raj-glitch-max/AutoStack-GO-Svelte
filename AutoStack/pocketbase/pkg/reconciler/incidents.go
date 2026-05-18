package reconciler

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
	pbmodels "github.com/pocketbase/pocketbase/models"
)

// Phase 3.9.4 — Incident first-class persistence.
//
// This file implements the Incident state machine and persistence layer.
// Incidents are operational objects with their own lifecycle, separate from
// operations and deployment_history. They are NEVER deleted before their
// retention TTL elapses, and unresolved incidents are immortal.
//
// ─────────────────────────────────────────────────────────────────────────
// SCOPE DISTINCTION (existing code)
// ─────────────────────────────────────────────────────────────────────────
//
//   IncidentTimeline (pkg/reconciler/incident.go) — READ MODEL.
//     A windowed reconstruction of operations + history for a target.
//     Computed on demand. NOT persisted.
//
//   Incident (this file) — PERSISTED RECORD.
//     A first-class operational object in the `incidents` collection.
//     Created when runtime failure / escalation occurs. Survives until
//     resolved + retention TTL elapses.
//
// ─────────────────────────────────────────────────────────────────────────
// HONEST CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. Incident creation is best-effort. If the DB write fails, the
//      runtime logs but does NOT abort the operation. Operationally, a
//      lost incident is a forensic gap, not a correctness failure.
//   2. The state machine is in-Go, validated on every transition.
//      Direct DB edits bypassing UpdateStatus() can violate transitions
//      but cannot violate the retention safety (cleanup.go enforces
//      separately).
//   3. Deduplication: at most ONE non-resolved incident per (target, source)
//      pair at a time. New occurrences attach to the existing incident
//      (escalation_count++) rather than creating a duplicate.
//   4. Retention TTL: resolved incidents are eligible for deletion N days
//      after resolved_at (default 90d, configurable). Unresolved incidents
//      are immortal until resolved.

// ─────────────────────────────────────────────────────────────────────────
// CONSTANTS — CANONICAL VALUES
// ─────────────────────────────────────────────────────────────────────────

// IncidentStatus is the state-machine position. Persisted as `incidents.status`.
type IncidentStatus string

const (
	IncidentStatusOpen         IncidentStatus = "open"
	IncidentStatusEscalating   IncidentStatus = "escalating"
	IncidentStatusAcknowledged IncidentStatus = "acknowledged"
	IncidentStatusResolved     IncidentStatus = "resolved"
	IncidentStatusReopened     IncidentStatus = "reopened"
	IncidentStatusSuppressed   IncidentStatus = "suppressed"
)

// IncidentSeverity classifies operational impact.
type IncidentSeverity string

const (
	IncidentSeverityLow      IncidentSeverity = "low"
	IncidentSeverityMedium   IncidentSeverity = "medium"
	IncidentSeverityHigh     IncidentSeverity = "high"
	IncidentSeverityCritical IncidentSeverity = "critical"
)

// IncidentSource is the canonical reason for incident creation.
// Persisted as `incidents.source`. Stable strings; renaming requires migration.
type IncidentSource string

const (
	IncidentSourceRetryExhausted          IncidentSource = "retry-exhausted"
	IncidentSourceAmbiguityEscalation     IncidentSource = "ambiguity-escalation"
	IncidentSourceLeaseLossMidCall        IncidentSource = "lease-loss-mid-call"
	IncidentSourceInterruptedUnknown      IncidentSource = "interrupted-unknown-outcome"
	IncidentSourceRollbackFailed          IncidentSource = "rollback-failed"
	IncidentSourceReclaimAfterCrash       IncidentSource = "reclaim-after-crash"
	IncidentSourceInvariantViolation      IncidentSource = "invariant-violation" // Phase 3.9.5 reserve

	// Phase 3.9.7 — provider truth incidents.
	// IncidentSourceProviderContradiction fires when provider state
	// explicitly contradicts runtime expectation.
	IncidentSourceProviderContradiction IncidentSource = "provider-contradiction"
	// IncidentSourceProviderUnknowable fires when provider state cannot
	// be determined after repeated ambiguous observations.
	IncidentSourceProviderUnknowable IncidentSource = "provider-unknowable"
	// IncidentSourceDriftPersistent fires when a provider drift record
	// has been open beyond the escalation threshold.
	IncidentSourceDriftPersistent IncidentSource = "drift-persistent"

	// Phase 3.9.9 — provider silence incident.
	// IncidentSourceProviderSilence fires when no authoritative or inferred
	// provider observation has been received for an active target within the
	// silence detection window. Silence is distinct from ambiguous observations
	// (the provider responds but returns unclear results) — silence means the
	// runtime has stopped hearing from the provider entirely. Classification is
	// separate from contradiction detection and must not be conflated with
	// provider failure.
	IncidentSourceProviderSilence IncidentSource = "provider-silence"
)

// ─────────────────────────────────────────────────────────────────────────
// STATE MACHINE
// ─────────────────────────────────────────────────────────────────────────

// incidentTransitions is the authoritative allowed-transition map.
// A transition not listed is FORBIDDEN; UpdateIncidentStatus refuses it.
//
// Self-transitions (no-op) are always permitted implicitly.
var incidentTransitions = map[IncidentStatus][]IncidentStatus{
	IncidentStatusOpen: {
		IncidentStatusEscalating,
		IncidentStatusAcknowledged,
		IncidentStatusResolved,
		IncidentStatusSuppressed,
	},
	IncidentStatusEscalating: {
		IncidentStatusAcknowledged,
		IncidentStatusResolved,
		IncidentStatusEscalating, // re-escalation increments count
	},
	IncidentStatusAcknowledged: {
		IncidentStatusResolved,
		IncidentStatusEscalating, // operator acked, then runtime escalated again
	},
	IncidentStatusResolved: {
		IncidentStatusReopened,
	},
	IncidentStatusReopened: {
		IncidentStatusEscalating,
		IncidentStatusAcknowledged,
		IncidentStatusResolved,
	},
	IncidentStatusSuppressed: {
		IncidentStatusOpen,     // operator unsuppressed
		IncidentStatusReopened, // operator-explicit reopen
	},
}

// IsAllowedIncidentTransition reports whether from→to is permitted.
// Implicit self-transitions return true. Unknown from/to returns false.
func IsAllowedIncidentTransition(from, to IncidentStatus) bool {
	if from == to {
		return true
	}
	allowed, ok := incidentTransitions[from]
	if !ok {
		return false
	}
	for _, t := range allowed {
		if t == to {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────
// CREATION + ESCALATION API
// ─────────────────────────────────────────────────────────────────────────

// CreateOrEscalateIncident creates a new incident OR escalates an existing
// non-resolved incident for (target, source). Deduplication: while an
// incident is open/escalating/acknowledged/reopened/suppressed for the
// same (target, source), new failures attach to it as escalations rather
// than creating a parallel incident.
//
// Returns the incident ID and a bool indicating whether the incident was
// newly created (true) or escalated (false). Best-effort: returns ("", false)
// on DB error.
//
// The `runtimeState` argument captures the runtime context snapshot at
// creation time (retry_class, retry_count, ambiguity_source, lease_holder,
// etc.) so operators can reconstruct "what did the runtime believe when
// this fired" without log archaeology.
type IncidentRuntimeState struct {
	RetryClass        string `json:"retry_class,omitempty"`
	RetryCount        int    `json:"retry_count,omitempty"`
	AmbiguitySource   string `json:"ambiguity_source,omitempty"`
	AmbiguityNative   string `json:"ambiguity_native_state,omitempty"`
	LeaseHolder       string `json:"lease_holder,omitempty"`
	LeaseEpoch        int    `json:"lease_epoch,omitempty"`
	LeaseValid        bool   `json:"lease_valid,omitempty"`
	NativeProviderCode string `json:"native_provider_code,omitempty"`
}

func (r *Reconciler) CreateOrEscalateIncident(targetID, rolloutID string, source IncidentSource, severity IncidentSeverity, operationID, forensicSummary string, runtimeState IncidentRuntimeState) (string, bool) {
	if targetID == "" || source == "" {
		return "", false
	}

	// Deduplication: find an existing non-resolved incident for this (target, source).
	existing, err := r.findActiveIncident(targetID, source)
	if err != nil {
		log.Printf("[INCIDENT_LOOKUP_ERR] target=%s source=%s err=%v", targetID, source, err)
		return "", false
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if existing != nil {
		// Escalation path: bump count + update last_seen_at + transition
		// status if needed.
		newCount := existing.GetInt("escalation_count") + 1
		existing.Set("escalation_count", newCount)
		existing.Set("last_seen_at", now)
		existing.Set("last_operation_id", operationID)
		existing.Set("forensic_summary", forensicSummary)

		// Severity upgrade: only increase, never downgrade.
		currentSev := IncidentSeverity(existing.GetString("severity"))
		if severityRank(severity) > severityRank(currentSev) {
			existing.Set("severity", string(severity))
		}

		// Status transition: open/acknowledged → escalating on re-occurrence.
		// resolved → reopened would require operator explicit reopen, so
		// we DO NOT auto-reopen a resolved incident here. Instead, a NEW
		// incident is created (a previously-resolved incident stays resolved
		// for forensic continuity; the new incident is the current problem).
		currentStatus := IncidentStatus(existing.GetString("status"))
		if currentStatus == IncidentStatusResolved {
			// Resolved incident — create a fresh one instead of resurrecting.
			return r.createFreshIncident(targetID, rolloutID, source, severity, operationID, forensicSummary, runtimeState, now)
		}
		// Transition to escalating if not already terminal.
		if currentStatus != IncidentStatusEscalating &&
			IsAllowedIncidentTransition(currentStatus, IncidentStatusEscalating) {
			existing.Set("status", string(IncidentStatusEscalating))
		}

		if err := r.app.Dao().SaveRecord(existing); err != nil {
			log.Printf("[INCIDENT_ESCALATE_ERR] id=%s err=%v", existing.Id, err)
			return existing.Id, false
		}
		if r.metrics != nil {
			r.metrics.IncIncidentEscalation()
		}
		log.Printf("[INCIDENT_ESCALATED] id=%s target=%s source=%s severity=%s count=%d",
			existing.Id, targetID, source, existing.GetString("severity"), newCount)
		return existing.Id, false
	}

	return r.createFreshIncident(targetID, rolloutID, source, severity, operationID, forensicSummary, runtimeState, now)
}

func (r *Reconciler) createFreshIncident(targetID, rolloutID string, source IncidentSource, severity IncidentSeverity, operationID, forensicSummary string, runtimeState IncidentRuntimeState, now string) (string, bool) {
	col, err := r.app.Dao().FindCollectionByNameOrId("incidents")
	if err != nil {
		log.Printf("[INCIDENT_COLLECTION_ERR] err=%v", err)
		return "", false
	}
	rec := pbmodels.NewRecord(col)
	rec.Set("target", targetID)
	rec.Set("rollout", rolloutID)
	rec.Set("source", string(source))
	rec.Set("severity", string(severity))
	rec.Set("status", string(IncidentStatusOpen))
	rec.Set("escalation_count", 0)
	rec.Set("first_seen_at", now)
	rec.Set("last_seen_at", now)
	rec.Set("first_operation_id", operationID)
	rec.Set("last_operation_id", operationID)
	rec.Set("forensic_summary", forensicSummary)

	// Serialize the runtime state snapshots as JSON for forensic reconstruction.
	if runtimeState.RetryClass != "" || runtimeState.RetryCount > 0 {
		if b, err := json.Marshal(map[string]interface{}{
			"class": runtimeState.RetryClass,
			"count": runtimeState.RetryCount,
			"native_code": runtimeState.NativeProviderCode,
		}); err == nil {
			rec.Set("retry_state", string(b))
		}
	}
	if runtimeState.AmbiguitySource != "" {
		if b, err := json.Marshal(map[string]interface{}{
			"source":      runtimeState.AmbiguitySource,
			"native_state": runtimeState.AmbiguityNative,
		}); err == nil {
			rec.Set("ambiguity_state", string(b))
		}
	}
	if runtimeState.LeaseHolder != "" {
		if b, err := json.Marshal(map[string]interface{}{
			"holder": runtimeState.LeaseHolder,
			"epoch":  runtimeState.LeaseEpoch,
			"valid":  runtimeState.LeaseValid,
		}); err == nil {
			rec.Set("lease_state", string(b))
		}
	}

	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[INCIDENT_CREATE_ERR] target=%s source=%s err=%v", targetID, source, err)
		return "", false
	}
	if r.metrics != nil {
		r.metrics.IncIncidentOpened(string(source))
	}
	log.Printf("[INCIDENT_OPENED] id=%s target=%s source=%s severity=%s op=%s",
		rec.Id, targetID, source, severity, operationID)
	return rec.Id, true
}

// findActiveIncident returns the most-recently-updated non-resolved incident
// for (target, source), or nil if none exists. Returns error only on DB failure.
//
// "Active" = status NOT IN (resolved). Suppressed incidents are deduplicated
// against (they exist and the operator chose to suppress; new failures
// should escalate the existing record, not spawn a duplicate).
func (r *Reconciler) findActiveIncident(targetID string, source IncidentSource) (*pbmodels.Record, error) {
	records, err := r.app.Dao().FindRecordsByExpr("incidents",
		dbx.NewExp("target = {:t} AND source = {:s} AND status != 'resolved'",
			dbx.Params{"t": targetID, "s": string(source)}))
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	// Most recent by last_seen_at.
	latest := records[0]
	for _, r := range records[1:] {
		if r.GetString("last_seen_at") > latest.GetString("last_seen_at") {
			latest = r
		}
	}
	return latest, nil
}

// UpdateIncidentStatus transitions an incident to a new status, validated
// against the state-machine table. Refuses forbidden transitions.
//
// For status=resolved: reason MUST be non-empty (truthfulness rule).
// For status=acknowledged: actor MUST be non-empty (audit trail).
func (r *Reconciler) UpdateIncidentStatus(incidentID string, newStatus IncidentStatus, reason, actor string) error {
	if incidentID == "" {
		return fmt.Errorf("incidentID required")
	}
	rec, err := r.app.Dao().FindRecordById("incidents", incidentID)
	if err != nil {
		return fmt.Errorf("incident not found: %w", err)
	}
	currentStatus := IncidentStatus(rec.GetString("status"))
	if !IsAllowedIncidentTransition(currentStatus, newStatus) {
		return fmt.Errorf("forbidden incident transition: %s → %s", currentStatus, newStatus)
	}
	if newStatus == IncidentStatusResolved && reason == "" {
		return fmt.Errorf("resolved status requires reason (truthfulness rule)")
	}
	if newStatus == IncidentStatusAcknowledged && actor == "" {
		return fmt.Errorf("acknowledged status requires actor (audit trail)")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	rec.Set("status", string(newStatus))
	rec.Set("last_seen_at", now)
	if newStatus == IncidentStatusResolved {
		rec.Set("resolved_at", now)
		rec.Set("resolution_reason", reason)
	}
	if newStatus == IncidentStatusAcknowledged {
		rec.Set("acknowledged_by", actor)
	}
	if newStatus == IncidentStatusReopened {
		// Reopening clears resolved_at so the retention timer restarts on
		// the next resolution. The original first_seen_at is preserved.
		rec.Set("resolved_at", nil)
		rec.Set("resolution_reason", "")
	}

	if err := r.app.Dao().SaveRecord(rec); err != nil {
		return fmt.Errorf("save incident: %w", err)
	}
	if r.metrics != nil {
		r.metrics.IncIncidentTransition(string(newStatus))
	}
	log.Printf("[INCIDENT_TRANSITION] id=%s from=%s to=%s actor=%s reason=%s",
		incidentID, currentStatus, newStatus, actor, reason)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────
// RETENTION SAFETY HELPERS (used by cleanup.go)
// ─────────────────────────────────────────────────────────────────────────

// IsOperationReferencedByIncident returns true if any non-resolved incident
// references the operation as first_operation_id OR last_operation_id.
// The retention sweep MUST skip operations that return true here —
// deleting them would erase forensic evidence of an active incident.
func IsOperationReferencedByIncident(app *pb.PocketBase, operationID string) bool {
	if operationID == "" {
		return false
	}
	var rows []struct {
		ID string `db:"id"`
	}
	err := app.Dao().DB().
		Select("id").
		From("incidents").
		Where(dbx.NewExp(
			"status != 'resolved' AND (first_operation_id = {:op} OR last_operation_id = {:op})",
			dbx.Params{"op": operationID})).
		Limit(1).
		All(&rows)
	if err != nil {
		log.Printf("[INCIDENT_REF_LOOKUP_ERR] op=%s err=%v", operationID, err)
		// Conservative: on DB error, treat as referenced. Better to leak
		// retention than to lose forensic evidence.
		return true
	}
	return len(rows) > 0
}

// PurgeResolvedIncidents deletes resolved incidents whose resolved_at is
// older than the given retention window. Unresolved incidents are NEVER
// affected. Returns the number of rows deleted.
//
// Best-effort: errors are logged but do not propagate. Retention is a
// hygiene concern, not a correctness concern.
func PurgeResolvedIncidents(app *pb.PocketBase, retention time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-retention).Format("2006-01-02 15:04:05")
	res, err := app.Dao().DB().NewQuery(
		`DELETE FROM incidents
		 WHERE status = 'resolved'
		   AND resolved_at IS NOT NULL
		   AND resolved_at <> ''
		   AND resolved_at < {:cutoff}`,
	).Bind(dbx.Params{"cutoff": cutoff}).Execute()
	if err != nil {
		return 0, fmt.Errorf("purge resolved incidents: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ─────────────────────────────────────────────────────────────────────────
// READ API (for HTTP endpoints)
// ─────────────────────────────────────────────────────────────────────────

// IncidentRecord is the JSON-friendly shape returned by query endpoints.
// Distinct from the DB record so we can shape the response without
// exposing PocketBase internals.
type IncidentRecord struct {
	ID                string `json:"id"`
	Target            string `json:"target"`
	Rollout           string `json:"rollout"`
	Source            string `json:"source"`
	Severity          string `json:"severity"`
	Status            string `json:"status"`
	EscalationCount   int    `json:"escalation_count"`
	FirstSeenAt       string `json:"first_seen_at"`
	LastSeenAt        string `json:"last_seen_at"`
	ResolvedAt        string `json:"resolved_at,omitempty"`
	ResolutionReason  string `json:"resolution_reason,omitempty"`
	AcknowledgedBy    string `json:"acknowledged_by,omitempty"`
	FirstOperationID  string `json:"first_operation_id,omitempty"`
	LastOperationID   string `json:"last_operation_id,omitempty"`
	ForensicSummary   string `json:"forensic_summary"`
	AmbiguityState    string `json:"ambiguity_state,omitempty"`
	RetryState        string `json:"retry_state,omitempty"`
	LeaseState        string `json:"lease_state,omitempty"`
}

// ListIncidents returns recent incidents, newest first, up to limit.
// Filter by status if non-empty; "all" or "" returns all statuses.
func ListIncidents(app *pb.PocketBase, statusFilter string, limit int) ([]IncidentRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := app.Dao().DB().
		Select("id", "target", "rollout", "source", "severity", "status",
			"escalation_count", "first_seen_at", "last_seen_at", "resolved_at",
			"resolution_reason", "acknowledged_by", "first_operation_id",
			"last_operation_id", "forensic_summary", "ambiguity_state",
			"retry_state", "lease_state").
		From("incidents")

	if statusFilter != "" && statusFilter != "all" {
		query = query.Where(dbx.NewExp("status = {:s}", dbx.Params{"s": statusFilter}))
	}

	var rows []struct {
		ID               string `db:"id"`
		Target           string `db:"target"`
		Rollout          string `db:"rollout"`
		Source           string `db:"source"`
		Severity         string `db:"severity"`
		Status           string `db:"status"`
		EscalationCount  int    `db:"escalation_count"`
		FirstSeenAt      string `db:"first_seen_at"`
		LastSeenAt       string `db:"last_seen_at"`
		ResolvedAt       string `db:"resolved_at"`
		ResolutionReason string `db:"resolution_reason"`
		AcknowledgedBy   string `db:"acknowledged_by"`
		FirstOperationID string `db:"first_operation_id"`
		LastOperationID  string `db:"last_operation_id"`
		ForensicSummary  string `db:"forensic_summary"`
		AmbiguityState   string `db:"ambiguity_state"`
		RetryState       string `db:"retry_state"`
		LeaseState       string `db:"lease_state"`
	}
	err := query.
		OrderBy("last_seen_at DESC").
		Limit(int64(limit)).
		All(&rows)
	if err != nil {
		return nil, err
	}
	result := make([]IncidentRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, IncidentRecord{
			ID:               row.ID,
			Target:           row.Target,
			Rollout:          row.Rollout,
			Source:           row.Source,
			Severity:         row.Severity,
			Status:           row.Status,
			EscalationCount:  row.EscalationCount,
			FirstSeenAt:      row.FirstSeenAt,
			LastSeenAt:       row.LastSeenAt,
			ResolvedAt:       row.ResolvedAt,
			ResolutionReason: row.ResolutionReason,
			AcknowledgedBy:   row.AcknowledgedBy,
			FirstOperationID: row.FirstOperationID,
			LastOperationID:  row.LastOperationID,
			ForensicSummary:  row.ForensicSummary,
			AmbiguityState:   row.AmbiguityState,
			RetryState:       row.RetryState,
			LeaseState:       row.LeaseState,
		})
	}
	return result, nil
}

// GetIncident returns a single incident by ID.
func GetIncident(app *pb.PocketBase, incidentID string) (*IncidentRecord, error) {
	rec, err := app.Dao().FindRecordById("incidents", incidentID)
	if err != nil {
		return nil, err
	}
	return &IncidentRecord{
		ID:               rec.Id,
		Target:           rec.GetString("target"),
		Rollout:          rec.GetString("rollout"),
		Source:           rec.GetString("source"),
		Severity:         rec.GetString("severity"),
		Status:           rec.GetString("status"),
		EscalationCount:  rec.GetInt("escalation_count"),
		FirstSeenAt:      rec.GetString("first_seen_at"),
		LastSeenAt:       rec.GetString("last_seen_at"),
		ResolvedAt:       rec.GetString("resolved_at"),
		ResolutionReason: rec.GetString("resolution_reason"),
		AcknowledgedBy:   rec.GetString("acknowledged_by"),
		FirstOperationID: rec.GetString("first_operation_id"),
		LastOperationID:  rec.GetString("last_operation_id"),
		ForensicSummary:  rec.GetString("forensic_summary"),
		AmbiguityState:   rec.GetString("ambiguity_state"),
		RetryState:       rec.GetString("retry_state"),
		LeaseState:       rec.GetString("lease_state"),
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────

func severityRank(s IncidentSeverity) int {
	switch s {
	case IncidentSeverityLow:
		return 1
	case IncidentSeverityMedium:
		return 2
	case IncidentSeverityHigh:
		return 3
	case IncidentSeverityCritical:
		return 4
	default:
		return 0
	}
}

// severityForRetryClass maps the canonical retry class to an initial
// incident severity. Operators can manually override via UI later.
func severityForRetryClass(class providers.RetryClass) IncidentSeverity {
	switch class {
	case providers.RetryClassAuth, providers.RetryClassQuota:
		return IncidentSeverityHigh // operator action required
	case providers.RetryClassProviderUncertain:
		return IncidentSeverityHigh // outcome unknown
	case providers.RetryClassInternal, providers.RetryClassUnknown:
		return IncidentSeverityCritical // bug or unclassified
	case providers.RetryClassNever:
		return IncidentSeverityMedium // spec invalid; operator action
	default:
		return IncidentSeverityMedium
	}
}
