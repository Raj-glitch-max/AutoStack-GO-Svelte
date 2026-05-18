package verifier

import (
	"fmt"
	"time"
)

// NewObservation constructs a single provider observation with a stable ID.
// The ID format is: "obs:{executionID}:{stageID}:{nodeID}:{unix-nano}"
// This format is stable and replay-safe.
func NewObservation(
	executionID, stageID, nodeID string,
	providerState, nativeState string,
	trust TrustLevel,
	source string,
) ProviderObservation {
	now := time.Now().UTC()
	id := fmt.Sprintf("obs:%s:%s:%s:%d", executionID, stageID, nodeID, now.UnixNano())
	return ProviderObservation{
		ObservationID:          id,
		ExecutionID:            executionID,
		StageID:                stageID,
		NodeID:                 nodeID,
		ObservedAt:             now.Format(time.RFC3339Nano),
		ProviderState:          providerState,
		NativeState:            nativeState,
		TrustLevel:             trust,
		VerificationSource:     source,
		ConfidenceContribution: trustToWeight(trust),
	}
}

// NewObservationTimeline constructs an empty timeline for the given node.
func NewObservationTimeline(executionID, stageID, nodeID string) ObservationTimeline {
	return ObservationTimeline{
		ExecutionID: executionID,
		StageID:     stageID,
		NodeID:      nodeID,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

// AppendObservation adds an observation to the timeline.
// Observations are stored in the order they are appended.
// The caller is responsible for ordering by ObservedAt before calling.
func AppendObservation(tl *ObservationTimeline, obs ProviderObservation) {
	tl.Observations = append(tl.Observations, obs)
}

// IsStale returns true if the observation's age exceeds maxStaleness relative to refTime.
// Using explicit refTime makes the check deterministic and testable.
func IsStale(obs ProviderObservation, refTime time.Time, maxStaleness time.Duration) bool {
	observedAt, err := time.Parse(time.RFC3339Nano, obs.ObservedAt)
	if err != nil {
		// Try RFC3339 (without nanoseconds) as fallback.
		observedAt, err = time.Parse(time.RFC3339, obs.ObservedAt)
		if err != nil {
			return true // unparseable timestamp is treated as stale
		}
	}
	return refTime.Sub(observedAt) > maxStaleness
}

// MarkStaleObservations updates the StaleFlag on each observation in the timeline
// that exceeds maxStaleness relative to refTime.
// This modifies the timeline in place — call before running the verification engine.
func MarkStaleObservations(tl *ObservationTimeline, refTime time.Time, maxStaleness time.Duration) {
	for i := range tl.Observations {
		tl.Observations[i].StaleFlag = IsStale(tl.Observations[i], refTime, maxStaleness)
	}
}

// CountStale returns how many observations in the timeline are marked stale.
func CountStale(tl ObservationTimeline) int {
	n := 0
	for _, obs := range tl.Observations {
		if obs.StaleFlag {
			n++
		}
	}
	return n
}

// AllStale returns true if every observation in the timeline is stale.
// Returns false for an empty timeline.
func AllStale(tl ObservationTimeline) bool {
	if len(tl.Observations) == 0 {
		return false
	}
	for _, obs := range tl.Observations {
		if !obs.StaleFlag {
			return false
		}
	}
	return true
}

// LatestObservation returns the last observation in the timeline, or false if empty.
func LatestObservation(tl ObservationTimeline) (ProviderObservation, bool) {
	if len(tl.Observations) == 0 {
		return ProviderObservation{}, false
	}
	return tl.Observations[len(tl.Observations)-1], true
}

// trustToWeight maps a TrustLevel to a contribution weight in [0.0, 1.0].
func trustToWeight(trust TrustLevel) float64 {
	switch trust {
	case TrustHigh:
		return 1.0
	case TrustMedium:
		return 0.6
	case TrustLow:
		return 0.3
	default:
		return 0.5
	}
}
