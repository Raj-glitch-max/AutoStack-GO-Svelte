package runtime_deploy

import (
	"strings"
	"testing"
	"time"
)

// happyPathEvents returns the event chain of a typical successful deployment.
// The exact shape mirrors what the engine actually produces (verified live
// in §3 of the Phase-6 report).
func happyPathEvents(deploymentID string) []*DeploymentEvent {
	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	step := func(seq int, from, to DeploymentState, typ, detail string) *DeploymentEvent {
		return &DeploymentEvent{
			Sequence:     seq,
			DeploymentID: deploymentID,
			FromState:    from,
			ToState:      to,
			EventType:    typ,
			Detail:       detail,
			OccurredAt:   base.Add(time.Duration(seq) * time.Second),
		}
	}
	return []*DeploymentEvent{
		step(1, "", StatePlanned, "created", "deployment created"),
		step(2, StatePlanned, StateValidating, "engine.validate", ""),
		step(3, StateValidating, StateProvisioning, "engine.pull", ""),
		step(4, StateProvisioning, StateDeploying, "engine.run", ""),
		step(5, StateDeploying, StateVerifying, "engine.verify", ""),
		step(6, StateVerifying, StateStabilized, "engine.probe", ""),
		step(7, StateStabilized, StateCertified, "engine.certify", ""),
	}
}

func TestReplay_HappyPathDeterminism(t *testing.T) {
	events := happyPathEvents("d-1")
	a, err := Replay(events)
	if err != nil {
		t.Fatalf("first replay failed: %v", err)
	}
	b, err := Replay(events)
	if err != nil {
		t.Fatalf("second replay failed: %v", err)
	}
	if a.FinalState != StateCertified {
		t.Fatalf("expected FinalState=certified, got %q", a.FinalState)
	}
	if a.ChainHash != b.ChainHash {
		t.Fatalf("chain hash NOT deterministic: %q vs %q", a.ChainHash, b.ChainHash)
	}
	if a.EventCount != 7 {
		t.Fatalf("expected 7 events, got %d", a.EventCount)
	}
	if len(a.StateHistory) != 7 {
		t.Fatalf("expected 7 state periods, got %d", len(a.StateHistory))
	}
}

func TestReplay_DeterminismAcrossEventOrder(t *testing.T) {
	// Same events, shuffled. Replay sorts by sequence, so the result must
	// match the in-order replay byte-for-byte.
	events := happyPathEvents("d-1")
	shuffled := []*DeploymentEvent{
		events[3], events[0], events[6], events[2], events[5], events[1], events[4],
	}
	ordered, err := Replay(events)
	if err != nil {
		t.Fatalf("ordered replay failed: %v", err)
	}
	jumbled, err := Replay(shuffled)
	if err != nil {
		t.Fatalf("shuffled replay failed: %v", err)
	}
	if ordered.ChainHash != jumbled.ChainHash {
		t.Fatalf("hash differs after shuffle:\n  ordered=%s\n  jumbled=%s", ordered.ChainHash, jumbled.ChainHash)
	}
}

func TestReplay_RejectsSequenceGap(t *testing.T) {
	events := happyPathEvents("d-1")
	events[4].Sequence = 99 // create a gap
	_, err := Replay(events)
	if err == nil {
		t.Fatalf("expected ErrReplayInvalid for sequence gap, got nil")
	}
	if !strings.Contains(err.Error(), "sequence gap") {
		t.Fatalf("expected sequence gap error, got: %v", err)
	}
}

func TestReplay_RejectsImpossibleTransition(t *testing.T) {
	// Replace event #3 with one whose from_state does not match #2's to_state.
	events := happyPathEvents("d-1")
	events[2].FromState = StateCertified // impossible: came after validating
	_, err := Replay(events)
	if err == nil {
		t.Fatalf("expected ErrReplayInvalid for bad transition, got nil")
	}
}

func TestReplay_RollbackChainPreserved(t *testing.T) {
	// A real rollback chain captured from the engine.
	events := happyPathEvents("d-2")
	base := events[len(events)-1].OccurredAt.Add(time.Second)
	addStep := func(seq int, from, to DeploymentState, typ string) *DeploymentEvent {
		return &DeploymentEvent{
			Sequence: seq, DeploymentID: "d-2",
			FromState: from, ToState: to, EventType: typ,
			OccurredAt: base.Add(time.Duration(seq) * time.Second),
		}
	}
	events = append(events,
		addStep(8, StateCertified, StateRollbackPending, "operator.rollback_requested"),
		addStep(9, StateRollbackPending, StateRollingBack, "engine.rollback.run"),
		addStep(10, StateRollingBack, StateRolledBack, "engine.rollback.complete"),
	)
	r, err := Replay(events)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if r.FinalState != StateRolledBack {
		t.Fatalf("expected FinalState=rolled_back, got %q", r.FinalState)
	}
	// The state history must include the rollback transitions in order.
	wantStates := []DeploymentState{
		StatePlanned, StateValidating, StateProvisioning, StateDeploying,
		StateVerifying, StateStabilized, StateCertified,
		StateRollbackPending, StateRollingBack, StateRolledBack,
	}
	if len(r.StateHistory) != len(wantStates) {
		t.Fatalf("expected %d state periods, got %d", len(wantStates), len(r.StateHistory))
	}
	for i, want := range wantStates {
		if r.StateHistory[i].State != want {
			t.Fatalf("state[%d]: got %q want %q", i, r.StateHistory[i].State, want)
		}
	}
}

func TestReplay_HashIgnoresTimestamps(t *testing.T) {
	a := happyPathEvents("d-3")
	b := happyPathEvents("d-3")
	// Shift every timestamp in b by an hour. Hash must be unchanged.
	for _, ev := range b {
		ev.OccurredAt = ev.OccurredAt.Add(time.Hour)
	}
	ra, _ := Replay(a)
	rb, _ := Replay(b)
	if ra.ChainHash != rb.ChainHash {
		t.Fatalf("chain hash sensitive to timestamps: %q vs %q", ra.ChainHash, rb.ChainHash)
	}
}

func TestReplay_FailedChainTerminatesAtFailed(t *testing.T) {
	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	events := []*DeploymentEvent{
		{Sequence: 1, DeploymentID: "d-4", FromState: "", ToState: StatePlanned, EventType: "created", OccurredAt: base},
		{Sequence: 2, DeploymentID: "d-4", FromState: StatePlanned, ToState: StateValidating, EventType: "engine.validate", OccurredAt: base.Add(time.Second)},
		{Sequence: 3, DeploymentID: "d-4", FromState: StateValidating, ToState: StateProvisioning, EventType: "engine.pull", OccurredAt: base.Add(2 * time.Second)},
		{Sequence: 4, DeploymentID: "d-4", FromState: StateProvisioning, ToState: StateFailed, EventType: "engine.fail", Error: "image pull denied", OccurredAt: base.Add(3 * time.Second)},
	}
	r, err := Replay(events)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if r.FinalState != StateFailed {
		t.Fatalf("expected FinalState=failed, got %q", r.FinalState)
	}
	if r.EventCount != 4 {
		t.Fatalf("expected 4 events, got %d", r.EventCount)
	}
}
