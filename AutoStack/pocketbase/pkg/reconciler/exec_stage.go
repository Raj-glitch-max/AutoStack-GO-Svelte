package reconciler

// exec_stage.go implements the Phase 3.2 execution-state model.
//
// The ExecStage type describes the fine-grained stage of an in-flight
// operation (operations table, exec_stage column). It is orthogonal to
// deployment_targets.status: the target status is what the WORLD sees;
// the exec stage is what the DISPATCHER is doing.
//
// The model is authoritative for:
//   - Which transitions are allowed at which exec stage
//   - What the reconciler must do on process restart (replay semantics)
//   - What timeout applies at each stage
//   - Which owner is responsible for the target during each stage
//
// Design contract: phase3/lifecycle-normalization-model.md N-1..N-8 and
// the transition table in phase3/workflow-lifecycle-contracts.md.
//
// The 13 canonical exec states are:
//
//	QUEUED       — operation row created; CAS claim not yet attempted.
//	DISPATCHING  — CAS claim succeeded; about to call provider.
//	PROVISIONING — provider is creating/updating infrastructure resources.
//	CONFIGURING  — provider resources created; applying config (env, secrets).
//	VERIFYING    — waiting for the provider's readiness signal.
//	READY        — provider confirmed running; about to release target.
//	PARTIAL      — some resources deployed; others failed (ambiguous success).
//	FAILED       — operation reached a terminal failure.
//	ROLLING_BACK — rollback operation in flight; provider call in progress.
//	ROLLED_BACK  — rollback completed; target back at prior revision.
//	DELETING     — destroy operation in flight; provider call in progress.
//	DELETED      — destroy confirmed; target row at terminal state.
//	AMBIGUOUS    — operation timed out or confirmation window elapsed; state unclear.
//
// Note on PROVISIONING vs. CONFIGURING vs. VERIFYING:
//
// Most Phase 3.1 providers (Cloud Run, ECS) execute these stages atomically
// inside Provider.Deploy(). The dispatcher cannot observe sub-stage granularity
// from outside the provider call. Therefore, Phase 3.2 maps them to a single
// in-progress stage from the dispatcher's perspective:
//
//	before Deploy():    DISPATCHING → PROVISIONING
//	after Deploy() ok:  PROVISIONING → VERIFYING
//	after GetStatus ok: VERIFYING → READY
//
// CONFIGURING is reserved for Phase 3.3+ providers that expose a config
// application phase (e.g., secrets injection, service mesh config). It
// is defined here so the type system is complete, but not yet used.
//
// PARTIAL is reserved for Phase 3.2+ partial-success handling. Currently
// no provider returns partial success — all outcomes are atomic success
// or failure. The state is defined so the dispatch path can be extended
// without an interface change.

import (
	"fmt"
	"log"
	"time"

	"github.com/pocketbase/dbx"
	pb "github.com/pocketbase/pocketbase"
)

// ExecStage is the fine-grained stage of an in-flight operation.
type ExecStage string

const (
	ExecStageQueued      ExecStage = "queued"
	ExecStageDispatching ExecStage = "dispatching"
	ExecStageProvisioning ExecStage = "provisioning"
	ExecStageConfiguring ExecStage = "configuring"
	ExecStageVerifying   ExecStage = "verifying"
	ExecStageReady       ExecStage = "ready"
	ExecStagePartial     ExecStage = "partial"
	ExecStageFailed      ExecStage = "failed"
	ExecStageRollingBack ExecStage = "rolling_back"
	ExecStageRolledBack  ExecStage = "rolled_back"
	ExecStageDeleting    ExecStage = "deleting"
	ExecStageDeleted     ExecStage = "deleted"
	ExecStageAmbiguous   ExecStage = "ambiguous"

	// Phase 3.9.2 — Lease-related execution stages.
	//
	// These stages express ownership state explicitly so operators can
	// distinguish "the reconciler did not run a provider call because it
	// lost its lease" from "the reconciler called the provider and got
	// an error". They are recorded on operations.exec_stage just like
	// any other stage, so incident reconstruction surfaces them.
	//
	// ExecStageLeaseAcquiring — operation is waiting on AcquireLease.
	//   Reached: after createOperation, before claimTarget.
	//   Exit: ExecStageDispatching (acquired) | ExecStageFailed (denied).
	ExecStageLeaseAcquiring ExecStage = "lease_acquiring"

	// ExecStageLeaseLost — the lease was verified to be no longer held
	// by this pod (verify call returned false) BEFORE a provider
	// mutation was issued. The operation MUST NOT proceed.
	//   Reached: any time VerifyLease returns false during dispatch.
	//   Exit: terminal. No outgoing transitions.
	ExecStageLeaseLost ExecStage = "lease_lost"

	// ExecStageInterruptedUnknownOutcome — the lease was lost AFTER a
	// provider mutation was issued but BEFORE the reconciler could
	// commit success. The cloud-side resource may exist; the reconciler
	// CANNOT claim success or failure. Operators must investigate.
	//   Reached: from PROVISIONING/DELETING/ROLLING_BACK when lease lost.
	//   Exit: terminal. Forensic-only.
	ExecStageInterruptedUnknownOutcome ExecStage = "interrupted_unknown_outcome"

	// ExecStageReclaimPending — sweep has identified an expired-lease op
	// and is about to flip it to failed. Transient operational stage.
	//   Reached: from any in-flight stage when sweep selects the op.
	//   Exit: ExecStageReclaimVerified | ExecStageFailed.
	ExecStageReclaimPending ExecStage = "reclaim_pending"

	// ExecStageReclaimVerified — sweep completed the reclaim and the
	// operation row has been transitioned to failed. The target is
	// released to error.
	//   Reached: from ExecStageReclaimPending after sweep commits.
	//   Exit: terminal.
	ExecStageReclaimVerified ExecStage = "reclaim_verified"
)

// execStageTransitions is the authoritative allowed-transition table for
// operation exec stages. A stage not listed here forbids all transitions
// (terminal stage). Identical (no-op) transitions are always permitted.
//
// Deploy path:      queued → dispatching → provisioning → verifying → ready | failed | ambiguous
// Destroy path:     queued → dispatching → deleting → deleted | failed | ambiguous
// Rollback path:    queued → dispatching → rolling_back → rolled_back | failed | ambiguous
// Partial success:  provisioning → partial → verifying | failed
// Configuring (future): provisioning → configuring → verifying
var execStageTransitions = map[ExecStage][]ExecStage{
	ExecStageQueued: {
		ExecStageLeaseAcquiring, // Phase 3.9.2: acquire lease before claim
		ExecStageDispatching,
		ExecStageFailed, // early abort before CAS
	},
	ExecStageLeaseAcquiring: {
		ExecStageDispatching,
		ExecStageLeaseLost, // lease denied or stolen
		ExecStageFailed,
	},
	ExecStageDispatching: {
		ExecStageProvisioning,
		ExecStageRollingBack,
		ExecStageDeleting,
		ExecStageLeaseLost,                  // Phase 3.9.2: verify failed pre-dispatch
		ExecStageInterruptedUnknownOutcome,  // Phase 3.9.2: verify failed post-dispatch
		ExecStageFailed,                     // CAS lost or claim error
	},
	ExecStageProvisioning: {
		ExecStageConfiguring,
		ExecStageVerifying,
		ExecStagePartial,
		ExecStageFailed,
		ExecStageAmbiguous,
		ExecStageInterruptedUnknownOutcome, // Phase 3.9.2: lease lost mid-mutation
	},
	ExecStageConfiguring: {
		ExecStageVerifying,
		ExecStageFailed,
		ExecStageAmbiguous,
		ExecStageInterruptedUnknownOutcome,
	},
	ExecStageVerifying: {
		ExecStageReady,
		ExecStageFailed,
		ExecStageAmbiguous,
		ExecStageInterruptedUnknownOutcome,
	},
	ExecStagePartial: {
		ExecStageVerifying,
		ExecStageFailed,
		ExecStageRollingBack,
	},
	ExecStageRollingBack: {
		ExecStageRolledBack,
		ExecStageFailed,
		ExecStageAmbiguous,
		ExecStageInterruptedUnknownOutcome,
	},
	ExecStageDeleting: {
		ExecStageDeleted,
		ExecStageFailed,
		ExecStageAmbiguous,
		ExecStageInterruptedUnknownOutcome,
	},
	ExecStageReclaimPending: {
		ExecStageReclaimVerified,
		ExecStageFailed,
	},
	// Terminal stages — no transitions out.
	ExecStageReady:                     {},
	ExecStageRolledBack:                {},
	ExecStageDeleted:                   {},
	ExecStageFailed:                    {},
	ExecStageAmbiguous:                 {},
	ExecStageLeaseLost:                 {},
	ExecStageInterruptedUnknownOutcome: {},
	ExecStageReclaimVerified:           {},
}

// execStageOwner documents which component owns the target at each stage.
// This is a documentation contract, not a runtime enforcement (enforcement
// is via the CAS on operations.current_operation).
var execStageOwner = map[ExecStage]string{
	ExecStageQueued:       "reconciler (pre-claim; operation row created)",
	ExecStageDispatching:  "reconciler (CAS claimed; provider call imminent)",
	ExecStageProvisioning: "provider (Deploy in flight)",
	ExecStageConfiguring:  "provider (config phase in flight)",
	ExecStageVerifying:    "reconciler (GetStatus polling)",
	ExecStageReady:        "reconciler (releasing to running)",
	ExecStagePartial:      "reconciler (partial-success decision point)",
	ExecStageFailed:       "terminal",
	ExecStageRollingBack:  "provider (Rollback in flight)",
	ExecStageRolledBack:   "terminal",
	ExecStageDeleting:     "provider (Destroy in flight)",
	ExecStageDeleted:      "terminal",
	ExecStageAmbiguous:                 "reconciler (confirmation window expired)",
	ExecStageLeaseAcquiring:            "reconciler (acquiring lease before claim)",
	ExecStageLeaseLost:                 "terminal (no provider mutation occurred)",
	ExecStageInterruptedUnknownOutcome: "terminal (provider mutation may have committed; outcome unknown)",
	ExecStageReclaimPending:            "sweep (about to reclaim expired-lease op)",
	ExecStageReclaimVerified:           "terminal (sweep completed reclaim)",
}

// execStageReplaySemantics documents how the reconciler handles process
// restart at each stage. This is the truthful replay contract.
var execStageReplaySemantics = map[ExecStage]string{
	ExecStageQueued:       "sweep marks op failed; target returns to error; operator re-triggers",
	ExecStageDispatching:  "sweep marks op failed; target returns to error; operator re-triggers",
	ExecStageProvisioning: "sweep marks op failed; target to error; next dispatch calls idempotent Deploy()",
	ExecStageConfiguring:  "sweep marks op failed; target to error; next dispatch retries from Deploy()",
	ExecStageVerifying:    "sweep marks op failed; target to error; provider resource may already be running",
	ExecStageReady:        "sweep marks op failed; target to error; GetStatus on next tick corrects if provider is running",
	ExecStagePartial:      "sweep marks op failed; target to error; partial resources orphaned until OC-1 scanner runs",
	ExecStageFailed:       "terminal; no replay needed",
	ExecStageRollingBack:  "sweep marks op failed; target to error; provider may have partially rolled back",
	ExecStageRolledBack:   "terminal; no replay needed",
	ExecStageDeleting:     "sweep marks op failed; target to error; next Destroy() is idempotent on NOT_FOUND/INACTIVE",
	ExecStageDeleted:      "terminal; no replay needed",
	ExecStageAmbiguous:                 "terminal for the op; target ambiguity columns remain; reconciler polls next cycle",
	ExecStageLeaseAcquiring:            "sweep marks op failed; target back to error; lease eventually expires and is reclaimed by next acquirer",
	ExecStageLeaseLost:                 "terminal; no provider mutation occurred; safe to re-dispatch on next tick",
	ExecStageInterruptedUnknownOutcome: "operator MUST investigate; cloud resource may or may not exist; next GetStatus tick will surface real state",
	ExecStageReclaimPending:            "transient; sweep will commit ReclaimVerified within one sweep cycle",
	ExecStageReclaimVerified:           "terminal; reclaim is auditable via the reclaim history row",
}

// IsTerminalExecStage returns true when the exec stage is a terminal state
// from which no further transitions are possible within the operation.
// A terminal stage means the operation row is complete; the target may
// still be polled by subsequent reconcile cycles.
func IsTerminalExecStage(s ExecStage) bool {
	targets, ok := execStageTransitions[s]
	return ok && len(targets) == 0
}

// IsAllowedExecStageTransition returns true if the from→to transition is
// valid per execStageTransitions. Identical transitions always return true.
// Unknown from-stages default to permissive (graceful forward-compatibility).
func IsAllowedExecStageTransition(from, to ExecStage) bool {
	if from == to {
		return true
	}
	allowed, known := execStageTransitions[from]
	if !known {
		return true // unknown stage: allow, log elsewhere
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// setExecStage writes the exec_stage column on the operations row.
// Validates the transition before writing. A forbidden transition is logged
// but NOT written — the op stays at its prior stage. This prevents the
// stage from lying about where an operation is.
//
// Best-effort: DB errors are logged but do not propagate — caller continues.
func setExecStage(app *pb.PocketBase, opID string, from, to ExecStage) {
	if !IsAllowedExecStageTransition(from, to) {
		log.Printf("[EXEC_STAGE_FORBIDDEN] op=%s from=%s to=%s — transition refused", opID, from, to)
		return
	}
	_, err := app.Dao().DB().NewQuery(
		"UPDATE operations SET exec_stage = {:stage}, updated_at = {:ts} " +
			"WHERE id = {:id} AND (exec_stage = {:from} OR exec_stage IS NULL OR exec_stage = '')",
	).Bind(dbx.Params{
		"stage": string(to),
		"ts":    time.Now().UTC().Format(time.RFC3339),
		"id":    opID,
		"from":  string(from),
	}).Execute()
	if err != nil {
		log.Printf("[EXEC_STAGE_ERR] op=%s from=%s to=%s err=%v", opID, from, to, err)
		return
	}
	log.Printf("[EXEC_STAGE] op=%s from=%s to=%s", opID, from, to)
}

// execStageOwnerOf returns the owner description for a given exec stage,
// for use in operator-visible logs and lineage.
func execStageOwnerOf(s ExecStage) string {
	if o, ok := execStageOwner[s]; ok {
		return o
	}
	return "unknown"
}

// validateExecStageModel panics at startup if the transition table has
// obvious consistency errors. Called from a test and from init to guard
// against table corruption during development.
func validateExecStageModel() error {
	// Every stage referenced as a target must itself exist as a key.
	for from, targets := range execStageTransitions {
		for _, to := range targets {
			if _, ok := execStageTransitions[to]; !ok {
				return fmt.Errorf("exec stage transition table: %s → %s references unknown target stage", from, to)
			}
		}
	}
	// Every stage in the owner map must be in the transition table.
	for stage := range execStageOwner {
		if _, ok := execStageTransitions[stage]; !ok {
			return fmt.Errorf("exec stage owner map: %s not in transition table", stage)
		}
	}
	return nil
}

func init() {
	if err := validateExecStageModel(); err != nil {
		panic("exec stage model invalid: " + err.Error())
	}
}
