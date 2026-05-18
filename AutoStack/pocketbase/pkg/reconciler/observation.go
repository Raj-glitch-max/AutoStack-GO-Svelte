package reconciler

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
	pbmodels "github.com/pocketbase/pocketbase/models"
)

// Phase 3.9.7 — Provider Truth Reconciliation.
//
// Every provider state observation is now a first-class forensic record.
// Operators can reconstruct what the provider claimed, when it was observed,
// whether it was trusted, whether it contradicted runtime belief, and whether
// the divergence was transient or persistent.
//
// ─────────────────────────────────────────────────────────────────────────
// HONEST CONSTRAINTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. Provider observations are APPEND-ONLY. RecordProviderObservation
//      never updates an existing row; each observation is a new row.
//
//   2. Contradiction detection is INLINE and BEST-EFFORT. If the DB write
//      fails, the forensic record is lost — the operational action continues.
//
//   3. Drift records are NOT auto-repaired. The runtime opens drift when
//      a contradiction is detected and closes it when the provider converges.
//      If convergence never occurs, the drift record remains open indefinitely
//      until an operator investigates and calls ResolveProviderDrift explicitly,
//      OR until a subsequent non-contradicting observation closes it.
//
//   4. Observation trust is set by the CALLER, not by this package. The
//      caller knows HOW the observation was obtained (poll vs deploy result)
//      and can classify it correctly. This package overrides trust only when
//      contradiction is detected inline or when the observation is stale.
//
// ─────────────────────────────────────────────────────────────────────────
// EXPLICITLY UNSUPPORTED BEHAVIORS
// ─────────────────────────────────────────────────────────────────────────
//
//   - Provider eventual consistency: a provider that reports "pending"
//     immediately after a completed deploy is NOT a contradiction — it is
//     expected eventual-consistency lag. The caller MUST classify such
//     observations as ObservationTrustInferred, not ObservationTrustContradicted,
//     or use runtime_expected_state="" to skip inline detection.
//
//   - Stale cloud APIs: providers with multi-minute propagation delay will
//     appear as contradictions to the runtime. There is no automatic
//     distinction between "API lag" and "real state divergence."
//
//   - Partial provider outages: if the provider returns errors, the caller
//     MUST classify the observation as ObservationTrustAmbiguous. The runtime
//     does NOT claim to know the provider state in this case.
//
//   - Provider clock uncertainty: observed_at is the LOCAL runtime clock.
//     Provider timestamps (if any) belong in evidence_json but are NOT
//     used for ordering observations.
//
//   - Concurrent observation races: concurrent reconciles for the same target
//     can produce interleaved observations. Drift detection is per-target but
//     not serialized across goroutines — the DB's SQLite WAL serializes writes,
//     so two concurrent drift opens for the same (target, provider) will both
//     succeed (two drift records). This is an acceptable approximation.
//
//   - No provider truth validation: the runtime trusts provider responses.
//     A compromised or misbehaving provider API can inject false states.
//     Provider authentication is the defense; this file does not add one.

// ─────────────────────────────────────────────────────────────────────────
// OBSERVATION SOURCE
// ─────────────────────────────────────────────────────────────────────────

// ObservationSource is the canonical provenance of a provider observation.
// Persisted as observation_source in provider_observations.
// String values are STABLE; renaming requires a migration.
type ObservationSource string

const (
	// ObservationSourcePoll — periodic status poll (reconcile-tick GetStatus).
	ObservationSourcePoll ObservationSource = "poll"

	// ObservationSourceDeployResult — observation from the response/aftermath
	// of a Deploy call (what the provider reported as the post-deploy state).
	ObservationSourceDeployResult ObservationSource = "deploy-result"

	// ObservationSourceRollbackResult — observation from the response/aftermath
	// of a Rollback call.
	ObservationSourceRollbackResult ObservationSource = "rollback-result"

	// ObservationSourceReconciliationRead — an explicit status read that was
	// not part of a deploy/rollback (e.g., a pre-dispatch status check).
	ObservationSourceReconciliationRead ObservationSource = "reconciliation-read"

	// ObservationSourceStartupRehydrate — observation recorded during startup
	// rehydration to establish baseline provider truth.
	ObservationSourceStartupRehydrate ObservationSource = "startup-rehydrate"
)

// AllObservationSources — registry for completeness checks.
var AllObservationSources = []ObservationSource{
	ObservationSourcePoll,
	ObservationSourceDeployResult,
	ObservationSourceRollbackResult,
	ObservationSourceReconciliationRead,
	ObservationSourceStartupRehydrate,
}

// ─────────────────────────────────────────────────────────────────────────
// OBSERVATION TRUST
// ─────────────────────────────────────────────────────────────────────────

// ObservationTrust classifies the trustworthiness of a single provider
// observation. This is NOT an AI probability — it is a deterministic
// classification based on how the observation was obtained.
//
// String values are PERSISTED in provider_observations.observation_trust.
// Renaming requires a migration.
type ObservationTrust string

const (
	// ObservationTrustAuthoritative — provider gave a clear, definitive answer
	// (e.g., 200 OK with explicit canonical state). No ambiguity, no known
	// eventual-consistency lag.
	ObservationTrustAuthoritative ObservationTrust = "authoritative"

	// ObservationTrustInferred — runtime inferred state from indirect evidence
	// (e.g., no error from a deploy call = "probably deployed"). The provider
	// did not confirm the post-action state explicitly.
	ObservationTrustInferred ObservationTrust = "inferred"

	// ObservationTrustStale — observation exceeds the staleness window.
	// Provider state may have changed since this was read.
	ObservationTrustStale ObservationTrust = "stale"

	// ObservationTrustAmbiguous — provider gave an unclear or incomplete
	// answer (e.g., API timeout, partial response, error response where
	// commit status is unknown).
	ObservationTrustAmbiguous ObservationTrust = "ambiguous"

	// ObservationTrustContradicted — provider state explicitly contradicts
	// runtime expectation (e.g., provider says resource is missing, but
	// runtime believes it should be running). Set inline by RecordProviderObservation
	// if DetectObservationContradiction fires, or pre-set by the caller.
	ObservationTrustContradicted ObservationTrust = "contradicted"
)

// AllObservationTrustLevels — registry for completeness checks.
var AllObservationTrustLevels = []ObservationTrust{
	ObservationTrustAuthoritative,
	ObservationTrustInferred,
	ObservationTrustStale,
	ObservationTrustAmbiguous,
	ObservationTrustContradicted,
}

// ─────────────────────────────────────────────────────────────────────────
// PROVIDER CONFIDENCE CLASS
// ─────────────────────────────────────────────────────────────────────────

// ProviderConfidenceClass is the finer-grained confidence taxonomy for
// provider truth. These values appear in decision_inputs on runtime_decisions
// rows that were influenced by a provider observation.
//
// Distinct from ConfidenceLevel (high/medium/low), which is the broader
// per-decision confidence. ProviderConfidenceClass explains the REASON.
type ProviderConfidenceClass string

const (
	// ProviderConfidenceAuthoritative — provider gave a definitive answer;
	// the runtime is confident the observation reflects ground truth.
	ProviderConfidenceAuthoritative ProviderConfidenceClass = "authoritative_provider"

	// ProviderConfidenceInferred — runtime inferred state from indirect
	// evidence; confidence is medium.
	ProviderConfidenceInferred ProviderConfidenceClass = "inferred_provider"

	// ProviderConfidenceStaleCache — observation is beyond the staleness
	// window; the runtime is operating on potentially outdated information.
	ProviderConfidenceStaleCache ProviderConfidenceClass = "stale_cache"

	// ProviderConfidenceAmbiguous — provider gave an ambiguous/incomplete
	// answer; the runtime does not know the current state.
	ProviderConfidenceAmbiguous ProviderConfidenceClass = "ambiguous_result"

	// ProviderConfidenceContradicted — provider state contradicts runtime
	// expectation; the runtime has detected a divergence.
	ProviderConfidenceContradicted ProviderConfidenceClass = "contradiction_detected"

	// ProviderConfidenceAssumption — the runtime is operating on an assumption
	// with no provider evidence (e.g., no observations recorded yet).
	ProviderConfidenceAssumption ProviderConfidenceClass = "runtime_assumption"
)

// AllProviderConfidenceClasses — registry for completeness checks.
var AllProviderConfidenceClasses = []ProviderConfidenceClass{
	ProviderConfidenceAuthoritative,
	ProviderConfidenceInferred,
	ProviderConfidenceStaleCache,
	ProviderConfidenceAmbiguous,
	ProviderConfidenceContradicted,
	ProviderConfidenceAssumption,
}

// ClassifyProviderConfidence maps ObservationTrust to ProviderConfidenceClass.
// Pure function — deterministic, unit-testable without a DB.
func ClassifyProviderConfidence(trust ObservationTrust) ProviderConfidenceClass {
	switch trust {
	case ObservationTrustAuthoritative:
		return ProviderConfidenceAuthoritative
	case ObservationTrustInferred:
		return ProviderConfidenceInferred
	case ObservationTrustStale:
		return ProviderConfidenceStaleCache
	case ObservationTrustAmbiguous:
		return ProviderConfidenceAmbiguous
	case ObservationTrustContradicted:
		return ProviderConfidenceContradicted
	default:
		return ProviderConfidenceAssumption
	}
}

// ─────────────────────────────────────────────────────────────────────────
// STALENESS WINDOW
// ─────────────────────────────────────────────────────────────────────────

// observationStalenessWindow is the maximum age of an observation before
// it is classified as stale. At the default reconcile interval (30s), this
// gives 6 missed polls before stale — generous enough to survive a transient
// pause without false positives.
//
// This is a heuristic, not a guarantee. Providers with multi-minute
// propagation delays will still produce fresh-looking but incorrect observations.
const observationStalenessWindow = 3 * time.Minute

// IsObservationStale reports whether an observation's ObservedAt timestamp
// is beyond observationStalenessWindow. Pure function.
func IsObservationStale(observedAt time.Time) bool {
	return time.Since(observedAt) > observationStalenessWindow
}

// ─────────────────────────────────────────────────────────────────────────
// CONTRADICTION DETECTION
// ─────────────────────────────────────────────────────────────────────────

// contradictionStateMap defines (provider_state → []runtime_expected_states)
// pairs that constitute a known provider/runtime contradiction.
//
// Pure data — no DB access. Adding a new entry is the only way to introduce
// a new contradiction rule. Overly aggressive entries cause false positives
// during legitimate eventual-consistency windows; overly conservative entries
// miss real divergences. Each entry must be justified.
var contradictionStateMap = map[string][]string{
	// Provider says the resource does not exist; runtime believes it is live.
	"missing":   {"running", "deployed", "active", "updating", "healthy"},
	"not_found": {"running", "deployed", "active", "updating", "healthy"},
	"deleted":   {"running", "deployed", "active", "updating", "healthy"},
	// Provider says unambiguous error; runtime expects healthy operation.
	// NOT a contradiction if runtime_expected_state is "error" — that would
	// mean the runtime already knows about the problem.
	"error": {"running", "deployed", "active", "healthy"},
	// Provider says the resource is live; runtime says it was removed or
	// rolled back. This is the "ghost resource" scenario.
	"running":  {"deleted", "rolled_back", "rollback_complete"},
	"active":   {"deleted", "rolled_back", "rollback_complete"},
	"healthy":  {"deleted", "rolled_back", "rollback_complete"},
	"deployed": {"deleted", "rolled_back", "rollback_complete"},
	"updating": {"deleted", "rolled_back", "rollback_complete"},
}

// DetectObservationContradiction reports whether (providerState,
// runtimeExpectedState) is a known contradiction pair.
//
// Returns (true, reason) on contradiction; (false, "") otherwise.
// Pure function — deterministic, no side effects.
func DetectObservationContradiction(providerState, runtimeExpectedState string) (bool, string) {
	if providerState == "" || runtimeExpectedState == "" {
		return false, ""
	}
	if providerState == runtimeExpectedState {
		return false, ""
	}
	for _, expected := range contradictionStateMap[providerState] {
		if expected == runtimeExpectedState {
			return true, fmt.Sprintf("provider=%s but runtime_expected=%s", providerState, runtimeExpectedState)
		}
	}
	return false, ""
}

// ─────────────────────────────────────────────────────────────────────────
// PHASE 3.9.8 HELPERS — TRUST CLASSIFICATION + EXPECTED STATE MAPPING
// ─────────────────────────────────────────────────────────────────────────

// classifyObservationTrustFromConfidence maps TargetStatus.ConfidenceLevel
// ("high"/"medium"/"low"/"") to ObservationTrust. Pure function.
//
// Used when wiring RecordProviderObservation at GetStatus call sites. The
// caller overrides the result to ObservationTrustAmbiguous when
// TargetStatus.Ambiguous == true.
func classifyObservationTrustFromConfidence(confidenceLevel string) ObservationTrust {
	switch confidenceLevel {
	case "high":
		return ObservationTrustAuthoritative
	case "medium":
		return ObservationTrustInferred
	case "low":
		return ObservationTrustAmbiguous
	default:
		// Empty or unrecognised confidence — treat as inferred, not authoritative.
		return ObservationTrustInferred
	}
}

// runtimeToExpectedProviderState maps deployment_targets.status vocabulary
// to the provider-expected state used by DetectObservationContradiction.
//
// Returns "" for transitional states (creating, pending, deleting) where
// no reliable expectation exists. An empty expected state causes
// DetectObservationContradiction to skip contradiction detection.
//
// Pure function — no side effects.
func runtimeToExpectedProviderState(runtimeStatus string) string {
	switch runtimeStatus {
	case "running":
		return "running"
	case "updating":
		return "updating"
	case "error":
		return "error"
	case "deleted":
		return "deleted"
	case "rolled_back":
		return "rolled_back"
	default:
		// creating, pending, deleting, "" — transitional; no reliable expectation.
		return ""
	}
}

// ─────────────────────────────────────────────────────────────────────────
// INPUT + RECORD TYPES
// ─────────────────────────────────────────────────────────────────────────

// ProviderObservationInput is what the reconciler passes to
// RecordProviderObservation. ContradictionDetected and Trust may be
// pre-set by the caller; if not, inline detection runs.
type ProviderObservationInput struct {
	TargetID             string
	OperationID          string
	CycleID              string
	Provider             string
	ObservationSource    ObservationSource
	ProviderState        string
	RuntimeExpectedState string
	Trust                ObservationTrust
	EvidenceJSON         map[string]interface{}
	ContradictionDetected bool
	DriftDetected        bool
	DriftReason          string
	// ObservedAt defaults to time.Now().UTC() if zero.
	ObservedAt time.Time
}

// ProviderObservationRecord is the JSON-friendly shape for the read API.
type ProviderObservationRecord struct {
	ID                    string `json:"id"`
	Target                string `json:"target"`
	OperationID           string `json:"operation_id,omitempty"`
	CycleID               string `json:"cycle_id,omitempty"`
	PodID                 string `json:"pod_id,omitempty"`
	ObservedAt            string `json:"observed_at"`
	Provider              string `json:"provider"`
	ObservationSource     string `json:"observation_source"`
	ProviderState         string `json:"provider_state"`
	RuntimeExpectedState  string `json:"runtime_expected_state,omitempty"`
	ObservationTrust      string `json:"observation_trust"`
	EvidenceJSON          string `json:"evidence_json,omitempty"`
	ContradictionDetected bool   `json:"contradiction_detected"`
	DriftDetected         bool   `json:"drift_detected"`
	DriftReason           string `json:"drift_reason,omitempty"`
}

// DriftRecord is the JSON-friendly shape for provider_drifts read API.
type DriftRecord struct {
	ID               string `json:"id"`
	Target           string `json:"target"`
	Provider         string `json:"provider"`
	FirstSeenAt      string `json:"first_seen_at"`
	LastSeenAt       string `json:"last_seen_at"`
	DriftState       string `json:"drift_state"`
	DriftCount       int    `json:"drift_count"`
	ResolvedAt       string `json:"resolved_at,omitempty"`
	ResolutionReason string `json:"resolution_reason,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────
// PERSISTENCE — PROVIDER OBSERVATIONS
// ─────────────────────────────────────────────────────────────────────────

// RecordProviderObservation persists a provider state observation as a
// first-class forensic record. Best-effort — DB errors are logged but do
// not propagate. Returns the new row ID, or "" on failure.
//
// Side effects:
//   - If contradiction is detected (inline or pre-set), opens/updates a
//     drift record and emits a DecisionProviderContradiction + incident.
//   - If no contradiction is detected and an active drift exists for
//     (target, provider), resolves the drift ("provider converged").
//   - Updates metrics regardless of DB outcome (counters are best-effort too).
func (r *Reconciler) RecordProviderObservation(obs ProviderObservationInput) string {
	if obs.TargetID == "" || obs.Provider == "" || obs.ProviderState == "" {
		return ""
	}
	if obs.ObservedAt.IsZero() {
		obs.ObservedAt = time.Now().UTC()
	}

	// Inline contradiction detection (only if caller did not pre-set it and
	// we have both sides of the comparison).
	if !obs.ContradictionDetected && obs.RuntimeExpectedState != "" {
		if detected, reason := DetectObservationContradiction(obs.ProviderState, obs.RuntimeExpectedState); detected {
			obs.ContradictionDetected = true
			obs.DriftDetected = true
			if obs.DriftReason == "" {
				obs.DriftReason = reason
			}
			obs.Trust = ObservationTrustContradicted
		}
	}

	// Stale override: if not already contradicted, check age.
	if obs.Trust != ObservationTrustContradicted && IsObservationStale(obs.ObservedAt) {
		obs.Trust = ObservationTrustStale
	}

	col, err := r.app.Dao().FindCollectionByNameOrId("provider_observations")
	if err != nil {
		log.Printf("[OBS_COLLECTION_ERR] err=%v", err)
		return ""
	}
	rec := pbmodels.NewRecord(col)
	rec.Set("target", obs.TargetID)
	if obs.OperationID != "" {
		rec.Set("operation_id", obs.OperationID)
	}
	if obs.CycleID != "" {
		rec.Set("cycle_id", obs.CycleID)
	}
	rec.Set("pod_id", string(currentPodIdentity()))
	rec.Set("observed_at", obs.ObservedAt.UTC().Format(time.RFC3339))
	rec.Set("provider", obs.Provider)
	rec.Set("observation_source", string(obs.ObservationSource))
	rec.Set("provider_state", obs.ProviderState)
	if obs.RuntimeExpectedState != "" {
		rec.Set("runtime_expected_state", obs.RuntimeExpectedState)
	}
	rec.Set("observation_trust", string(obs.Trust))
	if obs.EvidenceJSON != nil {
		if b, jsonErr := json.Marshal(obs.EvidenceJSON); jsonErr == nil {
			rec.Set("evidence_json", string(b))
		}
	}
	rec.Set("contradiction_detected", obs.ContradictionDetected)
	rec.Set("drift_detected", obs.DriftDetected || obs.ContradictionDetected)
	if obs.DriftReason != "" {
		rec.Set("drift_reason", obs.DriftReason)
	}

	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[OBS_WRITE_ERR] target=%s provider=%s err=%v", obs.TargetID, obs.Provider, err)
		return ""
	}
	obsID := rec.Id

	log.Printf("[OBS_RECORDED] id=%s target=%s provider=%s source=%s state=%s trust=%s contradiction=%v",
		obsID, obs.TargetID, obs.Provider, obs.ObservationSource, obs.ProviderState,
		obs.Trust, obs.ContradictionDetected)

	// Metrics.
	if r.metrics != nil {
		r.metrics.IncProviderObservation(string(obs.Trust))
		if obs.ContradictionDetected {
			r.metrics.IncProviderContradiction()
		}
	}

	if obs.ContradictionDetected {
		// Open or update the drift record.
		driftID := r.openOrUpdateProviderDrift(obs.TargetID, obs.Provider, obs.ProviderState, obs.DriftReason)

		// Persist a contradiction decision.
		evRefs := []EvidenceRef{{Collection: "provider_observations", ID: obsID}}
		if driftID != "" {
			evRefs = append(evRefs, EvidenceRef{Collection: "provider_drifts", ID: driftID})
		}
		r.RecordDecision(DecisionRecord{
			TargetID:       obs.TargetID,
			OperationID:    obs.OperationID,
			CycleID:        obs.CycleID,
			DecisionType:   DecisionProviderContradiction,
			DecisionReason: obs.DriftReason,
			HumanSummary: fmt.Sprintf("provider %s claims state=%s but runtime expected=%s",
				obs.Provider, obs.ProviderState, obs.RuntimeExpectedState),
			PolicyName: PolicyNameObservationGuard,
			DecisionInputs: map[string]interface{}{
				"provider_state":          obs.ProviderState,
				"runtime_expected_state":  obs.RuntimeExpectedState,
				"observation_trust":       string(obs.Trust),
				"observation_source":      string(obs.ObservationSource),
				"provider_confidence":     string(ClassifyProviderConfidence(obs.Trust)),
			},
			EvidenceRefs: evRefs,
		})

		// Escalate a contradiction incident.
		r.CreateOrEscalateIncident(
			obs.TargetID, "",
			IncidentSourceProviderContradiction,
			IncidentSeverityHigh,
			obs.OperationID,
			fmt.Sprintf("provider %s state=%s contradicts runtime expected=%s (drift_id=%s obs_id=%s)",
				obs.Provider, obs.ProviderState, obs.RuntimeExpectedState, driftID, obsID),
			IncidentRuntimeState{
				NativeProviderCode: obs.Provider,
				AmbiguitySource:    string(obs.ObservationSource),
				AmbiguityNative:    obs.ProviderState,
			},
		)
		return obsID
	}

	// No contradiction — if there was an active drift for this target+provider,
	// close it: the provider state has converged with runtime expectation.
	existing, err := r.findActiveProviderDrift(obs.TargetID, obs.Provider)
	if err != nil {
		log.Printf("[OBS_DRIFT_LOOKUP_ERR] target=%s provider=%s err=%v", obs.TargetID, obs.Provider, err)
	} else if existing != nil {
		reason := fmt.Sprintf("provider state converged: observed=%s (obs_id=%s)", obs.ProviderState, obsID)
		if resolveErr := r.ResolveProviderDrift(existing.Id, reason); resolveErr != nil {
			log.Printf("[OBS_DRIFT_RESOLVE_ERR] drift=%s err=%v", existing.Id, resolveErr)
		}
	}
	return obsID
}

// ─────────────────────────────────────────────────────────────────────────
// PERSISTENCE — PROVIDER DRIFTS
// ─────────────────────────────────────────────────────────────────────────

// openOrUpdateProviderDrift opens a new drift record or updates an existing
// active one for (target, provider). Best-effort. Returns the drift record ID.
func (r *Reconciler) openOrUpdateProviderDrift(targetID, provider, driftState, reason string) string {
	now := time.Now().UTC().Format(time.RFC3339)

	existing, err := r.findActiveProviderDrift(targetID, provider)
	if err != nil {
		log.Printf("[DRIFT_LOOKUP_ERR] target=%s provider=%s err=%v", targetID, provider, err)
	}

	if existing != nil {
		newCount := existing.GetInt("drift_count") + 1
		existing.Set("drift_count", newCount)
		existing.Set("last_seen_at", now)
		existing.Set("drift_state", driftState)
		if err := r.app.Dao().SaveRecord(existing); err != nil {
			log.Printf("[DRIFT_UPDATE_ERR] id=%s err=%v", existing.Id, err)
			return ""
		}
		log.Printf("[DRIFT_UPDATED] id=%s target=%s provider=%s count=%d state=%s",
			existing.Id, targetID, provider, newCount, driftState)
		return existing.Id
	}

	col, err := r.app.Dao().FindCollectionByNameOrId("provider_drifts")
	if err != nil {
		log.Printf("[DRIFT_COLLECTION_ERR] err=%v", err)
		return ""
	}
	rec := pbmodels.NewRecord(col)
	rec.Set("target", targetID)
	rec.Set("provider", provider)
	rec.Set("first_seen_at", now)
	rec.Set("last_seen_at", now)
	rec.Set("drift_state", driftState)
	rec.Set("drift_count", 1)
	if reason != "" {
		rec.Set("resolution_reason", "")
	}
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		log.Printf("[DRIFT_CREATE_ERR] target=%s provider=%s err=%v", targetID, provider, err)
		return ""
	}
	log.Printf("[DRIFT_OPENED] id=%s target=%s provider=%s state=%s reason=%s",
		rec.Id, targetID, provider, driftState, reason)
	if r.metrics != nil {
		r.metrics.IncDriftDetected()
	}
	return rec.Id
}

// ResolveProviderDrift closes an active provider drift record with a reason.
// Returns error for validation failures; best-effort for DB errors.
// reason MUST be non-empty (truthfulness rule).
func (r *Reconciler) ResolveProviderDrift(driftID, reason string) error {
	if driftID == "" {
		return fmt.Errorf("driftID required")
	}
	if reason == "" {
		return fmt.Errorf("reason required for drift resolution (truthfulness rule)")
	}
	rec, err := r.app.Dao().FindRecordById("provider_drifts", driftID)
	if err != nil {
		return fmt.Errorf("drift record not found: %w", err)
	}
	if rec.GetString("resolved_at") != "" {
		return fmt.Errorf("drift %s is already resolved at %s", driftID, rec.GetString("resolved_at"))
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rec.Set("resolved_at", now)
	rec.Set("resolution_reason", reason)
	if err := r.app.Dao().SaveRecord(rec); err != nil {
		return fmt.Errorf("save drift resolution: %w", err)
	}
	if r.metrics != nil {
		r.metrics.IncDriftResolved()
	}
	log.Printf("[DRIFT_RESOLVED] id=%s reason=%s", driftID, reason)
	return nil
}

// findActiveProviderDrift returns the most-recently-updated unresolved drift
// for (target, provider), or nil if none. Error only on DB failure.
func (r *Reconciler) findActiveProviderDrift(targetID, provider string) (*pbmodels.Record, error) {
	records, err := r.app.Dao().FindRecordsByExpr("provider_drifts",
		dbx.NewExp("target = {:t} AND provider = {:p} AND (resolved_at IS NULL OR resolved_at = '')",
			dbx.Params{"t": targetID, "p": provider}))
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	latest := records[0]
	for _, rec := range records[1:] {
		if rec.GetString("last_seen_at") > latest.GetString("last_seen_at") {
			latest = rec
		}
	}
	return latest, nil
}

// ─────────────────────────────────────────────────────────────────────────
// READ API
// ─────────────────────────────────────────────────────────────────────────

// ListProviderObservations returns provider observations for a target,
// newest-first by observed_at, up to limit.
func ListProviderObservations(app *pb.PocketBase, targetID string, limit int) ([]ProviderObservationRecord, error) {
	if targetID == "" {
		return nil, fmt.Errorf("targetID required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []struct {
		ID                    string `db:"id"`
		Target                string `db:"target"`
		OperationID           string `db:"operation_id"`
		CycleID               string `db:"cycle_id"`
		PodID                 string `db:"pod_id"`
		ObservedAt            string `db:"observed_at"`
		Provider              string `db:"provider"`
		ObservationSource     string `db:"observation_source"`
		ProviderState         string `db:"provider_state"`
		RuntimeExpectedState  string `db:"runtime_expected_state"`
		ObservationTrust      string `db:"observation_trust"`
		EvidenceJSON          string `db:"evidence_json"`
		ContradictionDetected bool   `db:"contradiction_detected"`
		DriftDetected         bool   `db:"drift_detected"`
		DriftReason           string `db:"drift_reason"`
	}
	err := app.Dao().DB().
		Select("id", "target", "operation_id", "cycle_id", "pod_id",
			"observed_at", "provider", "observation_source", "provider_state",
			"runtime_expected_state", "observation_trust", "evidence_json",
			"contradiction_detected", "drift_detected", "drift_reason").
		From("provider_observations").
		Where(dbx.NewExp("target = {:t}", dbx.Params{"t": targetID})).
		OrderBy("observed_at DESC").
		Limit(int64(limit)).
		All(&rows)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderObservationRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProviderObservationRecord{
			ID:                    row.ID,
			Target:                row.Target,
			OperationID:           row.OperationID,
			CycleID:               row.CycleID,
			PodID:                 row.PodID,
			ObservedAt:            row.ObservedAt,
			Provider:              row.Provider,
			ObservationSource:     row.ObservationSource,
			ProviderState:         row.ProviderState,
			RuntimeExpectedState:  row.RuntimeExpectedState,
			ObservationTrust:      row.ObservationTrust,
			EvidenceJSON:          row.EvidenceJSON,
			ContradictionDetected: row.ContradictionDetected,
			DriftDetected:         row.DriftDetected,
			DriftReason:           row.DriftReason,
		})
	}
	return out, nil
}

// ListProviderDrifts returns drift records for a target, newest-first by
// last_seen_at, up to limit. Set activeOnly=true to return only unresolved drifts.
func ListProviderDrifts(app *pb.PocketBase, targetID string, activeOnly bool, limit int) ([]DriftRecord, error) {
	if targetID == "" {
		return nil, fmt.Errorf("targetID required")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := app.Dao().DB().
		Select("id", "target", "provider", "first_seen_at", "last_seen_at",
			"drift_state", "drift_count", "resolved_at", "resolution_reason").
		From("provider_drifts").
		Where(dbx.NewExp("target = {:t}", dbx.Params{"t": targetID}))
	if activeOnly {
		q = q.AndWhere(dbx.NewExp("(resolved_at IS NULL OR resolved_at = '')"))
	}
	var rows []struct {
		ID               string `db:"id"`
		Target           string `db:"target"`
		Provider         string `db:"provider"`
		FirstSeenAt      string `db:"first_seen_at"`
		LastSeenAt       string `db:"last_seen_at"`
		DriftState       string `db:"drift_state"`
		DriftCount       int    `db:"drift_count"`
		ResolvedAt       string `db:"resolved_at"`
		ResolutionReason string `db:"resolution_reason"`
	}
	if err := q.OrderBy("last_seen_at DESC").Limit(int64(limit)).All(&rows); err != nil {
		return nil, err
	}
	out := make([]DriftRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, DriftRecord{
			ID:               row.ID,
			Target:           row.Target,
			Provider:         row.Provider,
			FirstSeenAt:      row.FirstSeenAt,
			LastSeenAt:       row.LastSeenAt,
			DriftState:       row.DriftState,
			DriftCount:       row.DriftCount,
			ResolvedAt:       row.ResolvedAt,
			ResolutionReason: row.ResolutionReason,
		})
	}
	return out, nil
}
