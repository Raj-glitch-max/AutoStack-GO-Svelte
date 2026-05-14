# Phase 2 Finalization — Forensic Reconstruction Assessment

**Last Updated:** 2026-05-14

## Question

Can an operator reconstruct a complete deployment timeline using ONLY
AutoStack's logs and `deployment_history` table?

---

## Reconstruction Paths

### Path A: Normal successful deploy

**What happened:** Target P created, rollout propagated, dispatch ran, deploy succeeded, poll promoted to running.

**Reconstruction:**

```
deployment_history: action=created, status=in_progress, target=P
deployment_history: action=created, status=success, target=P
deployment_history: (next cycle) action=updated, status=in_progress, target=P
deployment_history: action=updated, status=success, target=P

logs:
  [DISPATCH_CLAIM] target=P operation=opXYZ action=created
  [DEPLOY_START] target=P operation=opXYZ image=...
  [DEPLOY_END] target=P operation=opXYZ duration_ms=XXX err=<nil>
  [HISTORY_WRITE] target=P action=created status=success
  [STATE_TRANSITION] target=P from=creating to=updating
  [RECONCILE_TARGET_COMPLETE] target=P status=updating
  (next cycle)
  [STATE_TRANSITION] target=P from=updating to=running
```

**Completeness:** ✅ Full timeline, operation duration, without guesswork.

---

### Path B: Deploy succeeded but rollout moved mid-flight (succeeded_stale)

**Reconstruction:**

```
deployment_history: action=created, status=in_progress, target=P
deployment_history: action=created, status=failed, target=P, message=stale spec

logs:
  [DISPATCH_CLAIM] target=P
  [DEPLOY_START] target=P
  [DEPLOY_END] target=P err=<nil>
  [STALE] target=P rollout moved during deploy, stale_count=1
  [OP_COMPLETE] op=opXYZ status=succeeded_stale
  [RELEASE_LOST_OWNERSHIP] (if sweep won) or [STATE_TRANSITION] creating→pending
```

**Completeness:** ✅ Clear that deploy succeeded but is untrusted. Stale count is visible in logs for each retry.

---

### Path C: Deploy failed (Hard error)

**Reconstruction:**

```
deployment_history: action=created, status=in_progress, target=P
deployment_history: action=created, status=failed, target=P, message=<sanitized>

logs:
  [DEPLOY_START] target=P
  [DEPLOY_END] target=P err=failed to create Cloud Run service:
  [FAILURE] target=P category=permanent message=failed to create Cloud Run
  [OP_COMPLETE] op=opXYZ status=failed
  [STATE_TRANSITION] target=P from=creating to=error
  [CIRCUIT_OPEN] after 5 failures
```

**Completeness:** ✅ Error message sanitized, failure category readable. Origin of the issue is recoverable.

---

### Path D: Crash mid-deploy → sweep reclaims

**Reconstruction:**

```
Operations row: status=failed, message=abandoned: process restart
deployment_targets: status=error, current_operation=''
deployment_history (sweep): action=error, status=failed, message=abandoned: process restart

logs: (on restart)
  [STARTUP_SWEEP] found N abandoned in_progress operations
  [OP_ABANDONED] operation=opXYZ target=P kind=deploy
  [STARTUP_SWEEP_HISTORY_ERR] or [HISTORY_WRITE] (writeAbandonHistory)
```

**Completeness:** ✅ Sweep behavior is logged separately. The fact that the deployment was abandoned is reconstructable. The ambiguity is: **we don't know if the Cloud Run service actually got created**. AutoStack honestly refuses to guess. An operator must check GCP console.

---

### Path E: Destroy succeeded but `DESTROY_CONFIRM_TIMEOUT` hit

**Reconstruction:**

```
deployment_history: action=deleted, status=in_progress, target=P
deployment_history: action=deleted, status=success, target=P

logs:
  [DISPATCH_CLAIM] kind=destroy
  [DESTROY_CONFIRM_TIMEOUT] service=... — DeleteService API succeeded but GetService still returns service after confirm window; verify via console
```

**Completeness:** ✅ Log explicitly calls out the timeout. Operator knows AutoStack's state says deleted but GCP may differ. Verdict: check GCP console.

---

### Path F: Circuit breaker opened

**Reconstruction:**

```
deployment_history: (5 failed rows for same target, each with reason)

logs:
  [FAILURE] category=transient (x4)
  [FAILURE] category=permanent (or 5th transient)
  [CIRCUIT_OPEN] target=P (first skip after threshold)
  [RECONCILE_SKIP] reason=circuit_open on subsequent cycles
```

**Completeness:** ✅ Failure category, count, and circuit state are all visible.

---

### Path G: Sweep reclaimed while dispatcher was running (sweep wins)

**Reconstruction:**

```
deployment_history:
  - action=created, status=success, target=P (dispatcher's success observation)
  - action=error, status=failed, target=P, message=abandoned: heartbeat went stale (sweep's record)
OR:
  - action=error, status=failed, target=P, message=abandoned: process restart (dispatcher's panic, sweep's reclaim)

logs:
  [DISPATCH_CLAIM] target=P operation=opXYZ
  [DEPLOY_START]
  [DEPLOY_END] (success)
  [OP_COMPLETE_NOOP] op=opXYZ — already terminal (sweep won)
  [RELEASE_LOST_OWNERSHIP] target=P op=opXYZ; observed_outcome=success
  [STARTUP_SWEEP] or [RUNTIME_SWEEP]
  [OP_ABANDONED] op=opXYZ
```

**Completeness:** ✅ Two history rows tell the complete story: the dispatcher succeeded but the sweep took over. The `[RELEASE_LOST_OWNERSHIP]` log explicitly connects dispatch observation to sweep authority.

---

## Forensic Gaps

### FG-1: Panic source not always distinguishable from normal dispatch failure

A panic in `dispatchDeploy`'s deferred recovery looks like:
`[DISPATCH_PANIC] target=P op=opXYZ panic=<value>`
vs a hard error `[DEPLOY_END] target=P err=<non-nil>`.

Both write to history. The panic's stack trace is in logs (Go prints panics to stderr which lands in the same log stream). **This is recoverable.**

### FG-2: Stale deploy (rolloutMovedDuring) — history doesn't record which revision was deployed

The history row shows `action=created/updated, status=failed, message=stale spec`. It does NOT record the revision that was actually deployed. An operator would need to cross-reference `operations.rollout_revision` (the revision at claim time) with `rollouts.updated` (the revision that replaced it). This is doable but not obvious.

**Severity:** Low — the operator's question is "did the right version deploy?" and the honest answer via this reconstruction path is: "the version at the time of the call was deployed, but the rollout was updated. The next cycle will deploy the new version." Not a gap in the data — just non-obvious.

---

## Verdict

**Forensic reconstruction is complete for all significant failure modes.** Every scenario from normal success to sweep-reclaimed crash has enough information in logs + history to reconstruct what happened without guesswork.

**The primary gaps are around Phase 3 enhancement:**
- FG-2: Adding revision info to history rows (Phase 3)
- Structured logging (Phase 3)
- per-revision diff visibility (Phase 3)

**Phase 2 operators can answer:** what happened, when, what AutoStack believes, and what the sweep classified as abandoned.

---

## Related
- [[phase2.9/observability-integrity-assessment]] — log tag coverage
- [[phase2.3/incident-reconstruction-assessment]]