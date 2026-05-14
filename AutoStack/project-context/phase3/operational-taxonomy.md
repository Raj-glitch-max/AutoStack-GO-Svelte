# Operational Taxonomy (Phase 3.5)

**Last Updated:** 2026-05-14
**Phase:** 3.5 (Operational Platform Maturity)

## Purpose

Define the **vocabulary** that operators see in the UI, in CLI output,
in logs, and in support conversations. Consistent terminology is a
prerequisite for operator trust; sloppy terminology is a stealth
abstraction lie (R-1).

This doc is the **operator-facing dictionary**. Engineering may use
finer-grained internal terms; the surface terms below are the contract.

## Tier 1: Lifecycle Vocabulary

The seven canonical lifecycle states (Phase 2 baseline preserved):

| Term | Operator-facing definition |
|---|---|
| **Pending** | Operator created the target; AutoStack hasn't started deploying yet. |
| **Creating** | Deploy in progress; provider has not yet confirmed ready. |
| **Running** | Provider reports the deployment is operational. |
| **Updating** | A change is being applied to a previously running target. |
| **Deleting** | Destroy in progress; provider has not yet confirmed gone. |
| **Deleted** | Confirmed gone. |
| **Error** | Failed; operator action required. |

NOT IN THIS LIST (rejected): "Healthy", "Active", "Live", "Idle",
"Stopped", "Suspended", "Paused" (for lifecycle — "Paused" is a
workflow term).

## Tier 2: Workflow Vocabulary

For deployment workflows (Phase 3.3+):

| Term | Operator-facing definition |
|---|---|
| **Direct deploy** | The standard one-step deploy. |
| **Blue/Green** | Provision the new revision alongside the old; switch traffic atomically. |
| **Canary** | Send a small percent of traffic to the new revision; observe; promote. |
| **Staged** | Deploy to multiple targets in sequence, halting on failure. |
| **Strategy** | The deployment pattern (one of the above). |
| **Step** | One atomic action within a strategy. |
| **Phase** | The workflow's current position (pending, running, observing, awaiting approval, completed, halted, rolled back). |
| **Approval** | Operator confirms the next step should proceed. |
| **Cancel** | Operator-initiated reversal of an in-flight workflow. |

NOT IN THIS LIST: "Pipeline" (avoid; suggests CI/CD), "Job" (avoid;
suggests batch), "Flow" (avoid; vague).

## Tier 3: Ambiguity Vocabulary

For surfaces where the system is uncertain:

| Term | Operator-facing definition |
|---|---|
| **Ambiguous state** | Current state has known uncertainty; details available. |
| **Pending confirmation** | An action was issued; provider hasn't fully acknowledged yet (S-4 ambiguity). |
| **Stale provider** | The provider hasn't reported in longer than expected (S-5 ambiguity). |
| **Provider-native state: X** | The provider's specific term (e.g., "Provisioning") preserved alongside the canonical state. |
| **Drift detected** | The deployed resource doesn't match the recorded spec. |
| **Drift cause: external mutation** | The resource was modified by something other than AutoStack. |

NOT IN THIS LIST: "Indeterminate", "Glitched", "Weird", "Frozen" (all vague).

## Tier 4: Reconciliation Vocabulary

For operators investigating "why":

| Term | Operator-facing definition |
|---|---|
| **Cycle** | One pass of the reconciliation loop (every 30s). |
| **Cycle ID** | The unique identifier for a cycle, used to correlate logs and lineage. |
| **Claim** | The reconciler taking exclusive ownership of a target's next action. |
| **Sweep** | The recovery scan that handles operations abandoned by a crashed dispatcher. |
| **Operation** | One in-flight provider action (deploy, destroy, etc.). |
| **Heartbeat** | The signal that an in-flight operation is alive. |
| **Suspicion** | The system has seen one error from `updating`; needs a second to confirm. |
| **Circuit open** | Repeated failures have temporarily stopped retries for this target. |
| **Backoff** | A waiting period before the next retry. |

NOT IN THIS LIST: "Worker thread", "Goroutine", "Mutex" (internal
mechanisms have engineering names, not operator names).

## Tier 5: Provider Vocabulary

| Term | Operator-facing definition |
|---|---|
| **Provider** | The cloud platform (Cloud Run, ECS, ACA). |
| **Cloud account** | The credentials + region pairing for a provider. |
| **Capability** | A feature a provider supports. |
| **Supported** | The capability is implemented for this provider. |
| **Capability semantic** | How the capability works concretely on this provider. |
| **Region** | The cloud region the account/target lives in. |
| **External ID** | The provider's identifier for the deployed resource. |
| **Revision** | A specific version of a deployed resource (provider-managed). |
| **Endpoint URL** | The public URL clients use to reach the deployment. |

NOT IN THIS LIST: "Cloud", "Vendor", "Platform" (used elsewhere for
AutoStack itself; reserve "provider" for the cloud).

## Tier 6: Lineage Vocabulary

| Term | Operator-facing definition |
|---|---|
| **Lineage** | The complete recorded history of a target's lifecycle. |
| **History row** | One entry in the lineage. |
| **Event type** | The kind of history row. |
| **Operator action** | A history row attributed to a specific operator. |
| **System action** | A history row produced by reconciliation. |

NOT IN THIS LIST: "Audit trail" (reserved for security audit, Phase
3.8), "Log" (reserve for runtime logs).

## Tier 7: Error Categorization Vocabulary

| Term | Operator-facing definition |
|---|---|
| **Auth failure** | The provider rejected our credentials. Operator must check IAM. |
| **Quota failure** | The provider's quota for this account is exceeded. |
| **Permanent failure** | The provider says no; retrying won't fix. Operator must investigate. |
| **Transient failure** | A temporary error; will retry automatically. |
| **Timeout** | The provider didn't respond in time. May retry. |

NOT IN THIS LIST: "Internal error", "Unknown error" (always classify;
"Unknown" hides truth). If genuinely unknown, surface "Unclassified"
with the raw message.

## Tier 8: Ambiguity Surfacing Vocabulary

| Bit | UI Text | Tooltip detail |
|---|---|---|
| S-1 (eventual consistency) | "Settling..." | "Provider may report stale state for up to ~Xs" |
| S-2 (provider-native gap) | "Ambiguous state" | "Provider-native: X; canonical: Y" |
| S-3 (capability gap) | "Not supported on this provider" | "<capability> capability is not implemented for <provider>" |
| S-4 (confirm timeout) | "Pending confirmation" | "Action issued; awaiting provider confirmation (deadline: <time>)" |
| S-5 (provider silence) | "Stale (last sync <time>)" | "Provider hasn't reported in over X minutes" |

## Tier 9: Action Vocabulary

| Action | Operator-facing label | Confirmation modal text |
|---|---|---|
| Initiate deploy | "Deploy" | "Deploy <image> to <target>?" |
| Initiate destroy | "Delete" | "This will permanently delete <target>. Continue?" |
| Cancel workflow | "Cancel workflow" | "Cancel the current <strategy>? This will roll back partial changes." |
| Approve workflow step | "Approve and continue" | (none — direct action) |
| Reject workflow step | "Reject and roll back" | "Roll back the canary from <X>% to 0%?" |
| Rollback to revision | "Roll back to <revision>" | "Roll back from <current> to <target>?" |
| Pause workflow | "Pause workflow" | (none) |
| Resume workflow | "Resume workflow" | (none) |
| Acknowledge drift | "Accept drift as new baseline" | "Update <target> spec to match provider state?" |
| Force-clear stuck op | "Clear stuck operation" | "Manually clear current_operation? Use only after verifying provider state." |

## The Glossary Discipline

When new operator-facing language is added:

1. Propose in PR description.
2. Update this doc.
3. Get reviewer ack that the term doesn't duplicate or conflict.
4. Apply consistently across UI, CLI, logs, docs.

When existing language is changed:

1. Document the change here AND in [[../decisions/]] as an ADR.
2. Update all surfaces atomically (UI, CLI, logs, docs in one PR).

## What This Doc Does NOT Cover

- Internal engineering terminology (CAS, sweep predicate, etc.).
- Per-provider native terms — UI surfaces canonical "Target" with a
  tooltip showing the provider-native term.
- Cost terminology — Phase 3.5+ work.

## Phase 3.5 Closure Criteria

For this doc:

1. Tier 1-9 vocabulary tables are sealed.
2. UI strings reviewed against this doc.
3. Log messages reviewed against this doc.
4. CLI / API responses reviewed against this doc.
5. Reviewer discipline established for adding/changing terms.

## Related

- [[operational-platform-maturity-roadmap]] — where this taxonomy applies
- [[deployment-lineage-model]] — lineage event types
- [[provider-capability-matrix]] — capability terms
- [[ambiguity-semantics-model]] — ambiguity surfacing
