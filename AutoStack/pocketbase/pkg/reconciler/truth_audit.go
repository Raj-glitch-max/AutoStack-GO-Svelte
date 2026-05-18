package reconciler

// Phase 3.9.8 — Periodic Provider Truth Audit.
//
// Runs RunContradictionAudit + RunInvariantAudit on a configurable
// background interval (default: 15 minutes). Findings are persisted as
// incidents via RecordContradictions + RecordViolationsAsIncidents.
//
// ─────────────────────────────────────────────────────────────────────────
// DESIGN INVARIANTS
// ─────────────────────────────────────────────────────────────────────────
//
//   1. DETERMINISTIC ORDERING: audit functions iterate AllContradictions
//      and AllInvariants in stable slice order. Each pass is independent.
//
//   2. NO DUPLICATE INCIDENT SPAM: CreateOrEscalateIncident deduplicates
//      by (target, source). A check that fires on every audit pass produces
//      exactly one incident that escalates monotonically — not N new incidents.
//
//   3. MONOTONIC ESCALATION ONLY: the incident system never de-escalates
//      severity. A finding that resolves on the next pass simply does not
//      produce a new escalation — the existing incident remains open until
//      an operator closes it or RunInvariantAudit stops finding it.
//
//   4. BEST-EFFORT: audit failures are logged but do not crash the runtime
//      or propagate. A broken audit check does not block reconciliation.
//
//   5. CONFIGURABLE INTERVAL: AUTOSTACK_TRUTH_AUDIT_INTERVAL_MINUTES env var
//      (default: 15). Setting to 0 disables the goroutine (useful in test
//      environments where audit DB writes interfere with expected state).
//
// ─────────────────────────────────────────────────────────────────────────
// EXPLICITLY NOT SUPPORTED
// ─────────────────────────────────────────────────────────────────────────
//
//   - Auto-resolution of incidents when a check no longer fires. The audit
//     only opens/escalates — resolution requires operator action.
//   - Distributed coordination: in a multi-pod setup, each pod runs its
//     own audit pass. CreateOrEscalateIncident is idempotent for the same
//     (target, source), so concurrent passes produce the same incident rows.
//     There is no leader-election for the audit goroutine.
//   - Backpressure: if the audit takes longer than the interval, passes
//     overlap. Each pass is independent and non-blocking.

import (
	"log"
	"time"
)

// StartTruthAuditLoop starts the periodic truth audit goroutine. Called from
// Reconciler.Start() when TruthAuditInterval > 0. Listens on stopCh.
//
// The first pass runs after one full interval (not immediately) so the
// startup invariant audit (which runs synchronously before the loop) is
// not immediately followed by a duplicate pass.
func (r *Reconciler) StartTruthAuditLoop() {
	if r.config.TruthAuditInterval <= 0 {
		log.Printf("[TRUTH_AUDIT_DISABLED] TruthAuditInterval=0 — periodic audit not started")
		return
	}
	log.Printf("[TRUTH_AUDIT_READY] interval=%s", r.config.TruthAuditInterval)
	go r.runTruthAuditLoop()
}

func (r *Reconciler) runTruthAuditLoop() {
	ticker := time.NewTicker(r.config.TruthAuditInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			log.Printf("[TRUTH_AUDIT_STOPPED]")
			return
		case <-ticker.C:
			r.RunTruthAuditOnce()
		}
	}
}

// RunTruthAuditOnce runs one full truth audit pass: contradiction checks
// followed by invariant checks. Best-effort — panics are recovered.
// Safe to call directly (demand-only) from HTTP handlers.
func (r *Reconciler) RunTruthAuditOnce() {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[TRUTH_AUDIT_PANIC] %v", rec)
		}
	}()

	startedAt := time.Now()

	// 1. Contradiction audit (provider truth + runtime state divergence).
	contradictions := RunContradictionAudit(r.app)
	if len(contradictions) > 0 {
		log.Printf("[TRUTH_AUDIT_CONTRADICTIONS] found=%d", len(contradictions))
		r.RecordContradictions(contradictions)
	}

	// 2. Invariant audit (runtime integrity checks).
	violations := RunInvariantAudit(r.app)
	if len(violations) > 0 {
		log.Printf("[TRUTH_AUDIT_VIOLATIONS] found=%d", len(violations))
		r.RecordViolationsAsIncidents(violations)
	}

	elapsed := time.Since(startedAt)
	log.Printf("[TRUTH_AUDIT_COMPLETE] elapsed=%s contradictions=%d violations=%d",
		elapsed.Round(time.Millisecond), len(contradictions), len(violations))
}
