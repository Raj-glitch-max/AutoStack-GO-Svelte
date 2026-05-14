package reconciler

import (
	"sync/atomic"
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
}

// Snapshot returns a point-in-time copy of all counters.
// Atomic loads, no allocation beyond the result struct.
func (m *Metrics) Snapshot() MetricsSnapshot {
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
	}
}

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
