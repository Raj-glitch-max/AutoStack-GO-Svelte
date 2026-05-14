# Operational Platform Maturity Roadmap (Phase 3.5)

**Last Updated:** 2026-05-14
**Phase:** 3.5 (Operational Platform Maturity)

## Purpose

Roadmap for evolving AutoStack from "deployment control plane" into
"operationally usable platform." UX, dashboards, lineage visualization,
incident debugging — built on the truthful semantics established in
Phase 2 and the multi-provider primitives from Phase 3.0-3.4.

## What Phase 3.5 Delivers

| Area | Phase 2 baseline | Phase 3.5 target |
|---|---|---|
| Dashboard | PocketBase admin + frontend basics | Operator-first deployment dashboard |
| Rollout visibility | Status text | Visual timeline + lifecycle + workflow phases |
| Reconciliation visibility | Logs | Cycle inspection UI with grep by cycle_id |
| Provider visibility | None | Capability matrix per cloud account |
| Lineage visualization | Database rows | Searchable, filterable history view |
| Drift visibility | drift_summary field | UI badge + diff renderer |
| Ambiguity visibility | DB columns | Honest UI surfacing per [[partial-success-semantics]] |
| Incident debugging | Manual log scraping | Reconstruction view per target |

## The Phase 3.5 Sub-Components

### Component PC-1: Deployment Dashboard

The primary operator surface. Replaces ad-hoc PocketBase admin usage.

Layout:
```
┌─────────────────────────────────────────────────────────┐
│ AutoStack — Deployments                                 │
├─────────────────────────────────────────────────────────┤
│ Rollouts:  ▼ all  / pending(2) / error(1) / running(8)  │
├─────────────────────────────────────────────────────────┤
│ Name         Provider     Status         Workflow       │
│ ───────────────────────────────────────────────────────  │
│ payments-api gcp-cloudrun running        canary 50%     │
│ orders-svc   aws-ecs      running                       │
│ frontend     gcp-cloudrun error (auth)                  │
│ batch-jobs   azure-aca    running (ambiguous)           │
└─────────────────────────────────────────────────────────┘
```

Filters: provider, status, workflow phase, region, account.

Each row clicks into the target detail (PC-2).

### Component PC-2: Target Detail View

Per-target operational view. Five sections:

**Section 1: Status header**
- Canonical lifecycle state + badge.
- Workflow phase + step number (if active).
- Drift indicator (if any).
- Ambiguity indicator (if any).
- Last-synced timestamp.

**Section 2: Capability matrix**
- Renders this provider's capability profile.
- Rows: Capability, Supported, Semantic, Constraints, Notes.
- Highlights which capabilities affect this target's strategy.

**Section 3: Lineage timeline**
- Visual timeline of `deployment_history` rows.
- Hover for message detail.
- Filter by event type.
- Cycle correlation: click a cycle ID, see all events with that cycle.

**Section 4: Live operation**
- If `current_operation != ""`, show the operation row.
- Heartbeat status, started_at, last_updated.
- Cancel button (capability-gated).

**Section 5: Action buttons**
- Deploy (with strategy selector).
- Destroy (with confirm dialog).
- Rollback (with revision picker, capability-gated).
- Pause/Resume workflow (if active).

### Component PC-3: Reconciliation Inspector

For debugging "why didn't this target reconcile?":

```
┌─────────────────────────────────────────────────────────┐
│ Reconciler State                                        │
├─────────────────────────────────────────────────────────┤
│ Last tick:     2026-05-14T10:23:45 (12s ago)            │
│ Next tick:     2026-05-14T10:24:15                      │
│ Worker pool:   4 workers (2 idle, 2 active)             │
│ Queue depth:   12                                       │
│ Circuit open:  1 target (frontend / auth failure)       │
├─────────────────────────────────────────────────────────┤
│ Recent cycles                                           │
│ 2026-05-14T10:23 cycle=ab12cd34 — 8 targets, 7.2s       │
│ 2026-05-14T10:22 cycle=cd34ef56 — 8 targets, 6.8s       │
└─────────────────────────────────────────────────────────┘
```

Click a cycle for the cycle detail view (PC-4).

### Component PC-4: Cycle Detail View

Per-cycle reconstruction:

- Targets processed.
- Per-target outcome (claimed / skipped / error / dispatched).
- Logs filtered by `cycle=<id>`.
- Lineage rows tagged with this cycle.

This is the operator-facing version of [[../phase2.9/forensic-reconstruction-assessment]]'s
7 reconstruction paths.

### Component PC-5: Incident Debugging Workflow

A guided flow for "this target is stuck. why?":

1. Show target's current status + ambiguity + drift + workflow phase.
2. Show last 10 cycles' decisions for this target.
3. Show last 5 lineage rows.
4. Show current operation (if any) + heartbeat freshness.
5. Show circuit state.
6. Suggest action based on state combination.

Suggestions table:

| State combination | Suggested action |
|---|---|
| `error` + permanent failure | Operator respec, then deploy |
| `creating` + op terminal | Clear `current_operation` manually (with warning) |
| `deleting` + confirm timeout | Verify provider console; mark deleted manually |
| `running` + drift detected | Review drift diff; respec or accept |
| Stuck `running` + workflow halted | Cancel workflow, then proceed |

These suggestions are decision aids, not auto-actions. Operator
confirms each step.

### Component PC-6: Provider Capability Browser

A standalone view of supported capabilities per provider.

```
┌─────────────────────────────────────────────────────────┐
│ Provider Capabilities                                   │
├─────────────────────────────────────────────────────────┤
│        Cloud Run | ECS        | ACA                     │
│ ────────────────────────────────────────────────────────│
│ Deploy:    ✅       ✅           ✅                     │
│ Rollback:  ✅ grad   ✅ cutover ✅ grad                │
│ Canary:    ⚠️ Phase 3.3 work pending  ✅                │
│ Drift:     ❌       ❌           ❌                     │
│ ...                                                     │
└─────────────────────────────────────────────────────────┘
```

Cells show capability + semantic + notes (tooltip).

### Component PC-7: Operational Search

Search across targets, lineage, and operations:

- "find all targets in error" — filtered list.
- "find all deploys that succeeded but had stale-spec rollback" — filtered lineage.
- "find all canary workflows in awaiting_approval > 1h" — filtered targets.

Implemented as PocketBase queries with predefined templates.

## Frontend Architecture

Phase 3.5 evolves the SvelteKit frontend:

- New routes under `/dashboard/`, `/targets/`, `/reconciler/`,
  `/providers/`.
- Real-time updates via WebSocket subscriptions (existing Phase 2
  infrastructure).
- Capability matrix data fetched from
  `GET /api/v1/providers/:name/capabilities` (Phase 3.1).
- Lineage queries via existing PocketBase REST.

No new frontend frameworks. SvelteKit + Tailwind continues.

## The UX Principles

### UX-1 — Truth Over Comfort

The UI never renders comfortable lies. If the target's state has
ambiguity, drift, or stuck workflow, the UI shows it — even if the
operator would prefer green. P-5 of [[ambiguity-semantics-model]].

### UX-2 — Capability-Aware Controls

Buttons that invoke unsupported capabilities are **disabled** with
explanatory tooltip. Operator never clicks "Rollback" and gets
`ErrNotImplemented` — they see "rollback not supported on this
provider" before clicking.

### UX-3 — Provider Differences Surfaced

Provider-specific behavior is annotated, not hidden. "Rollback (instant
cutover)" vs "Rollback (gradual)" appear in tooltips. R-1 mitigation.

### UX-4 — One-Click Reconstruction

From any target, an operator can reconstruct the full lifecycle in ≤ 3
clicks. The lineage timeline + cycle filtering achieves this.

### UX-5 — Operator Decisions Persist

Workflow approvals, rollback choices, drift-acceptance — all written to
PocketBase with operator attribution. Audit trail is the same as
lineage; nothing lives in UI session state.

## What Phase 3.5 Refuses

- **Refuse:** Auto-fix buttons. The operator decides, the platform
  executes.
- **Refuse:** Notifications without operator opt-in (Phase 3.7 domain).
- **Refuse:** Inline log streaming for unsupported providers — show
  "log streaming not available" rather than empty pane.
- **Refuse:** Cross-target bulk operations without explicit confirm.

## Phase 3.5 Closure Criteria

1. PC-1..PC-7 are implemented in SvelteKit.
2. WebSocket subscriptions for live updates extend to cloud targets
   (currently K8s only).
3. UX principles UX-1..UX-5 are reviewed against actual UI screens.
4. Operator usability test: tester can debug 3 prepared scenarios using
   only the UI.
5. Capability matrix view renders correctly for all 3 providers.

## Related

- [[operational-taxonomy]] — what operators see vs. internal terminology
- [[deployment-lineage-model]] — lineage visualization model
- [[incident-reconstruction-guide]] — operator-facing playbook
- [[provider-capability-matrix]] — data source for PC-6
- [[ambiguity-semantics-model]] — UX-1 mandate
- [[partial-success-semantics]] — UI display contract
