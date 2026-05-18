package verifier

import (
	"fmt"
	"strings"
	"time"
)

// successStates are provider states that indicate a successfully running deployment.
// The verifier normalizes all provider states to lowercase before comparison.
var successStates = map[string]bool{
	"running": true, "healthy": true, "active": true,
	"serving": true, "succeeded": true, "completed": true,
	"ready": true, "available": true, "online": true,
}

// failureStates are provider states that indicate a definitively failed deployment.
var failureStates = map[string]bool{
	"failed": true, "error": true, "crashed": true,
	"stopped": true, "deleted": true, "timeout": true,
	"terminated": true, "decommissioned": true, "killed": true,
	"rejected": true,
}

// transitionalStates are provider states during in-progress operations.
// They are not success or failure — they require continued observation.
var transitionalStates = map[string]bool{
	"pending": true, "creating": true, "updating": true,
	"deploying": true, "starting": true, "provisioning": true,
	"draining": true, "stopping": true, "deleting": true,
}

// isSuccessState returns true if the normalized state indicates success.
func isSuccessState(state string) bool {
	return successStates[strings.ToLower(state)]
}

// isFailureState returns true if the normalized state indicates failure.
func isFailureState(state string) bool {
	return failureStates[strings.ToLower(state)]
}

// isTransitionalState returns true if the state indicates in-progress work.
func isTransitionalState(state string) bool {
	return transitionalStates[strings.ToLower(state)]
}

// areContradictoryStates returns true when stateA and stateB cannot both be
// true at the same time in a well-behaved provider lifecycle.
//
// Examples of contradictory pairs:
//   - success then failure (regression after confirmed success)
//   - deleted then running (resurrection without redeploy)
//   - failed then healthy (recovery without operator action)
func areContradictoryStates(stateA, stateB string) bool {
	a := strings.ToLower(stateA)
	b := strings.ToLower(stateB)

	// Success → failure is a regression (contradictory).
	if isSuccessState(a) && isFailureState(b) {
		return true
	}
	// Deleted → success is a resurrection (contradictory).
	if a == "deleted" && isSuccessState(b) {
		return true
	}
	// Failed → healthy without operator action is contradictory.
	if a == "failed" && (b == "healthy" || b == "running" || b == "serving") {
		return true
	}
	return false
}

// DetectContradictions scans the timeline for temporally inconsistent observations.
// A contradiction is any pair (obs[i], obs[j], i < j) where obs[j] contradicts obs[i].
//
// Contradictions are returned in detection order. An empty slice means the timeline
// is internally consistent (but not necessarily verified).
//
// Rules:
//   - A success state followed by a failure state is always a contradiction.
//   - "deleted" followed by any success state is a contradiction.
//   - "failed" followed by "healthy"|"running"|"serving" without an intervening
//     redeploy is a contradiction.
//   - Transitional states are NOT contradictory — they represent normal lifecycle.
func DetectContradictions(tl ObservationTimeline) []Contradiction {
	var contradictions []Contradiction

	for i := 0; i < len(tl.Observations); i++ {
		for j := i + 1; j < len(tl.Observations); j++ {
			a := tl.Observations[i]
			b := tl.Observations[j]

			if areContradictoryStates(a.ProviderState, b.ProviderState) {
				cid := fmt.Sprintf("contradiction:%s:%s:%s:%d-%d",
					tl.ExecutionID, tl.StageID, tl.NodeID, i, j)
				contradictions = append(contradictions, Contradiction{
					ContradictionID: cid,
					ObservationA:    a,
					ObservationB:    b,
					Reason: fmt.Sprintf("state regression: observation[%d]=%q then observation[%d]=%q",
						i, a.ProviderState, j, b.ProviderState),
					DetectedAt: time.Now().UTC().Format(time.RFC3339),
				})
				// Mark the later observation.
				tl.Observations[j].ContradictionFlag = true
			}
		}
	}

	return contradictions
}

// IsConsistent returns true when the timeline has zero contradictions.
// An empty timeline is considered consistent (no evidence of inconsistency).
func IsConsistent(tl ObservationTimeline) bool {
	return len(DetectContradictions(tl)) == 0
}

// LatestProviderState returns the normalized ProviderState from the most recent
// non-stale observation, or ("", false) if no non-stale observations exist.
func LatestProviderState(tl ObservationTimeline) (string, bool) {
	for i := len(tl.Observations) - 1; i >= 0; i-- {
		obs := tl.Observations[i]
		if !obs.StaleFlag {
			return strings.ToLower(obs.ProviderState), true
		}
	}
	return "", false
}

// ConvergenceVerdict classifies the convergence state from the latest non-stale
// provider state, independent of the stabilization window.
//
// Returns:
//   - "success"      when the latest state is a success state.
//   - "failure"      when the latest state is a failure state.
//   - "transitional" when the provider is still in a transitional state.
//   - "unknown"      when the state cannot be interpreted.
func ConvergenceVerdict(tl ObservationTimeline) string {
	state, ok := LatestProviderState(tl)
	if !ok {
		return "unknown"
	}
	switch {
	case isSuccessState(state):
		return "success"
	case isFailureState(state):
		return "failure"
	case isTransitionalState(state):
		return "transitional"
	default:
		return "unknown"
	}
}
