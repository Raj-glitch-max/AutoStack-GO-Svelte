# Execution Lifecycle

This document describes the lifecycle of a single execution through the AutoStack runtime.

---

## States and Transitions

### ExecutionTask FSM

```
pending → queued → dispatched → running ──→ completed
                                         ├──→ paused → running
                                         ├──→ retry_wait → queued
                                         ├──→ verification_wait → completed
                                         │                      └──→ failed
                                         ├──→ contradicted → quarantined
                                         │               └──→ rolled_back
                                         ├──→ failed → retry_wait
                                         │          └──→ rolled_back
                                         └──→ quarantined
```

Terminal states: `completed`, `failed`, `contradicted`, `quarantined`, `rolled_back`.

Any transition not in the allowed map is rejected. There is no bypass.

### WorkflowExecution FSM

```
pending → running → completed
                 ├──→ failed
                 ├──→ paused → running
                 ├──→ blocked (approval wait) → running
                 └──→ rolling_back → rolled_back
```

---

## Execution Sequence

1. **Workflow graph is validated** — `VerifyWorkflowGraph()` checks entry node, edge reachability, hash integrity.

2. **`ExecuteWorkflowGraph()` is called** — returns `WorkflowExecution` in `running` state with entry node active, plus an initial `StepTransition`.

3. **Each node outcome is applied** — `AdvanceWorkflow()` takes a `NodeOutcome` (which edge to follow), transitions the execution, records a `StepTransition`.

4. **Tasks are dispatched** — `DispatchTask()` assigns a worker deterministically via `SHA-256(taskID + sorted(workerIDs))`.

5. **Task is executed** — the worker transitions the task through `queued → dispatched → running`.

6. **Provider operation runs** — `ProviderOperation` FSM tracks the cloud provider interaction. `DetectProviderContradiction()` scans for deploy-success-resource-missing, delete-success-resource-alive, rollback-success-unhealthy, scale-acknowledged-capacity-unchanged.

7. **Contradiction detected** — if a contradiction is found, the task transitions to `contradicted`. No auto-resolution. Operator decides.

8. **Completion or failure** — task transitions to `completed` or `failed`. `MarkTaskCompleted()` / `MarkTaskFailed()` stamp `CompletedAt`.

9. **Workflow advances** — `AdvanceWorkflow()` follows the appropriate edge (success/failure/timeout/contradiction/rollback).

10. **Checkpoint is written** — `BuildExecutionCheckpoint()` records the task set at a known logical clock. Hash is verified at write time.

11. **Replay certification** — `CertifyReplay()` produces a tamper-evident report. All 4 checks must pass: timeline integrity, checkpoint validity, divergence-free, confidence consistent.

12. **Platform certification** — `RunPlatformReadinessAudit()` evaluates all 5 gates. `PlatformCertified = true` only when all pass.

---

## Honest Limitations

- **Replay is read-only** — it does not re-execute any provider operation.
- **Divergence detection compares event hashes** — different ordering policies will produce divergence false-positives.
- **Abandoned task detection** requires a live worker roster — workers must emit heartbeats.
- **Checkpoint-assisted replay** requires that the checkpoint `StateHash` was computed over the same event set.
- **Retry policy** enforces backoff — `ComputeRetryBackoff()` clamps to `[BaseDelay, MaxDelay]`. Retries are not guaranteed to succeed.
