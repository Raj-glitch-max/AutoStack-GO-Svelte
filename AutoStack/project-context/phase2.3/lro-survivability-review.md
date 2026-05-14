# Long-Running Operation Survivability — Phase 2.3

## Last Updated
2026-05-14

## Premise

A Cloud Run deploy can take 5–10 minutes on cold-start, longer on
revisions with large container images or slow startup probes. The
control plane must distinguish:

- "live operation taking its time" — should not be reclaimed.
- "abandoned operation from a dead process" — must be reclaimed.

## Time budget today

| Bound | Value | What it gates |
|---|---|---|
| Reconciler tick | 30s | How often dispatch decisions are made |
| `DeployTimeout` | 15 min | Hard upper bound on a single `dispatchDeploy` call. Bounded `context.WithTimeout`. |
| `waitForServiceReady` internal | 1 h | Misleading — parent ctx (`DeployTimeout`) wins. |
| Status-poll ctx | 30s | Per-target GetStatus call |
| `heartbeatInterval` | 60s | Heartbeat tick |
| `abandonedOpThreshold` | 20 min (currently unused at sweep time) | Reserved for future runtime sweep |
| `failureThreshold` | 5 | Per-target failure count before circuit opens |
| Sweep frequency | startup-only | No runtime sweep |

## What survives long deploys today

| Scenario | Outcome |
|---|---|
| Deploy completes in 1 min | ✓ Trivial. |
| Deploy completes in 13 min | ✓ Under DeployTimeout. Heartbeat ticks 13 times. Op stays in_progress. Dispatcher releases. |
| Deploy completes in 16 min | ✗ DeployTimeout fires at 15 min. ctx-cancelled propagates to provider call → ctx.Err() in `waitForServiceReady` → Deploy returns (result with status="cancelled", nil error). Dispatcher's result.Status="error" branch fires → target → error. Cloud Run service may have been created or may be in-progress; AutoStack reports `error`. |
| Deploy completes in 22 min (somehow) | Same as 16 min — bounded by DeployTimeout. |
| Process restart at 5 min into a 7-min deploy (single-pod) | Sweep marks op abandoned → target → error. Cloud Run service in unknown state (may have been created by the dying process). Operator must clear error and re-deploy; convergent. |
| Process restart at 5 min into a 7-min deploy (rolling restart, new pod boots before old pod gone) | Phase 2.3 fix: sweep ignores op with heartbeat within last 2 min. Old pod can continue. Old pod eventually exits cleanly. ✓ |
| Process restart at 5 min into a 7-min deploy (multi-pod, peer pod boots) | Sweep sweeps peer's live op. **Wrong.** Deferred — see [[ownership-integrity-review]] O-1. |
| Deploy stalls at 14 min (provider hangs) | DeployTimeout fires at 15 min. Same as 16-min case. |
| Provider rate-limit causes deploy to time out at 10 min into a 15-min budget | result.Status="timeout" from waitForServiceReady → result.Status="error" branch → target → error → ClassifyError sees "timeout" → ShouldRetry returns true (FailureTimeout retries). Circuit eventually opens. |

## Survivability gaps

### S-1: 15-min DeployTimeout is the entire budget

**Today:** A single deploy attempt has 15 minutes. Cloud Run cold-start
of a large image (multi-GB container) can flirt with this limit.

**Severity:** Medium. Most deploys will be fine; edge cases will fail
with `result.Status="timeout"` and require operator retry.

**Phase 2.5 work:** Configurable per-rollout DeployTimeout in
`target_config`. Deferred — needs operator UX for setting it.

### S-2: Heartbeat-aware sweep buys 2 min of survivability, not more

**Phase 2.3 fix:** Sweep ignores ops with `updated_at` within 2 min.
This protects against the rolling-restart case but does NOT extend
DeployTimeout. A 16-min deploy still fails.

### S-3: No mid-deploy "still alive?" check from reconciler

**Today:** While dispatcher is mid-flight, the reconciler skips this
target. Reconciler has no way to detect that the dispatcher goroutine
is wedged (not panicked, just stuck in a syscall). DeployTimeout is the
only escape.

**Severity:** Low — a panic or syscall hang in Go is rare; DeployTimeout
catches the worst case.

**Phase 2.5 work:** Reconciler could check `operations.updated_at` for
ops it knows are in flight; if the heartbeat went stale (> 2 min), log
loudly. Not a fix; a signal. Deferred.

### S-4: `succeeded_stale` does not budget against the rollout's deploy retries

**Today:** A respec-flapping rollout perpetually re-dispatches. No
"max stale retries" budget. Each attempt consumes ~15 min budget worst
case.

**Severity:** Theoretical. No production evidence of the pathology.

**Phase 2.5 work:** `succeeded_stale` count on the target row; circuit
opens after N. Deferred.

### S-5: Long-running Destroy

**Today:** Destroy is dispatched the same way as Deploy with
`DeployTimeout = 15 min`. Cloud Run's `DeleteService` is async — the
API returns 200 in milliseconds. Provider-side cleanup may take longer
but is unobserved.

**Severity:** Very low. Destroy completes in seconds API-wise.

### S-6: Process SIGTERM during deploy gives no shutdown grace

**Today:** SIGTERM cancels the process context (Go's signal handling
is implicit in PocketBase). The dispatcher's `Provider.Deploy` call is
mid-flight; the ctx.Done() fires; the provider returns
context-cancelled error; dispatcher writes failed history; target →
error.

**Severity:** Low. Operator-visible result is "deploy errored" which
is honest. The operator must re-deploy.

**Phase 2.5 work:** Graceful-shutdown that lets in-flight ops complete
up to N seconds before SIGTERM forces. Requires PocketBase-level
shutdown hook integration. Deferred.

## Phase 2.3 implementation in this area

- S-2: heartbeat-aware sweep (already noted in [[ownership-integrity-review]] O-2).

## Deferred

- All others above are Phase 2.5+ items with clear designs.

## Related
- [[ownership-integrity-review]]
- [[replay-safety-assessment]]
- [[../reconciler/sweep-and-heartbeat-semantics]]
