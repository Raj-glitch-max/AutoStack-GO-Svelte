# Deployment Lineage Model (Phase 3.5)

**Last Updated:** 2026-05-14
**Phase:** 3.5 (Operational Platform Maturity)

## Purpose

Define the **lineage model** — how AutoStack records, indexes, and
surfaces the complete history of every target's lifecycle. This is the
operator-facing extension of Phase 2's G-12 (lineage completeness)
guarantee.

Lineage is the forensic spine. If it's incomplete, gaps in operator
debugging surface as platform-trust failures.

## What Lineage Is

A lineage is the **append-only sequence of events** capturing every
significant transition in a target's life:

- Operator intent recorded (deploy created, endDate set, respec).
- Reconciler decisions (claim, dispatch, sweep).
- Provider responses (deploy started, ready, error, destroy initiated,
  deleted).
- Ambiguity transitions (entered, cleared).
- Workflow transitions (strategy started, step started, step ended,
  approval, rejection, cancellation).
- Drift detection (detected, resolved).

The lineage of a single target may be thousands of rows over its
lifetime. Phase 3.5 makes this navigable.

## The Storage Model

```sql
CREATE TABLE deployment_history (
    id TEXT PRIMARY KEY,
    target TEXT NOT NULL,
    rollout TEXT,
    operation TEXT,           -- operations.id, if applicable
    provider TEXT,
    status TEXT NOT NULL,     -- event type (see taxonomy below)
    message TEXT,
    cycle_id TEXT,            -- for cross-component correlation (G-13)
    actor TEXT,               -- "system", "operator:<user-id>", "sweep", etc.
    revision TEXT,            -- provider revision name (Phase 3.3 addition)
    workflow_step INT,        -- workflow-aware events (Phase 3.3 addition)
    created DATETIME NOT NULL,
    INDEX idx_target_created (target, created),
    INDEX idx_cycle (cycle_id),
    INDEX idx_operation (operation)
);
```

Phase 3 adds three columns to Phase 2's existing schema: `revision`,
`workflow_step`, `actor`. Migration is Phase 3.5 scope.

## The Event Taxonomy

The `status` column is an enumerated event type.

### Lifecycle events (Phase 2 baseline, preserved)

| status | Meaning |
|---|---|
| `created` | Target row created (operator intent) |
| `in_progress` | Deploy/Destroy dispatched |
| `succeeded` | Deploy/Destroy succeeded |
| `failed` | Deploy/Destroy failed |
| `succeeded_stale` | Deploy succeeded but spec moved (Phase 2.6) |
| `sweep_abandoned` | Sweep reclaimed an abandoned op |
| `ownership_lost` | Dispatcher returned after sweep already took over |
| `transition_refused` | Status change refused by guard (Phase 2 F-5) |

### Phase 3.3 workflow events

| status | Meaning |
|---|---|
| `workflow_started` | Strategy initiated |
| `workflow_step_started` | Step dispatched |
| `workflow_step_succeeded` | Step completed successfully |
| `workflow_step_failed` | Step failed (HaltOnFailure may apply) |
| `workflow_phase_transition` | Workflow phase changed |
| `workflow_approved` | Operator approved continuation |
| `workflow_rejected` | Operator rejected; rollback triggered |
| `workflow_paused` | Operator paused |
| `workflow_resumed` | Operator resumed |
| `workflow_cancelled` | Cancellation initiated |
| `workflow_completed` | Strategy finished successfully |
| `workflow_halted` | Strategy halted on failure |

### Phase 3.3 rollback events

| status | Meaning |
|---|---|
| `rollback_started` | Operator initiated rollback |
| `rollback_succeeded` | Rollback completed; new revision serving |
| `rollback_failed` | Rollback errored |

### Phase 3.2 drift events

| status | Meaning |
|---|---|
| `drift_detected` | Spec-vs-actual diff found |
| `drift_accepted` | Operator accepted drift as new baseline |
| `drift_resolved` | Drift resolved by respec or external sync |

### Phase 3.0 ambiguity events

| status | Meaning |
|---|---|
| `ambiguity_entered` | Target acquired an ambiguity flag |
| `ambiguity_cleared` | Ambiguity flag cleared |
| `ambiguity_timeout` | Bounded ambiguity expired without resolution |

### Phase 3.5 administrative events

| status | Meaning |
|---|---|
| `intent_destroy` | Operator set endDate (Phase 2.3 M-2 baseline) |
| `intent_respec` | Operator changed manifest |
| `manual_clear` | Operator manually cleared a stuck field |

The full event set is closed: adding an event requires updating this
doc AND every UI surface that filters lineage.

## The Actor Field

```
actor IN (
  'system',
  'sweep',
  'reconciler',
  'workflow',
  'drift_cycle',
  'operator:<user_id>'
)
```

Distinguishes who caused an event. Operators investigating "did
someone change this?" filter on `actor LIKE 'operator:%'`.

## The Lineage Query Patterns

Phase 3.5 ships these canonical queries:

### Q-1: Full target lineage

```sql
SELECT * FROM deployment_history
WHERE target = ?
ORDER BY created DESC;
```

### Q-2: Cycle correlation

```sql
SELECT * FROM deployment_history
WHERE cycle_id = ?
ORDER BY created;
```

All events from one reconciler cycle across all affected targets.
Useful for "what happened in this cycle?"

### Q-3: Operator action audit

```sql
SELECT * FROM deployment_history
WHERE actor LIKE 'operator:%'
  AND created > ?
ORDER BY created DESC;
```

Who did what, when.

### Q-4: Workflow timeline

```sql
SELECT * FROM deployment_history
WHERE target = ?
  AND status LIKE 'workflow_%'
ORDER BY created;
```

### Q-5: Rollback chain

```sql
SELECT * FROM deployment_history
WHERE target = ?
  AND (status LIKE 'rollback_%' OR status = 'workflow_cancelled')
ORDER BY created;
```

### Q-6: Ambiguity transitions

```sql
SELECT * FROM deployment_history
WHERE target = ?
  AND status LIKE 'ambiguity_%'
ORDER BY created;
```

These queries are exposed via PocketBase REST + cached frontend
templates.

## The Lineage Surfacing

| UI Component | Query | Display |
|---|---|---|
| Target detail timeline | Q-1 | Visual chronological timeline |
| Cycle inspector | Q-2 | Cycle-grouped event list |
| Audit log view | Q-3 | Operator-action filtered list |
| Workflow timeline | Q-4 | Strategy-step swimlane |
| Rollback inspector | Q-5 | Rollback chain visualization |
| Ambiguity history | Q-6 | Bit-on / bit-off transitions |

## The Lineage Retention

Phase 3.5 does NOT auto-delete lineage rows. History is immutable and
unbounded. Deferred to Phase 3.7+:

- Lineage retention TTL policies.
- Lineage archival (move old rows to cold storage).
- Lineage compaction (merge adjacent identical-status rows).

Until then, operators with growing DB monitor row counts manually. The
Phase 2 envelope (≤ 20 targets) keeps this tractable. Phase 3.4
envelope (≤ 100 targets) is still tractable for typical retention.

## The Honesty Rules

### LR-1 — No Lineage Mutation

Existing rows are NEVER updated. Corrections are new rows with explicit
status (e.g., `correction_applied`). G-12 immutability preserved.

### LR-2 — No Silent Drop

Every event type defined above MUST produce a history row. A lifecycle
moment without a row is a bug. Phase 3.5 ships
`TestLineageCompleteness` to enforce.

### LR-3 — Cycle Correlation Always Present

Every reconciler-emitted row sets `cycle_id`. Operator-emitted rows
may have empty `cycle_id` (operator action is not in a reconciler
cycle). UI distinguishes.

### LR-4 — Sanitized Messages Only

`message` column passes through `sanitizeError` (Phase 2 F-8). No
credential leakage in lineage. Same hygiene as logs.

### LR-5 — Provider-Specific Detail Goes In Message

The provider's specific error message goes in `message`, not in
`status`. The status taxonomy stays small and stable; provider detail
varies.

## Phase 3.5 Closure Criteria

For this doc + corresponding code:

1. Migration adds `revision`, `workflow_step`, `actor` columns to
   `deployment_history`.
2. The event taxonomy is implemented in code via constants in
   `pkg/reconciler/lineage.go`.
3. Q-1..Q-6 queries are exposed via REST endpoints (PocketBase view
   collections or custom routes).
4. UI components consume the lineage per
   [[operational-platform-maturity-roadmap]].
5. `TestLineageCompleteness` enforces LR-2 — for each event type, the
   producing code path is tested.

## Related

- [[../phase2.9/operational-guarantee-matrix]] — G-12 baseline
- [[../phase2.9/forensic-reconstruction-assessment]] — 7 reconstruction paths
- [[operational-taxonomy]] — terms for event types
- [[operational-platform-maturity-roadmap]] — UI consumers
- [[workflow-lifecycle-contracts]] — Phase 3.3 events
- [[ambiguity-semantics-model]] — Phase 3.0 events
