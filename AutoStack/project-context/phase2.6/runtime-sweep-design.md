# Runtime Sweep Design — Phase 2.6

## Last Updated
2026-05-14

## Problem

Phase 2.3 startup sweep handles process-restart abandonment. Phase 2.6
adds a **runtime** sweep that catches:

- Dispatcher goroutines that crashed without panic (impossible in pure
  Go, but possible if the host OS killed a single thread).
- Dispatchers stuck in syscalls past DeployTimeout (cannot happen — ctx
  cancellation propagates, but defense in depth).
- The OS-2 / OS-7 stuck-window: process died after first heartbeat
  fired but before the next, so startup sweep preserved the op as
  "live" and target stayed stuck.

## Design

A separate goroutine, `runRuntimeSweep`, ticks every 5 minutes (same
order of magnitude as the heartbeat liveness window). Each tick:

1. Query in-progress ops with `updated_at` older than the liveness
   window plus a generous margin (e.g., 5 minutes).
2. For each found op: mark abandoned, release target to error, write
   history. Same actions as the startup sweep.

### Key difference from startup sweep

- **Startup sweep:** any op whose heartbeat falls outside the liveness
  window (or which never heartbeated). Aggressive.
- **Runtime sweep:** any op whose heartbeat fell outside the liveness
  window AND there is no concurrent dispatcher in this process that
  owns it.

Single-pod: the concurrent-dispatcher check is trivial. Any in-progress
op in DB whose heartbeat is stale must belong to a dead goroutine
(since this is the only process). Reclaim is safe.

Multi-pod: this check is hard — we cannot tell if a peer pod's
dispatcher is alive. **Multi-pod runtime sweep is unsafe without
pod-identity stamping. Phase 2.7 work.**

### SQL

```sql
SELECT id, target, kind, started_at, updated_at
FROM operations
WHERE status = 'in_progress'
  AND updated_at < :cutoff
```

cutoff = now - (2 × heartbeatInterval + sweepMargin), where
sweepMargin = 3 min. So cutoff = now - 5 min.

### Cadence

- Tick every 5 min.
- Margin: cutoff is now - 5min, so an op needs to have heartbeated 5+
  minutes ago to be eligible.
- A dispatcher whose heartbeat goroutine is healthy refreshes
  updated_at every 60s, so never qualifies.

### Boot integration

The runtime sweep is started after the startup sweep, inside
`Reconciler.Start()`. It uses the same mutex and stop channel as the
reconciler.

## Implementation

Add `runRuntimeSweep` to the reconciler. Run it as a goroutine launched
by `Reconciler.Start()` after the startup sweep completes. Stop it on
`Reconciler.Stop()` via the same stop channel.

## Hazards considered

### H-1: Runtime sweep races with a dispatcher about to heartbeat

**Setup:** Dispatcher's heartbeat scheduled for T+0.99s. Runtime
sweep at T+0.5s observes updated_at = T-60s (old). Cutoff is T-300s.
T-60s > T-300s → op is NOT old enough. Sweep skips. ✓

### H-2: Runtime sweep races with dispatcher's release

**Setup:** Dispatcher about to call `completeOperation` and
`releaseTarget`. Runtime sweep observes the op as in_progress with
stale heartbeat (heartbeat goroutine already exited because deployCtx
cancelled).

**Timeline:**
- Dispatcher: provider returns success at T.
- Heartbeat goroutine: `defer cancel()` fires, heartbeat exits.
- Dispatcher: 10ms later, calls `completeOperation` with CAS on
  in_progress.
- Runtime sweep: 5min between ticks. May see the op briefly stale
  before dispatcher's completion lands.

If sweep wins: op marked `failed`, target → error. Dispatcher's
`completeOperation` CAS fails (status not in_progress). Dispatcher's
`releaseTarget` CAS fails (current_operation cleared by sweep). All
silent.

**Outcome:** Dispatcher's actual provider-side success is lost from
truth. Target → error.

**Severity:** Low. The window is tight (between heartbeat-exit and
dispatcher-complete, both happen in <100ms typically). Plus, runtime
sweep's cutoff is +5 min so the op would need to have been NOT
heartbeated for 5+ min before this matters.

**Realistic timing:** Heartbeat ticks every 60s. Last heartbeat was at
most 59s before the dispatcher returned. So `updated_at` is at most 59s
old. Runtime sweep needs 5+ min staleness. **The race is
impossible** for normal deploys.

For ABNORMALLY long deploys (e.g., 16 min where DeployTimeout fired and
the dispatcher is wrapping up): heartbeat last fired at min 15
(internally). Dispatcher writes failed at min 15+. Runtime sweep's
next tick at min 20 sees `updated_at` = min 15 (5min stale, marginal).
Could fire. **Same outcome:** dispatcher's failed completion is the
same outcome the sweep would mark anyway.

**Acceptable.** ✓

### H-3: Sweep finds op whose dispatcher panicked just-now

Dispatcher's panic-recovery `completeOperation` marks failed. Then
`releaseTarget` runs. Heartbeat exits. Then function returns.

Runtime sweep finds nothing in-progress (already failed). ✓

### H-4: Multi-pod misclassification

Documented. Single-pod only. Phase 2.7 pod-identity stamping required.

## Hazards NOT mitigated

- Multi-pod (out of scope for 2.6).
- Operator manual edits to operations (out of scope; "trusted role").

## Related
- [[../phase2.3/sweep-and-heartbeat-semantics]] - corrected reference
- [[../reconciler/sweep-and-heartbeat-semantics]]
- [[../phase2.4/ownership-integrity-review]] OS-2, OS-7
