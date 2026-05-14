package reconciler

// DriftState classifies the relationship between desired spec (what the rollout
// says should be deployed) and the last deployed baseline (what was last
// successfully deployed, recorded as last_deployed_revision on the target).
//
// Phase 3.4 baseline: revision-only drift detection. Full spec-content
// comparison (image tag, compute, scale) requires deployed_spec snapshot
// and is deferred to Phase 3.5.
//
// The reconciler writes drift_state on every GetStatus tick. The UI MUST
// surface non-no_drift values honestly — drift is NOT a failure, it is a
// truthful statement about the gap between desired and deployed state.
type DriftState string

const (
	// DriftStateUnknown means no last_deployed_revision is available yet.
	// This is the initial state for targets that have never had a
	// successful deploy, or for pre-Phase-3.4 targets without the column.
	DriftStateUnknown DriftState = "unknown"

	// DriftStateNoDrift means the desired spec revision matches the last
	// successfully deployed revision. The deployed resource reflects what
	// the operator intends.
	DriftStateNoDrift DriftState = "no_drift"

	// DriftStateSuspected means the rollout spec was updated since the
	// last successful deploy. The provider has not yet been asked to
	// converge. A pending deploy will resolve this.
	DriftStateSuspected DriftState = "suspected_drift"

	// DriftStateConfirmed means spec drift has persisted across multiple
	// reconcile ticks without a deploy being dispatched. The target is
	// in a stable status (running or error) with an outdated spec.
	// This is NOT the same as a failed deploy — it means the operator
	// has not yet triggered convergence.
	DriftStateConfirmed DriftState = "confirmed_drift"

	// DriftStateStaleProvider means the provider reported a status that
	// conflicts with the known deployed baseline. For example, a provider
	// reports "updating" immediately after a confirmed successful deploy —
	// this is likely a stale read, not real rollout activity.
	DriftStateStaleProvider DriftState = "stale_provider_state"
)

// ambiguityEscalationThreshold is the number of consecutive
// [AMBIGUITY_TIMEOUT] observations before the reconciler escalates the
// target from "ambiguous" to "error". Set to 3 so a single provider
// hiccup does not prematurely escalate, but prolonged ambiguity does.
//
// At the default reconcile interval (30s), this means ambiguity that
// persists ~90s past its deadline is escalated. The deadline itself is
// UncertaintyP99 × 3 (capped at 10min), so total tolerance is:
//   ECS:        120s deadline + 3×30s = ~3.5 minutes
//   Cloud Run:   60s deadline + 3×30s = ~2.5 minutes
const ambiguityEscalationThreshold = 3

// detectRevisionDrift computes the drift state based on revision comparison.
//
// desiredRevision: rollout.updated_at — the timestamp of the current spec.
// lastDeployedRevision: the rollout revision that was last successfully deployed.
//
// Phase 3.4 only checks revision equality. It cannot distinguish "operator
// changed the spec" from "this is the first deploy ever" without the
// lastDeployedRevision being set by a prior deploy.
func detectRevisionDrift(desiredRevision, lastDeployedRevision string) DriftState {
	if lastDeployedRevision == "" {
		return DriftStateUnknown
	}
	if desiredRevision == lastDeployedRevision {
		return DriftStateNoDrift
	}
	// Revision mismatch: spec was updated since last deploy.
	return DriftStateSuspected
}

// isForwardProgressTransition reports whether a status transition from
// previousStatus to newStatus represents plausible forward progress.
//
// A "regressive" transition is one that cannot happen under normal provider
// lifecycle semantics — e.g., a deployed running service reporting "creating"
// or "pending". These are stale-read candidates.
//
// Returns false for known regression patterns; true for all other transitions.
// The reconciler uses this to route suspicious observations through the
// suspicion counter before accepting them as truth.
//
// Conservative by design: only explicitly known regressions are blocked.
// Unknown transitions are allowed through (fail-open). This prevents the
// reconciler from masking real provider state changes that do not fit the
// expected pattern.
func isForwardProgressTransition(previousStatus, newStatus string) bool {
	if previousStatus == newStatus {
		return true
	}
	type pair struct{ from, to string }
	knownRegressions := map[pair]bool{
		// A running resource cannot un-deploy itself back to pending/creating.
		{"running", "creating"}: true,
		{"running", "pending"}:  true,
		// A deleted resource cannot resurface.
		{"deleted", "running"}:   true,
		{"deleted", "updating"}:  true,
		{"deleted", "creating"}:  true,
		{"deleted", "pending"}:   true,
		{"deleted", "error"}:     true,
		{"deleted", "deleting"}:  true,
		{"deleted", "ambiguous"}: true,
		// Rolled-back to creating/pending is a regression.
		{"rolled_back", "creating"}: true,
		{"rolled_back", "pending"}:  true,
	}
	return !knownRegressions[pair{previousStatus, newStatus}]
}

// isLikelyStaleRead reports whether a status transition is likely a stale
// provider observation rather than a real state change.
//
// Phase 3.4 stale-read heuristic: a "running → updating" observation from
// GetStatus when there is no in-flight operation is suspicious. ECS and
// Cloud Run both have eventual-consistency windows where a prior deployment's
// "IN_PROGRESS" rolloutState can persist briefly after the service is healthy.
//
// This is a soft suspicion signal (routed through the suspicion counter),
// not a hard block. The reconciler accepts the observation once the
// per-provider threshold is reached.
func isLikelyStaleRead(previousStatus, newStatus string) bool {
	type pair struct{ from, to string }
	stalePatterns := map[pair]bool{
		// running→updating with no dispatch in flight: likely ECS/Cloud Run lag.
		{"running", "updating"}: true,
		// rolled_back→updating immediately after rollback: likely stale view.
		{"rolled_back", "updating"}: true,
	}
	return stalePatterns[pair{previousStatus, newStatus}]
}
