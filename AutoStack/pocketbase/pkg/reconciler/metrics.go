package reconciler

import (
	"sync"
	"sync/atomic"

	"github.com/janlauber/one-click/pkg/providers"
)

// Metrics is a minimal in-memory counter set for operational visibility.
//
// Phase 3.6 design constraint: NO Prometheus, NO OpenTelemetry, NO external
// metrics platform. The mandate is "minimal, truthful, operationally
// actionable" — counters that an operator can read via a status endpoint
// or periodic log dump, not a full observability stack.
//
// All fields are atomic.Int64 so increments are safe from concurrent
// reconciler ticks without a mutex. Reads via Snapshot() return a stable
// point-in-time view.
//
// Counters are monotonic since process start. Resets ONLY on process
// restart. An operator computing rates does so externally (delta over
// time window).
type Metrics struct {
	// Reconciliation
	cyclesTotal           atomic.Int64 // reconcile_cycles_total
	cyclesFailed          atomic.Int64 // reconcile_cycles_failed_total
	targetsReconciled     atomic.Int64 // targets_reconciled_total
	targetsSkipped        atomic.Int64 // targets_skipped_total

	// Dispatch
	deploysDispatched     atomic.Int64 // deploys_dispatched_total
	deploysSucceeded      atomic.Int64 // deploys_succeeded_total
	deploysFailed         atomic.Int64 // deploys_failed_total
	destroysDispatched    atomic.Int64 // destroys_dispatched_total
	destroysSucceeded     atomic.Int64 // destroys_succeeded_total
	destroysFailed        atomic.Int64 // destroys_failed_total
	rollbacksDispatched   atomic.Int64 // rollbacks_dispatched_total
	rollbacksSucceeded    atomic.Int64 // rollbacks_succeeded_total
	rollbacksFailed       atomic.Int64 // rollbacks_failed_total

	// Guards
	suspicionHolds        atomic.Int64 // suspicion_holds_total
	forwardProgressHolds  atomic.Int64 // forward_progress_holds_total
	staleReadHolds        atomic.Int64 // stale_read_holds_total
	lowConfidenceHolds    atomic.Int64 // low_confidence_holds_total

	// Drift
	driftDetected         atomic.Int64 // drift_detected_total
	driftResolved         atomic.Int64 // drift_resolved_total

	// Ambiguity
	ambiguitySet          atomic.Int64 // ambiguity_set_total
	ambiguityResolved     atomic.Int64 // ambiguity_resolved_total
	ambiguityTimeouts     atomic.Int64 // ambiguity_timeouts_total
	ambiguityEscalations  atomic.Int64 // ambiguity_escalations_total

	// Sweep
	sweepReclaimed        atomic.Int64 // sweep_reclaimed_total (startup + runtime combined)
	sweepForeignPod       atomic.Int64 // sweep_foreign_pod_total (Phase 3.8 forensic counter)

	// Confidence (provider truthfulness)
	highConfidenceObs     atomic.Int64 // provider_observations_high_total
	mediumConfidenceObs   atomic.Int64 // provider_observations_medium_total
	lowConfidenceObs      atomic.Int64 // provider_observations_low_total

	// Phase 3.9.3 retry runtime integration counters.
	// Per-class counters use a bounded map keyed by canonical RetryClass
	// strings (10 classes total — no per-target cardinality blowup).
	retryScheduledMu      sync.RWMutex
	retryScheduled        map[providers.RetryClass]int64
	retryExhaustedMu      sync.RWMutex
	retryExhausted        map[providers.RetryClass]int64
	retrySuppressedLease  atomic.Int64 // retries suppressed because lease was invalid
	retrySuppressedCooldn atomic.Int64 // ticks skipped because next_retry_at > now
	retryActivated        atomic.Int64 // retry_pending → pending transitions
	retryOperatorActions  atomic.Int64 // failures that required operator action (auth/quota/rate)
	ambiguityFromRetry    atomic.Int64 // ambiguity escalations triggered by retry exhaustion

	// Phase 3.9.4 incident counters. Bounded cardinality: 7 sources max.
	incidentOpenedMu      sync.RWMutex
	incidentOpenedBySource map[string]int64
	incidentEscalations   atomic.Int64
	incidentTransitionsMu sync.RWMutex
	incidentTransitions   map[string]int64 // by new status

	// Phase 3.9.6 decision counters. Bounded cardinality: 20 decision types.
	decisionMu        sync.RWMutex
	decisionsRecorded map[string]int64

	// Phase 3.9.7 provider observation counters.
	// providerObsMu guards providerObsByTrust (5 trust levels max).
	providerObsMu         sync.RWMutex
	providerObsByTrust    map[string]int64
	providerContradictions atomic.Int64

	// Phase 3.9.9 — provider silence and unknown-state counters.
	// providerSilenceTotal: targets that fired the ProviderSilenceOnActiveTarget check.
	// providerUnknownStateTotal: observations where the raw provider state was not a
	// recognized canonical value (trust was downgraded to ambiguous as a result).
	providerSilenceTotal      atomic.Int64
	providerUnknownStateTotal atomic.Int64

	// providerOpenDrifts is seeded from DB on startup (RehydrateMetrics).
	// It represents the count of currently-unresolved provider drift records and
	// is NOT a monotonic counter — it reflects a point-in-time DB count.
	providerOpenDrifts atomic.Int64
}

// MetricsSnapshot is the point-in-time view returned by Snapshot().
// Plain values (not atomics) so callers can compare, serialize, and
// reason about a consistent slice of state.
type MetricsSnapshot struct {
	CyclesTotal           int64 `json:"reconcile_cycles_total"`
	CyclesFailed          int64 `json:"reconcile_cycles_failed_total"`
	TargetsReconciled     int64 `json:"targets_reconciled_total"`
	TargetsSkipped        int64 `json:"targets_skipped_total"`

	DeploysDispatched     int64 `json:"deploys_dispatched_total"`
	DeploysSucceeded      int64 `json:"deploys_succeeded_total"`
	DeploysFailed         int64 `json:"deploys_failed_total"`
	DestroysDispatched    int64 `json:"destroys_dispatched_total"`
	DestroysSucceeded     int64 `json:"destroys_succeeded_total"`
	DestroysFailed        int64 `json:"destroys_failed_total"`
	RollbacksDispatched   int64 `json:"rollbacks_dispatched_total"`
	RollbacksSucceeded    int64 `json:"rollbacks_succeeded_total"`
	RollbacksFailed       int64 `json:"rollbacks_failed_total"`

	SuspicionHolds        int64 `json:"suspicion_holds_total"`
	ForwardProgressHolds  int64 `json:"forward_progress_holds_total"`
	StaleReadHolds        int64 `json:"stale_read_holds_total"`
	LowConfidenceHolds    int64 `json:"low_confidence_holds_total"`

	DriftDetected         int64 `json:"drift_detected_total"`
	DriftResolved         int64 `json:"drift_resolved_total"`

	AmbiguitySet          int64 `json:"ambiguity_set_total"`
	AmbiguityResolved     int64 `json:"ambiguity_resolved_total"`
	AmbiguityTimeouts     int64 `json:"ambiguity_timeouts_total"`
	AmbiguityEscalations  int64 `json:"ambiguity_escalations_total"`

	SweepReclaimed        int64 `json:"sweep_reclaimed_total"`
	SweepForeignPod       int64 `json:"sweep_foreign_pod_total"`

	HighConfidenceObs     int64 `json:"provider_observations_high_total"`
	MediumConfidenceObs   int64 `json:"provider_observations_medium_total"`
	LowConfidenceObs      int64 `json:"provider_observations_low_total"`

	// Phase 3.9.3 — retry runtime metrics.
	RetryScheduledByClass map[string]int64 `json:"retries_scheduled_by_class"`
	RetryExhaustedByClass map[string]int64 `json:"retries_exhausted_by_class"`
	RetrySuppressedLease  int64            `json:"retries_suppressed_by_lease_loss"`
	RetrySuppressedCooldown int64          `json:"retries_suppressed_by_cooldown"`
	RetryActivated        int64            `json:"retries_activated_total"`
	RetryOperatorActions  int64            `json:"retry_operator_action_required_total"`
	AmbiguityFromRetry    int64            `json:"ambiguity_escalations_from_retry_total"`

	// Phase 3.9.4
	IncidentsOpenedBySource  map[string]int64 `json:"incidents_opened_by_source"`
	IncidentEscalations      int64            `json:"incident_escalations_total"`
	IncidentTransitionsByStatus map[string]int64 `json:"incident_transitions_by_status"`

	// Phase 3.9.6
	DecisionsRecordedByType map[string]int64 `json:"decisions_recorded_by_type"`

	// Phase 3.9.7 — provider truth metrics.
	ProviderObsByTrust     map[string]int64 `json:"provider_observations_by_trust"`
	ProviderContradictions int64            `json:"provider_contradictions_total"`

	// Phase 3.9.9 — provider silence and unknown-state metrics.
	ProviderSilenceTotal      int64 `json:"provider_silence_detected_total"`
	ProviderUnknownStateTotal int64 `json:"provider_unknown_state_total"`
	// ProviderOpenDrifts is a DB-seeded point-in-time count, not a monotonic counter.
	// Zero before RehydrateMetrics runs; reset to DB truth on every startup.
	ProviderOpenDrifts int64 `json:"provider_open_drifts_count"`
}

// Snapshot returns a point-in-time copy of all counters.
// Atomic loads, no allocation beyond the result struct.
func (m *Metrics) Snapshot() MetricsSnapshot {
	// Phase 3.9.3: per-class maps are read under RWMutex (read lock).
	// Returned maps are copies so the caller cannot mutate internal state.
	scheduled := make(map[string]int64)
	exhausted := make(map[string]int64)
	m.retryScheduledMu.RLock()
	for k, v := range m.retryScheduled {
		scheduled[string(k)] = v
	}
	m.retryScheduledMu.RUnlock()
	m.retryExhaustedMu.RLock()
	for k, v := range m.retryExhausted {
		exhausted[string(k)] = v
	}
	m.retryExhaustedMu.RUnlock()

	incOpened := make(map[string]int64)
	incTrans := make(map[string]int64)
	m.incidentOpenedMu.RLock()
	for k, v := range m.incidentOpenedBySource {
		incOpened[k] = v
	}
	m.incidentOpenedMu.RUnlock()
	m.incidentTransitionsMu.RLock()
	for k, v := range m.incidentTransitions {
		incTrans[k] = v
	}
	m.incidentTransitionsMu.RUnlock()

	decRec := make(map[string]int64)
	m.decisionMu.RLock()
	for k, v := range m.decisionsRecorded {
		decRec[k] = v
	}
	m.decisionMu.RUnlock()

	provObs := make(map[string]int64)
	m.providerObsMu.RLock()
	for k, v := range m.providerObsByTrust {
		provObs[k] = v
	}
	m.providerObsMu.RUnlock()

	return MetricsSnapshot{
		CyclesTotal:          m.cyclesTotal.Load(),
		CyclesFailed:         m.cyclesFailed.Load(),
		TargetsReconciled:    m.targetsReconciled.Load(),
		TargetsSkipped:       m.targetsSkipped.Load(),
		DeploysDispatched:    m.deploysDispatched.Load(),
		DeploysSucceeded:     m.deploysSucceeded.Load(),
		DeploysFailed:        m.deploysFailed.Load(),
		DestroysDispatched:   m.destroysDispatched.Load(),
		DestroysSucceeded:    m.destroysSucceeded.Load(),
		DestroysFailed:       m.destroysFailed.Load(),
		RollbacksDispatched:  m.rollbacksDispatched.Load(),
		RollbacksSucceeded:   m.rollbacksSucceeded.Load(),
		RollbacksFailed:      m.rollbacksFailed.Load(),
		SuspicionHolds:       m.suspicionHolds.Load(),
		ForwardProgressHolds: m.forwardProgressHolds.Load(),
		StaleReadHolds:       m.staleReadHolds.Load(),
		LowConfidenceHolds:   m.lowConfidenceHolds.Load(),
		DriftDetected:        m.driftDetected.Load(),
		DriftResolved:        m.driftResolved.Load(),
		AmbiguitySet:         m.ambiguitySet.Load(),
		AmbiguityResolved:    m.ambiguityResolved.Load(),
		AmbiguityTimeouts:    m.ambiguityTimeouts.Load(),
		AmbiguityEscalations: m.ambiguityEscalations.Load(),
		SweepReclaimed:       m.sweepReclaimed.Load(),
		SweepForeignPod:      m.sweepForeignPod.Load(),
		HighConfidenceObs:    m.highConfidenceObs.Load(),
		MediumConfidenceObs:  m.mediumConfidenceObs.Load(),
		LowConfidenceObs:     m.lowConfidenceObs.Load(),
		// Phase 3.9.3
		RetryScheduledByClass:   scheduled,
		RetryExhaustedByClass:   exhausted,
		RetrySuppressedLease:    m.retrySuppressedLease.Load(),
		RetrySuppressedCooldown: m.retrySuppressedCooldn.Load(),
		RetryActivated:          m.retryActivated.Load(),
		RetryOperatorActions:    m.retryOperatorActions.Load(),
		AmbiguityFromRetry:      m.ambiguityFromRetry.Load(),
		IncidentsOpenedBySource: incOpened,
		IncidentEscalations:     m.incidentEscalations.Load(),
		IncidentTransitionsByStatus: incTrans,
		DecisionsRecordedByType: decRec,
		ProviderObsByTrust:        provObs,
		ProviderContradictions:    m.providerContradictions.Load(),
		ProviderSilenceTotal:      m.providerSilenceTotal.Load(),
		ProviderUnknownStateTotal: m.providerUnknownStateTotal.Load(),
		ProviderOpenDrifts:        m.providerOpenDrifts.Load(),
	}
}

// Phase 3.9.6 decision metric helper.
func (m *Metrics) IncDecisionRecorded(decisionType string) {
	m.decisionMu.Lock()
	if m.decisionsRecorded == nil {
		m.decisionsRecorded = make(map[string]int64)
	}
	m.decisionsRecorded[decisionType]++
	m.decisionMu.Unlock()
}

// Phase 3.9.4 incident metric helpers.
func (m *Metrics) IncIncidentOpened(source string) {
	m.incidentOpenedMu.Lock()
	if m.incidentOpenedBySource == nil {
		m.incidentOpenedBySource = make(map[string]int64)
	}
	m.incidentOpenedBySource[source]++
	m.incidentOpenedMu.Unlock()
}

func (m *Metrics) IncIncidentEscalation() { m.incidentEscalations.Add(1) }

func (m *Metrics) IncIncidentTransition(newStatus string) {
	m.incidentTransitionsMu.Lock()
	if m.incidentTransitions == nil {
		m.incidentTransitions = make(map[string]int64)
	}
	m.incidentTransitions[newStatus]++
	m.incidentTransitionsMu.Unlock()
}

// Phase 3.9.5 metric truthfulness state.
//
// At process restart, in-memory counters are zero while the DB carries
// persistent runtime truth (incidents, operations, history). Reading
// metrics immediately after restart would underreport. Rehydration
// computes the persisted counts and seeds the counters.
//
// IMPORTANT honesty constraint: rehydration is BEST-EFFORT and APPROXIMATE.
// We can rehydrate cumulative-style counters (total incidents opened by
// source) from DB row counts. We CANNOT rehydrate "per-restart" counters
// like cycle counts — those reset by definition.
//
// truthfulnessState reflects whether rehydration succeeded.

type MetricTruthfulnessState string

const (
	MetricTruthfulnessUntrusted MetricTruthfulnessState = "untrusted_pre_rehydrate"
	MetricTruthfulnessHydrated  MetricTruthfulnessState = "hydrated_from_db"
	MetricTruthfulnessPartial   MetricTruthfulnessState = "partial_db_read_error"
)

var (
	metricTruthfulnessMu    sync.RWMutex
	metricTruthfulnessState = MetricTruthfulnessUntrusted
)

// SetMetricTruthfulnessState updates the global state. Called by RehydrateMetrics.
func SetMetricTruthfulnessState(s MetricTruthfulnessState) {
	metricTruthfulnessMu.Lock()
	metricTruthfulnessState = s
	metricTruthfulnessMu.Unlock()
}

// GetMetricTruthfulnessState exposes the current state for endpoints.
func GetMetricTruthfulnessState() MetricTruthfulnessState {
	metricTruthfulnessMu.RLock()
	defer metricTruthfulnessMu.RUnlock()
	return metricTruthfulnessState
}

// Phase 3.9.3 retry runtime metric incs (bounded cardinality: 10 classes).

func (m *Metrics) IncRetryScheduled(class providers.RetryClass) {
	m.retryScheduledMu.Lock()
	if m.retryScheduled == nil {
		m.retryScheduled = make(map[providers.RetryClass]int64)
	}
	m.retryScheduled[class]++
	m.retryScheduledMu.Unlock()
}

func (m *Metrics) IncRetryExhausted(class providers.RetryClass) {
	m.retryExhaustedMu.Lock()
	if m.retryExhausted == nil {
		m.retryExhausted = make(map[providers.RetryClass]int64)
	}
	m.retryExhausted[class]++
	m.retryExhaustedMu.Unlock()
}

func (m *Metrics) IncRetrySuppressedLease()    { m.retrySuppressedLease.Add(1) }
func (m *Metrics) IncRetrySuppressedCooldown() { m.retrySuppressedCooldn.Add(1) }
func (m *Metrics) IncRetryActivated()          { m.retryActivated.Add(1) }
func (m *Metrics) IncRetryOperatorAction()     { m.retryOperatorActions.Add(1) }
func (m *Metrics) IncAmbiguityFromRetry()      { m.ambiguityFromRetry.Add(1) }

// Increment helpers — single line each, called from reconciler hot paths.
// Atomic increments are nanosecond-cheap; never refactor these into a
// generic "Add(name, n)" map-based API — that would reintroduce locking.

func (m *Metrics) IncCycle()                  { m.cyclesTotal.Add(1) }
func (m *Metrics) IncCycleFailed()            { m.cyclesFailed.Add(1) }
func (m *Metrics) IncTargetReconciled()       { m.targetsReconciled.Add(1) }
func (m *Metrics) IncTargetSkipped()          { m.targetsSkipped.Add(1) }

func (m *Metrics) IncDeployDispatched()       { m.deploysDispatched.Add(1) }
func (m *Metrics) IncDeploySucceeded()        { m.deploysSucceeded.Add(1) }
func (m *Metrics) IncDeployFailed()           { m.deploysFailed.Add(1) }
func (m *Metrics) IncDestroyDispatched()      { m.destroysDispatched.Add(1) }
func (m *Metrics) IncDestroySucceeded()       { m.destroysSucceeded.Add(1) }
func (m *Metrics) IncDestroyFailed()          { m.destroysFailed.Add(1) }
func (m *Metrics) IncRollbackDispatched()     { m.rollbacksDispatched.Add(1) }
func (m *Metrics) IncRollbackSucceeded()      { m.rollbacksSucceeded.Add(1) }
func (m *Metrics) IncRollbackFailed()         { m.rollbacksFailed.Add(1) }

func (m *Metrics) IncSuspicionHold()          { m.suspicionHolds.Add(1) }
func (m *Metrics) IncForwardProgressHold()    { m.forwardProgressHolds.Add(1) }
func (m *Metrics) IncStaleReadHold()          { m.staleReadHolds.Add(1) }
func (m *Metrics) IncLowConfidenceHold()      { m.lowConfidenceHolds.Add(1) }

func (m *Metrics) IncDriftDetected()          { m.driftDetected.Add(1) }
func (m *Metrics) IncDriftResolved()          { m.driftResolved.Add(1) }

func (m *Metrics) IncAmbiguitySet()           { m.ambiguitySet.Add(1) }
func (m *Metrics) IncAmbiguityResolved()      { m.ambiguityResolved.Add(1) }
func (m *Metrics) IncAmbiguityTimeout()       { m.ambiguityTimeouts.Add(1) }
func (m *Metrics) IncAmbiguityEscalation()    { m.ambiguityEscalations.Add(1) }

func (m *Metrics) IncSweepReclaimed()         { m.sweepReclaimed.Add(1) }
func (m *Metrics) IncSweepForeignPod()        { m.sweepForeignPod.Add(1) }

// IncConfidenceObs increments the per-level confidence counter.
// Called from reconcileOne after each successful GetStatus.
func (m *Metrics) IncConfidenceObs(level string) {
	switch level {
	case "high":
		m.highConfidenceObs.Add(1)
	case "medium":
		m.mediumConfidenceObs.Add(1)
	case "low":
		m.lowConfidenceObs.Add(1)
	}
}

// Phase 3.9.7 — provider truth metric helpers.

// IncProviderObservation increments the per-trust-level observation counter.
func (m *Metrics) IncProviderObservation(trust string) {
	m.providerObsMu.Lock()
	if m.providerObsByTrust == nil {
		m.providerObsByTrust = make(map[string]int64)
	}
	m.providerObsByTrust[trust]++
	m.providerObsMu.Unlock()
}

// IncProviderContradiction increments the provider contradiction total.
func (m *Metrics) IncProviderContradiction() {
	m.providerContradictions.Add(1)
}

// Phase 3.9.9 — provider silence and unknown-state metric helpers.

// IncProviderSilence increments the silence detection counter. Called when
// ProviderSilenceOnActiveTarget contradiction fires for a target.
func (m *Metrics) IncProviderSilence() { m.providerSilenceTotal.Add(1) }

// IncProviderUnknownState increments the unknown-state counter. Called when
// a raw provider state string is not recognized as canonical and trust is
// downgraded to ambiguous.
func (m *Metrics) IncProviderUnknownState() { m.providerUnknownStateTotal.Add(1) }
