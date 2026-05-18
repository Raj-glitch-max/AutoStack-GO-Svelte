<script lang="ts">
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Alert } from "flowbite-svelte";

  // Drill mode is strictly simulation — no production mutations.
  // Every API call in this surface goes to /api/v1/platform/drills/*
  // which is a read-only simulation endpoint. Nothing is written to
  // production state.

  const DRILLS = [
    {
      id: "contradiction",
      label: "Contradiction Detection",
      description: "Simulate a provider observation that contradicts the desired state. Verifies the contradiction detection pipeline surfaces the event correctly.",
      risk: "zero",
    },
    {
      id: "replay-mismatch",
      label: "Replay Mismatch",
      description: "Simulate a replay manifest hash failure. Verifies the operator is notified and no automatic recovery is attempted.",
      risk: "zero",
    },
    {
      id: "corrupted-backup",
      label: "Corrupted Backup",
      description: "Simulate a backup with an invalid payload hash. Verifies the backup verification surface rejects it and surfaces the failure.",
      risk: "zero",
    },
    {
      id: "governance-deadlock",
      label: "Governance Deadlock",
      description: "Simulate an execution requiring multi-approval where one approver is unavailable. Verifies escalation path and that execution remains blocked (not auto-approved).",
      risk: "zero",
    },
    {
      id: "worker-quarantine",
      label: "Worker Quarantine",
      description: "Simulate a worker reporting unhealthy status. Verifies the worker is quarantined and not assigned new tasks until operator clears it.",
      risk: "zero",
    },
    {
      id: "archive-tamper",
      label: "Archive Tamper Detection",
      description: "Simulate a modified archive entry. Verifies the audit chain detects the tamper and surfaces a chain integrity failure.",
      risk: "zero",
    },
    {
      id: "tenant-isolation",
      label: "Tenant Isolation Probe",
      description: "Simulate a cross-tenant data access attempt. Verifies that tenant boundaries are enforced and no cross-tenant data is returned.",
      risk: "zero",
    },
    {
      id: "jwt-flood",
      label: "Expired JWT Flood",
      description: "Simulate a burst of requests with revoked or expired JWTs. Verifies rate limiting and revocation enforcement without impacting legitimate sessions.",
      risk: "zero",
    },
  ] as const;

  type DrillID = typeof DRILLS[number]["id"];

  let results: Record<string, { loading: boolean; data: any; error: string }> = {};

  async function runDrill(drillID: DrillID) {
    results = { ...results, [drillID]: { loading: true, data: null, error: "" } };
    try {
      const data = await client.send(`/api/v1/platform/drills/${drillID}`, { method: "POST" });
      results = { ...results, [drillID]: { loading: false, data, error: "" } };
    } catch (e: any) {
      results = { ...results, [drillID]: { loading: false, data: null, error: e?.message ?? "Drill failed" } };
    }
  }

  function drillState(id: string) {
    return results[id] ?? { loading: false, data: null, error: "" };
  }
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div>
    <div class="flex items-center gap-3">
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Operational Drill Mode</h1>
      <Badge color="orange" class="text-sm font-semibold tracking-wide">SIMULATION ONLY</Badge>
    </div>
    <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
      Each drill simulates a failure scenario and verifies the platform responds correctly.
      <strong class="text-gray-700 dark:text-gray-300">No production state is mutated.</strong>
      All drill API calls are read-only simulations — they never write to the operational store,
      never modify running executions, and never send external requests.
    </p>
    <div class="mt-3 rounded-md border border-orange-200 bg-orange-50 dark:bg-orange-900/20 p-3">
      <p class="text-xs font-semibold text-orange-700 dark:text-orange-400">
        SIMULATION MODE ACTIVE — These drills exercise platform detection paths only.
        They do not inject real faults into running infrastructure.
        Real fault injection requires an external harness; see docs/operational-drills.md.
      </p>
    </div>
  </div>

  <div class="grid gap-4 md:grid-cols-2">
    {#each DRILLS as drill}
      {@const state = drillState(drill.id)}
      <Card>
        <div class="space-y-3">
          <div class="flex items-start justify-between gap-2">
            <div>
              <h3 class="text-sm font-semibold text-gray-800 dark:text-gray-200">{drill.label}</h3>
              <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">{drill.description}</p>
            </div>
            <Badge color="green" class="text-xs shrink-0">Zero Risk</Badge>
          </div>

          <button
            on:click={() => runDrill(drill.id)}
            disabled={state.loading}
            class="w-full rounded-md border border-primary-600 px-3 py-1.5 text-xs font-medium text-primary-600
              hover:bg-primary-50 dark:hover:bg-primary-900/20 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {state.loading ? "Running…" : "Run Drill (Simulation)"}
          </button>

          {#if state.error}
            <Alert color="red" class="text-xs">{state.error}</Alert>
          {:else if state.data}
            <div class="rounded-md bg-gray-50 dark:bg-gray-800 p-3 space-y-2">
              <div class="flex items-center gap-2">
                <Badge color={state.data.passed ? "green" : "red"} class="text-xs">
                  {state.data.passed ? "PASS" : "FAIL"}
                </Badge>
                <span class="text-xs text-gray-600 dark:text-gray-400">{state.data.summary}</span>
              </div>
              {#if state.data.expected_behavior}
                <p class="text-xs text-gray-500">
                  <span class="font-medium">Expected:</span> {state.data.expected_behavior}
                </p>
              {/if}
              {#if state.data.observed_behavior}
                <p class="text-xs text-gray-500">
                  <span class="font-medium">Observed:</span> {state.data.observed_behavior}
                </p>
              {/if}
              {#if state.data.violations?.length}
                <div class="rounded border border-red-200 bg-red-50 dark:bg-red-900/20 p-2">
                  {#each state.data.violations as v}
                    <p class="text-xs text-red-600 dark:text-red-400">{v}</p>
                  {/each}
                </div>
              {/if}
              {#if state.data.operator_action_required}
                <p class="text-xs font-medium text-orange-600 dark:text-orange-400">
                  Operator action required: {state.data.operator_action_required}
                </p>
              {/if}
            </div>
          {/if}
        </div>
      </Card>
    {/each}
  </div>

  <div class="rounded-md border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4 space-y-1">
    <p class="text-xs font-semibold text-gray-700 dark:text-gray-300">Unsupported in Drill Mode</p>
    <p class="text-xs text-gray-500">Real process kill (kill -9) — requires external test harness</p>
    <p class="text-xs text-gray-500">Cross-replica fault injection — single-node only</p>
    <p class="text-xs text-gray-500">Real network partition — simulated in-process only</p>
    <p class="text-xs text-gray-500">Production infrastructure mutation — strictly prohibited from this surface</p>
  </div>
</div>
