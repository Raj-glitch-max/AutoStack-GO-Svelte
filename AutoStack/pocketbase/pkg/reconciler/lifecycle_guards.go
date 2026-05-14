package reconciler

// lifecycle_guards.go implements Phase 3.2 lifecycle transition enforcement
// for deployment_targets.status.
//
// This replaces the stub comment "a tripwire, not a full state machine.
// Refining it is Phase 2 work" in cloud.go. The IsAllowedTargetTransition
// function is now the authoritative transition table for target lifecycle.
//
// Design reference: phase3/lifecycle-normalization-model.md N-1..N-8 and
// phase3/workflow-lifecycle-contracts.md.
//
// Model:
//
//	Target lifecycle states and their allowed successors:
//
//	pending      — initial state when target is created
//	creating     — provider resource creation in flight
//	updating     — provider resource update in flight
//	running      — provider confirmed healthy
//	error        — provider returned failure or reconciler gave up
//	deleting     — destroy operation in flight
//	deleted      — destroy confirmed (terminal)
//	rolling_back — rollback operation in flight (Phase 3.3)
//	rolled_back  — rollback confirmed (Phase 3.3)
//	partial      — partial success; some resources up (Phase 3.2)
//	ambiguous    — state unknown; reconciler polling (Phase 3.2)
//
// Key contracts:
//
//	deleted is unconditionally terminal. No transition out.
//	rolled_back may transition back to pending (operator re-deploy).
//	error may transition to pending (operator-initiated retry) or deleting.
//	running may transition to updating (new deploy) or deleting.
//	ambiguous may resolve to running, error, or deleted on the next poll.

// targetTransitions is the authoritative allowed-transition map for
// deployment_targets.status. Any transition not listed is forbidden.
// Same-state (no-op) transitions are always permitted implicitly.
//
// Philosophy: the map is PERMISSIVE for forward progress and RESTRICTIVE
// for regressions. A "pending" observation from the provider while the
// target is "running" is refused — that is a provider flap, not a valid
// state regression. The dispatcher's claimTarget CAS handles contention.
var targetTransitions = map[string][]string{
	"pending": {
		"creating",   // dispatcher claims and begins deploy
		"error",      // spec-parse failure before claim (markTargetError)
		"deleting",   // rare: deleted before first deploy attempt
	},
	"creating": {
		"updating",   // deploy succeeded; awaiting GetStatus convergence
		"running",    // deploy succeeded and GetStatus confirmed healthy immediately
		"error",      // deploy failed
		"deleting",   // pending_destroy was set during deploy; rerouted post-success
		"partial",    // partial success (Phase 3.2)
		"ambiguous",  // provider silent during creation
	},
	"updating": {
		"running",    // GetStatus confirms healthy after update
		"error",      // GetStatus or suspicion says error
		"deleting",   // pending_destroy rerouted post-success
		"partial",    // partial success (Phase 3.2)
		"ambiguous",  // provider silent during update
	},
	"running": {
		"updating",   // new deploy dispatched
		"error",      // GetStatus returned error (after suspicion threshold)
		"deleting",   // destroy intent
		"rolling_back", // rollback initiated (Phase 3.3)
		"ambiguous",  // provider not responding
	},
	"error": {
		"pending",    // operator-initiated retry (UI sets status=pending)
		"deleting",   // operator-initiated delete of errored target
		"creating",   // reconciler retry on next dispatch (auto-retry path, Phase 3+)
	},
	"deleting": {
		"deleted",    // destroy confirmed
		"error",      // destroy failed
		"ambiguous",  // destroy confirmation timed out (S-4)
	},
	"rolling_back": {
		"rolled_back", // rollback confirmed
		"error",       // rollback failed
		"ambiguous",   // provider silent during rollback
	},
	"rolled_back": {
		"pending",    // operator re-deploy after rollback
		"deleting",   // operator delete after rollback
		"running",    // GetStatus confirms rolled-back revision is healthy
		"updating",   // GetStatus sees convergence still in progress
		"error",      // GetStatus sees failure even at rolled-back revision
		"ambiguous",  // GetStatus sees ambiguous state during convergence
	},
	"partial": {
		"running",    // partial resolved to fully running
		"error",      // partial failed further
		"rolling_back", // operator rollback from partial state
		"deleting",   // operator delete of partial deployment
		"ambiguous",
	},
	"ambiguous": {
		"running",    // next poll resolved positively
		"error",      // next poll resolved negatively
		"deleted",    // next poll confirmed deletion
		"deleting",   // destroy in progress; ambiguity was during drain
	},
	// Terminal state: no transitions out.
	"deleted": {},
}

// IsAllowedTargetTransition returns true if the from→to transition is
// valid per targetTransitions. Empty from (first observation) permits any
// target state. Identical from/to (no-op) is always permitted.
//
// This is the Phase 3.2 replacement of the Phase 2 stub isAllowedTransition
// in cloud.go. The old function is kept as an alias for backwards compatibility
// within the package but delegates here.
func IsAllowedTargetTransition(from, to string) bool {
	if from == "" || from == to {
		return true
	}
	allowed, known := targetTransitions[from]
	if !known {
		// Unknown from-state: conservative permissive to handle forward-compat.
		// Log; don't block.
		return true
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// IsTerminalTargetStatus returns true when the status is a terminal state
// from which no operational transition is valid (beyond operator actions
// like explicit re-deploy, which set status=pending via API).
func IsTerminalTargetStatus(status string) bool {
	if status == "deleted" {
		return true
	}
	// ambiguous and rolled_back are not permanently terminal — they can
	// resolve in subsequent cycles or via operator action.
	return false
}

// ForbiddenTargetTransitionReason returns a human-readable explanation for
// why a given from→to transition is forbidden. Used in log messages and
// in the operator-facing history row for TRANSITION_REFUSED events.
func ForbiddenTargetTransitionReason(from, to string) string {
	switch {
	case from == "deleted":
		return "deleted is a terminal state; no transitions permitted"
	case from == "running" && (to == "pending" || to == "creating"):
		return "running→" + to + " refused: single provider observation cannot regress a healthy state"
	case from == "updating" && (to == "pending" || to == "creating"):
		return "updating→" + to + " refused: update in progress; provider flap not accepted as regression"
	case to == "pending" && from != "error" && from != "rolled_back":
		return from + "→pending refused: only error and rolled_back may return to pending"
	default:
		return from + "→" + to + " is not in the allowed transition table"
	}
}
