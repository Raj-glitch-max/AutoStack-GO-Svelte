package reconciler

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/janlauber/one-click/pkg/providers"
	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
	pbmodels "github.com/pocketbase/pocketbase/models"
)

// intToStr is a small helper for HumanSummary formatting without pulling in fmt.
func intToStr(n int) string { return strconv.Itoa(n) }

// Phase 3.9.6 — Runtime Decision persistence.
//
// Every meaningful runtime decision flows through RecordDecision so an
// operator can reconstruct WHY the runtime acted from persisted DB state
// alone. This file defines the canonical taxonomy + the single persistence
// entry point. Read paths (narrative, contradictions) live in separate files.
//
// ─────────────────────────────────────────────────────────────────────────
// HONEST CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. Decisions are APPEND-ONLY. RecordDecision never updates an existing
//      row. Updating would rewrite operational history.
//
//   2. RecordDecision is BEST-EFFORT. DB errors are logged; they do NOT
//      propagate. A lost decision is a forensic gap, not a correctness
//      failure — the underlying runtime action still occurred.
//
//   3. Suppression dedupe is APPROXIMATE. We skip writing the same
//      (target, decision_type, decision_reason) within a 60s window to
//      avoid table flooding on persistent-skip scenarios (e.g., cooldown
//      ticking every 30s). The dedupe is in-memory; it does NOT survive
//      restart — that's correct, because post-restart the operator wants
//      to see the first observation of an ongoing suppression.
//
//   4. policy_name + policy_version are SNAPSHOTS taken at decision time.
//      They do NOT reference a registry table. When Phase 4 introduces
//      tunable policies, the registry can be backfilled against these
//      strings without breaking the schema.

// ─────────────────────────────────────────────────────────────────────────
// POLICY VERSIONING
// ─────────────────────────────────────────────────────────────────────────

// PolicyVersion is the semver-style version of the decision-making semantics
// in this build. Bump when policy SEMANTICS change (retry budgets, lease
// duration, escalation thresholds), not for cosmetic edits.
const PolicyVersion = "3.9.7.0"

// Policy name constants — passed to RecordDecision so operators can filter
// decisions by which subsystem made them.
const (
	PolicyNameRetryEngine     = "retry_engine"
	PolicyNameLifecycleGuard  = "lifecycle_guard"
	PolicyNameLeaseEngine     = "lease_engine"
	PolicyNameInvariantAudit  = "invariant_audit"
	PolicyNameCircuitBreaker  = "circuit_breaker"
	PolicyNameStartupAudit    = "startup_audit"
	PolicyNameContradiction   = "contradiction_audit"
	PolicyNameObservationGuard = "observation_guard"
	PolicyNameSpecDiff         = "spec_diff"
)

// ─────────────────────────────────────────────────────────────────────────
// DECISION TAXONOMY
// ─────────────────────────────────────────────────────────────────────────

// DecisionType is the canonical taxonomy of runtime decisions.
// String values are PERSISTED. Renaming requires a migration.
type DecisionType string

const (
	// — Retry engine (6 — one per applyRetryAction arm + DecideRetry branches)
	DecisionRetryScheduled           DecisionType = "retry_scheduled"
	DecisionRetryExhausted           DecisionType = "retry_exhausted"
	DecisionRetryRefusedNonRetryable DecisionType = "retry_refused_non_retryable"
	DecisionRetryRefusedLeaseInvalid DecisionType = "retry_refused_lease_invalid"
	DecisionRetryEscalatedAmbiguity  DecisionType = "retry_escalated_ambiguity"
	DecisionRetryOperatorAction      DecisionType = "retry_operator_action"

	// — Lease lifecycle (3 — Phase 3.9.2 abort sites)
	DecisionLeaseLostPreCall   DecisionType = "lease_lost_pre_call"
	DecisionLeaseLostMidCall   DecisionType = "lease_lost_mid_call"
	DecisionLeaseAcquireDenied DecisionType = "lease_acquire_denied"

	// — Reconcile suppressions (3)
	DecisionCircuitBreakerOpen  DecisionType = "circuit_breaker_open"
	DecisionCooldownActive      DecisionType = "cooldown_active"
	DecisionRetryBackoffWindow  DecisionType = "retry_backoff_window"

	// — Guard holds (1 — subtype in decision_reason)
	DecisionObservationHeld DecisionType = "observation_held"

	// — Refusals / aborts (2)
	DecisionStartupRefusedFatal DecisionType = "startup_refused_fatal"
	DecisionDispatchClaimLost   DecisionType = "dispatch_claim_lost"

	// — Contradiction (1 — Phase 3.9.6 cross-table consistency findings)
	DecisionContradictionDetected DecisionType = "contradiction_detected"

	// — Provider truth (4 — Phase 3.9.7 provider observation forensics)
	DecisionProviderContradiction  DecisionType = "provider_contradiction"
	DecisionProviderUnknowable     DecisionType = "provider_unknowable"
	DecisionDriftOpened            DecisionType = "drift_opened"
	DecisionDriftResolved          DecisionType = "drift_resolved"

	// — Spec drift (1 — Phase 3.10 semantic DeploySpec change detection)
	DecisionDeploySpecDriftDetected DecisionType = "deployspec_drift_detected"
)

// AllDecisionTypes — registry for completeness checks + metric labels.
var AllDecisionTypes = []DecisionType{
	DecisionRetryScheduled,
	DecisionRetryExhausted,
	DecisionRetryRefusedNonRetryable,
	DecisionRetryRefusedLeaseInvalid,
	DecisionRetryEscalatedAmbiguity,
	DecisionRetryOperatorAction,
	DecisionLeaseLostPreCall,
	DecisionLeaseLostMidCall,
	DecisionLeaseAcquireDenied,
	DecisionCircuitBreakerOpen,
	DecisionCooldownActive,
	DecisionRetryBackoffWindow,
	DecisionObservationHeld,
	DecisionStartupRefusedFatal,
	DecisionDispatchClaimLost,
	DecisionContradictionDetected,
	// Phase 3.9.7 — provider truth
	DecisionProviderContradiction,
	DecisionProviderUnknowable,
	DecisionDriftOpened,
	DecisionDriftResolved,
	// Phase 3.10 — spec drift
	DecisionDeploySpecDriftDetected,
}

// ─────────────────────────────────────────────────────────────────────────
// CONFIDENCE MODEL
// ─────────────────────────────────────────────────────────────────────────

// ConfidenceLevel — operational certainty classification. Not AI probability.
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

// ConfidenceSource — why this confidence level was chosen.
type ConfidenceSource string

const (
	ConfidenceSourceDeterministicPolicy ConfidenceSource = "deterministic_policy"
	ConfidenceSourceProviderClassified  ConfidenceSource = "provider_classified"
	ConfidenceSourceProviderAmbiguous   ConfidenceSource = "provider_ambiguous"
	ConfidenceSourceLeaseAuthoritative  ConfidenceSource = "lease_authoritative"
	ConfidenceSourceLeaseInferred       ConfidenceSource = "lease_inferred"
	ConfidenceSourceClassifierFallback  ConfidenceSource = "classifier_fallback"
	ConfidenceSourceInvariantAudit      ConfidenceSource = "invariant_audit"
)

// confidenceFor is a pure function: given the decision type and the
// classifier output (if any) + lease validity, return the canonical
// (ConfidenceLevel, ConfidenceSource) pair.
//
// Unit-testable without a DB. The mapping is deterministic; same inputs
// always produce same outputs.
func confidenceFor(dt DecisionType, classDecision *providers.RetryDecision, leaseValid bool) (ConfidenceLevel, ConfidenceSource) {
	switch dt {
	case DecisionRetryRefusedLeaseInvalid, DecisionLeaseLostPreCall, DecisionLeaseAcquireDenied:
		return ConfidenceHigh, ConfidenceSourceLeaseAuthoritative

	case DecisionLeaseLostMidCall:
		// Outcome unknown: provider mutation may have committed before lease loss.
		return ConfidenceLow, ConfidenceSourceLeaseInferred

	case DecisionRetryEscalatedAmbiguity:
		return ConfidenceMedium, ConfidenceSourceProviderAmbiguous

	case DecisionRetryScheduled, DecisionRetryExhausted,
		DecisionRetryRefusedNonRetryable, DecisionRetryOperatorAction:
		if classDecision == nil {
			return ConfidenceLow, ConfidenceSourceClassifierFallback
		}
		switch classDecision.Class {
		case providers.RetryClassUnknown:
			return ConfidenceLow, ConfidenceSourceClassifierFallback
		case providers.RetryClassProviderUncertain:
			return ConfidenceMedium, ConfidenceSourceProviderAmbiguous
		default:
			return ConfidenceHigh, ConfidenceSourceProviderClassified
		}

	case DecisionCircuitBreakerOpen, DecisionCooldownActive,
		DecisionRetryBackoffWindow, DecisionStartupRefusedFatal,
		DecisionDispatchClaimLost:
		return ConfidenceHigh, ConfidenceSourceDeterministicPolicy

	case DecisionObservationHeld:
		// Guard logic is deterministic but the underlying provider observation
		// may itself be wrong (that's why the guard exists).
		return ConfidenceMedium, ConfidenceSourceDeterministicPolicy

	case DecisionContradictionDetected:
		return ConfidenceHigh, ConfidenceSourceInvariantAudit

	// Phase 3.9.7 — provider truth decisions.
	case DecisionProviderContradiction:
		// Contradiction evidence is authoritative about the EXISTENCE of divergence;
		// it is not authoritative about which side is wrong.
		return ConfidenceMedium, ConfidenceSourceProviderAmbiguous
	case DecisionProviderUnknowable:
		// Provider state cannot be determined; runtime is operating blind.
		return ConfidenceLow, ConfidenceSourceClassifierFallback
	case DecisionDriftOpened, DecisionDriftResolved:
		// Drift lifecycle events are deterministic policy decisions.
		return ConfidenceHigh, ConfidenceSourceDeterministicPolicy
	}
	return ConfidenceLow, ConfidenceSourceClassifierFallback
}

// ─────────────────────────────────────────────────────────────────────────
// EVIDENCE REFERENCES
// ─────────────────────────────────────────────────────────────────────────

// EvidenceRef points to a related row in another collection.
// Serialized to evidence_refs JSON column.
type EvidenceRef struct {
	Collection string `json:"collection"`
	ID         string `json:"id"`
}

// ─────────────────────────────────────────────────────────────────────────
// RECORD API
// ─────────────────────────────────────────────────────────────────────────

// DecisionRecord is the input to RecordDecision. Fields not provided
// receive sensible defaults; confidence is computed from DecisionType +
// ClassDecision + LeaseValid via confidenceFor.
type DecisionRecord struct {
	TargetID       string
	RolloutID      string
	OperationID    string
	CycleID        string
	DecisionType   DecisionType
	DecisionReason string
	DecisionInputs map[string]interface{}
	EvidenceRefs   []EvidenceRef
	HumanSummary   string
	PolicyName     string
	ClassDecision  *providers.RetryDecision // optional; informs confidence
	LeaseValid     bool                     // optional; informs confidence
}

// ─────────────────────────────────────────────────────────────────────────
// SUPPRESSION DEDUPE
// ─────────────────────────────────────────────────────────────────────────

// decisionDedupeWindow is how long to suppress writing the same
// (target, decision_type, decision_reason) tuple. Prevents table flooding
// when a target is persistently in a held state (e.g., cooldown active
// across every tick for several minutes).
const decisionDedupeWindow = 60 * time.Second

var (
	decisionDedupeMu    sync.Mutex
	decisionDedupeCache = make(map[string]time.Time)
)

// shouldDedupeDecision returns true if the same (target+type+reason) was
// recorded within decisionDedupeWindow. Side-effect: updates the cache.
func shouldDedupeDecision(targetID string, dt DecisionType, reason string) bool {
	key := targetID + "|" + string(dt) + "|" + reason
	decisionDedupeMu.Lock()
	defer decisionDedupeMu.Unlock()
	if last, ok := decisionDedupeCache[key]; ok && time.Since(last) < decisionDedupeWindow {
		return true
	}
	decisionDedupeCache[key] = time.Now()
	// Best-effort cache prune: remove entries older than 2x window.
	if len(decisionDedupeCache) > 1000 {
		threshold := time.Now().Add(-2 * decisionDedupeWindow)
		for k, t := range decisionDedupeCache {
			if t.Before(threshold) {
				delete(decisionDedupeCache, k)
			}
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────
// PERSISTENCE
// ─────────────────────────────────────────────────────────────────────────

// RecordDecision persists a runtime decision. Best-effort: errors are
// logged but do not propagate.
//
// Suppression dedupe applies to a small subset of decision types that fire
// on every tick during a persistent state (cooldown, retry-backoff-window,
// observation-held). Other decision types (retry scheduled/exhausted,
// lease aborts, contradictions, startup refusal) always write.
//
// Returns the new row ID, or empty string on failure.
func (r *Reconciler) RecordDecision(d DecisionRecord) string {
	if d.TargetID == "" || d.DecisionType == "" {
		return ""
	}
	// Dedupe noisy suppression decisions.
	if shouldDedupeSuppressionType(d.DecisionType) {
		if shouldDedupeDecision(d.TargetID, d.DecisionType, d.DecisionReason) {
			return ""
		}
	}

	confidence, confSource := confidenceFor(d.DecisionType, d.ClassDecision, d.LeaseValid)

	col, err := r.app.Dao().FindCollectionByNameOrId("runtime_decisions")
	if err != nil {
		log.Printf("[DECISION_COLLECTION_ERR] err=%v", err)
		return ""
	}
	rec := pbmodels.NewRecord(col)
	rec.Set("target", d.TargetID)
	if d.RolloutID != "" {
		rec.Set("rollout", d.RolloutID)
	}
	if d.OperationID != "" {
		rec.Set("operation_id", d.OperationID)
	}
	if d.CycleID != "" {
		rec.Set("reconcile_cycle_id", d.CycleID)
	}
	rec.Set("pod_id", string(currentPodIdentity()))
	rec.Set("decision_type", string(d.DecisionType))
	rec.Set("decision_reason", d.DecisionReason)

	if d.DecisionInputs != nil {
		if b, err := json.Marshal(d.DecisionInputs); err == nil {
			rec.Set("decision_inputs", string(b))
		}
	}
	if len(d.EvidenceRefs) > 0 {
		if b, err := json.Marshal(d.EvidenceRefs); err == nil {
			rec.Set("evidence_refs", string(b))
		}
	}

	policyName := d.PolicyName
	if policyName == "" {
		policyName = policyNameForType(d.DecisionType)
	}
	rec.Set("policy_name", policyName)
	rec.Set("policy_version", PolicyVersion)
	rec.Set("runtime_confidence", string(confidence))
	rec.Set("confidence_source", string(confSource))
	rec.Set("decision_timestamp", time.Now().UTC().Format(time.RFC3339))
	if d.HumanSummary != "" {
		rec.Set("human_summary", d.HumanSummary)
	}

	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[DECISION_WRITE_ERR] type=%s target=%s err=%v", d.DecisionType, d.TargetID, err)
		return ""
	}
	if r.metrics != nil {
		r.metrics.IncDecisionRecorded(string(d.DecisionType))
	}
	return rec.Id
}

// shouldDedupeSuppressionType — decisions that fire on every tick during
// persistent state benefit from dedupe. One-shot decisions (retry scheduled,
// lease aborts, contradictions, startup refusal) bypass dedupe entirely.
func shouldDedupeSuppressionType(dt DecisionType) bool {
	switch dt {
	case DecisionCooldownActive, DecisionRetryBackoffWindow, DecisionCircuitBreakerOpen,
		DecisionObservationHeld:
		return true
	}
	return false
}

// policyNameForType — default policy_name when the caller omits it.
func policyNameForType(dt DecisionType) string {
	switch dt {
	case DecisionRetryScheduled, DecisionRetryExhausted, DecisionRetryRefusedNonRetryable,
		DecisionRetryRefusedLeaseInvalid, DecisionRetryEscalatedAmbiguity,
		DecisionRetryOperatorAction:
		return PolicyNameRetryEngine
	case DecisionLeaseLostPreCall, DecisionLeaseLostMidCall, DecisionLeaseAcquireDenied:
		return PolicyNameLeaseEngine
	case DecisionCircuitBreakerOpen:
		return PolicyNameCircuitBreaker
	case DecisionCooldownActive, DecisionRetryBackoffWindow:
		return PolicyNameLifecycleGuard
	case DecisionObservationHeld:
		return PolicyNameObservationGuard
	case DecisionStartupRefusedFatal:
		return PolicyNameStartupAudit
	case DecisionDispatchClaimLost:
		return PolicyNameLifecycleGuard
	case DecisionContradictionDetected:
		return PolicyNameContradiction
	case DecisionProviderContradiction, DecisionProviderUnknowable,
		DecisionDriftOpened, DecisionDriftResolved:
		return PolicyNameObservationGuard
	}
	return PolicyNameLifecycleGuard
}

// String stable representations for tests.
func (d DecisionType) String() string     { return string(d) }
func (c ConfidenceLevel) String() string  { return string(c) }
func (c ConfidenceSource) String() string { return string(c) }

// ─────────────────────────────────────────────────────────────────────────
// READ API
// ─────────────────────────────────────────────────────────────────────────

// DecisionRow is the JSON-friendly shape returned by query endpoints.
type DecisionRow struct {
	ID                string `json:"id"`
	Target            string `json:"target"`
	Rollout           string `json:"rollout,omitempty"`
	OperationID       string `json:"operation_id,omitempty"`
	ReconcileCycleID  string `json:"reconcile_cycle_id,omitempty"`
	PodID             string `json:"pod_id,omitempty"`
	DecisionType      string `json:"decision_type"`
	DecisionReason    string `json:"decision_reason"`
	DecisionInputs    string `json:"decision_inputs,omitempty"`
	EvidenceRefs      string `json:"evidence_refs,omitempty"`
	PolicyName        string `json:"policy_name"`
	PolicyVersion     string `json:"policy_version"`
	RuntimeConfidence string `json:"runtime_confidence"`
	ConfidenceSource  string `json:"confidence_source"`
	DecisionTimestamp string `json:"decision_timestamp"`
	HumanSummary      string `json:"human_summary,omitempty"`
}

// ListDecisions queries runtime_decisions with optional filters.
// Returns rows newest-first by decision_timestamp.
//
// Provides anti-table-scan: at least one of target / cycleID is required.
func ListDecisions(app *pb.PocketBase, target, cycleID, decisionType, since, until string, limit int) ([]DecisionRow, error) {
	if target == "" && cycleID == "" {
		return nil, errMissingFilter
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := app.Dao().DB().
		Select("id", "target", "rollout", "operation_id", "reconcile_cycle_id",
			"pod_id", "decision_type", "decision_reason", "decision_inputs",
			"evidence_refs", "policy_name", "policy_version", "runtime_confidence",
			"confidence_source", "decision_timestamp", "human_summary").
		From("runtime_decisions")

	if target != "" {
		query = query.AndWhere(dbx.NewExp("target = {:t}", dbx.Params{"t": target}))
	}
	if cycleID != "" {
		query = query.AndWhere(dbx.NewExp("reconcile_cycle_id = {:c}", dbx.Params{"c": cycleID}))
	}
	if decisionType != "" {
		query = query.AndWhere(dbx.NewExp("decision_type = {:dt}", dbx.Params{"dt": decisionType}))
	}
	if since != "" {
		query = query.AndWhere(dbx.NewExp("decision_timestamp >= {:s}", dbx.Params{"s": since}))
	}
	if until != "" {
		query = query.AndWhere(dbx.NewExp("decision_timestamp <= {:u}", dbx.Params{"u": until}))
	}

	var rows []struct {
		ID                string `db:"id"`
		Target            string `db:"target"`
		Rollout           string `db:"rollout"`
		OperationID       string `db:"operation_id"`
		ReconcileCycleID  string `db:"reconcile_cycle_id"`
		PodID             string `db:"pod_id"`
		DecisionType      string `db:"decision_type"`
		DecisionReason    string `db:"decision_reason"`
		DecisionInputs    string `db:"decision_inputs"`
		EvidenceRefs      string `db:"evidence_refs"`
		PolicyName        string `db:"policy_name"`
		PolicyVersion     string `db:"policy_version"`
		RuntimeConfidence string `db:"runtime_confidence"`
		ConfidenceSource  string `db:"confidence_source"`
		DecisionTimestamp string `db:"decision_timestamp"`
		HumanSummary      string `db:"human_summary"`
	}
	if err := query.
		OrderBy("decision_timestamp DESC").
		Limit(int64(limit)).
		All(&rows); err != nil {
		return nil, err
	}
	out := make([]DecisionRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, DecisionRow(r))
	}
	return out, nil
}

var errMissingFilter = &decisionsError{msg: "at least one of 'target' or 'cycle_id' query param is required"}

type decisionsError struct{ msg string }

func (e *decisionsError) Error() string { return e.msg }
