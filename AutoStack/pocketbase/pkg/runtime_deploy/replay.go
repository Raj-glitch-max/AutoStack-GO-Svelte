package runtime_deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
)

// ReplayResult is the terminal state of a deployment as reconstructed
// purely from its event chain. No I/O, no driver calls, no clocks.
type ReplayResult struct {
	DeploymentID string          `json:"deployment_id"`
	FinalState   DeploymentState `json:"final_state"`
	StateHistory []StatePeriod   `json:"state_history"`
	EventCount   int             `json:"event_count"`
	ChainHash    string          `json:"chain_hash"` // SHA-256 over normalized event chain
}

// StatePeriod is one interval where the deployment was in a given state.
type StatePeriod struct {
	State    DeploymentState `json:"state"`
	EnteredAt string         `json:"entered_at"`
	ExitedAt  string         `json:"exited_at,omitempty"` // empty for the final state
}

// ErrReplayInvalid is returned when the event chain cannot reconstruct a
// valid lifecycle (gap in sequence, contradictory transition, etc.).
var ErrReplayInvalid = errors.New("runtime_deploy: invalid replay event chain")

// Replay rebuilds the deployment's lifecycle from its event chain. This is
// a PURE function: given the same event slice, it returns the same result
// every time. That's the determinism contract.
//
// Strictness:
//   - Sequence numbers must be a dense 1..N run.
//   - Each event's from_state must equal the previous event's to_state
//     (or be empty on the very first event).
//   - Empty event list returns an error — there is no valid empty lifecycle.
func Replay(events []*DeploymentEvent) (ReplayResult, error) {
	if len(events) == 0 {
		return ReplayResult{}, fmt.Errorf("%w: no events", ErrReplayInvalid)
	}

	// Sort defensively by sequence so caller doesn't need to.
	sorted := make([]*DeploymentEvent, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Sequence < sorted[j].Sequence
	})

	// Validate sequence is 1..N with no gaps or duplicates.
	for i, ev := range sorted {
		expected := i + 1
		if ev.Sequence != expected {
			return ReplayResult{}, fmt.Errorf("%w: sequence gap at index %d: got %d expected %d",
				ErrReplayInvalid, i, ev.Sequence, expected)
		}
	}

	// Walk the chain. Each step's from_state must match the previous to_state.
	periods := make([]StatePeriod, 0, len(sorted))
	var prev DeploymentState
	for i, ev := range sorted {
		if i == 0 {
			if ev.FromState != "" {
				return ReplayResult{}, fmt.Errorf("%w: first event has non-empty from_state %q",
					ErrReplayInvalid, ev.FromState)
			}
		} else if ev.FromState != prev {
			return ReplayResult{}, fmt.Errorf("%w: event #%d from_state %q != previous to_state %q",
				ErrReplayInvalid, ev.Sequence, ev.FromState, prev)
		}
		// State change (or first event) → new period; same state → ignore.
		if i == 0 || ev.ToState != prev {
			if len(periods) > 0 {
				periods[len(periods)-1].ExitedAt = ev.OccurredAt.UTC().Format("2006-01-02T15:04:05.000000000Z")
			}
			periods = append(periods, StatePeriod{
				State:     ev.ToState,
				EnteredAt: ev.OccurredAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
			})
		}
		prev = ev.ToState
	}

	return ReplayResult{
		DeploymentID: sorted[0].DeploymentID,
		FinalState:   prev,
		StateHistory: periods,
		EventCount:   len(sorted),
		ChainHash:    HashEventChain(sorted),
	}, nil
}

// HashEventChain produces a SHA-256 over the sequence-ordered (from, to, type)
// tuples. Timestamps are EXCLUDED so two replays of the same logical chain
// produce the same hash regardless of when they're run. This is the
// content-identity property documented in docs/replay-certification.md.
func HashEventChain(events []*DeploymentEvent) string {
	h := sha256.New()
	for _, ev := range events {
		fmt.Fprintf(h, "%d|%s|%s|%s\n", ev.Sequence, ev.FromState, ev.ToState, ev.EventType)
	}
	return hex.EncodeToString(h.Sum(nil))
}
