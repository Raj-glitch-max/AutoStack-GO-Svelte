<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";
  import { ShieldCheck, ShieldAlert, AlertTriangle, RefreshCw } from "lucide-svelte";

  let loading = true;
  let error = "";
  let status: any = null;

  async function fetchStatus() {
    loading = true;
    error = "";
    try {
      const res = await client.send("/api/v1/reconciler/truth-status", { method: "GET" });
      status = res;
    } catch (e: any) {
      error = e?.message ?? "Failed to load runtime truth status";
    } finally {
      loading = false;
    }
  }

  onMount(fetchStatus);

  function trustColor(state: string) {
    if (state === "hydrated_from_db") return "green";
    if (state === "partial_db_read_error") return "yellow";
    return "red";
  }

  function statusColor(s: string) {
    if (s === "green") return "green";
    if (s === "yellow") return "yellow";
    return "red";
  }
</script>

<div class="max-w-4xl mx-auto px-4 pt-16 pb-8 space-y-6">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Runtime Truthfulness Status</h1>
    <button
      on:click={fetchStatus}
      class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white"
    >
      <RefreshCw class="w-4 h-4" />
      Refresh
    </button>
  </div>

  <p class="text-sm text-gray-500 dark:text-gray-400">
    Runtime health reflects the truthfulness of the reconciler's internal state.
    This is separate from deployment success — infrastructure may be healthy
    while runtime metrics are partially hydrated.
  </p>

  {#if loading}
    <div class="flex justify-center py-12"><Spinner /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if status}
    <!-- Overall status banner -->
    <div class="rounded-lg border p-4 flex items-center gap-3
      {status.status === 'green' ? 'bg-green-50 border-green-200 dark:bg-green-900/20 dark:border-green-800' :
       status.status === 'yellow' ? 'bg-yellow-50 border-yellow-200 dark:bg-yellow-900/20 dark:border-yellow-800' :
       'bg-red-50 border-red-200 dark:bg-red-900/20 dark:border-red-800'}">
      {#if status.status === 'green'}
        <ShieldCheck class="w-6 h-6 text-green-600" />
        <div>
          <div class="font-semibold text-green-800 dark:text-green-300">Runtime Truthfulness: HEALTHY</div>
          <div class="text-sm text-green-700 dark:text-green-400">Metrics are hydrated from DB. No fatal invariant violations.</div>
        </div>
      {:else if status.status === 'yellow'}
        <AlertTriangle class="w-6 h-6 text-yellow-600" />
        <div>
          <div class="font-semibold text-yellow-800 dark:text-yellow-300">Runtime Truthfulness: DEGRADED</div>
          <div class="text-sm text-yellow-700 dark:text-yellow-400">Partial metrics or open contradictions detected. Review the details below.</div>
        </div>
      {:else}
        <ShieldAlert class="w-6 h-6 text-red-600" />
        <div>
          <div class="font-semibold text-red-800 dark:text-red-300">Runtime Truthfulness: CRITICAL</div>
          <div class="text-sm text-red-700 dark:text-red-400">Fatal invariant violation detected. Reconciliation may be blocked.</div>
        </div>
      {/if}
    </div>

    <!-- Metrics grid -->
    <div class="grid grid-cols-2 md:grid-cols-3 gap-4">
      <Card>
        <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Metric Truthfulness</div>
        <Badge color={trustColor(status.metric_truthfulness)}>{status.metric_truthfulness}</Badge>
      </Card>
      <Card>
        <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Unresolved Incidents</div>
        <div class="text-2xl font-bold {status.unresolved_incidents > 0 ? 'text-yellow-600' : 'text-green-600'}">
          {status.unresolved_incidents}
        </div>
      </Card>
      <Card>
        <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Unresolved Drifts</div>
        <div class="text-2xl font-bold {status.unresolved_drifts > 0 ? 'text-yellow-600' : 'text-green-600'}">
          {status.unresolved_drifts}
        </div>
      </Card>
      <Card>
        <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Open Contradictions</div>
        <div class="text-2xl font-bold {status.open_contradictions > 0 ? 'text-red-600' : 'text-green-600'}">
          {status.open_contradictions}
        </div>
      </Card>
      <Card>
        <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Stale Observations (30m)</div>
        <div class="text-2xl font-bold text-gray-700 dark:text-gray-300">{status.stale_observations_30m}</div>
      </Card>
      <Card>
        <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Fatal Invariants</div>
        <div class="text-2xl font-bold {status.fatal_invariant_count > 0 ? 'text-red-600' : 'text-green-600'}">
          {status.fatal_invariant_count}
        </div>
      </Card>
    </div>

    <!-- Provider observations by trust -->
    {#if status.provider_observations_by_trust && Object.keys(status.provider_observations_by_trust).length > 0}
      <Card>
        <h3 class="font-semibold text-gray-800 dark:text-white mb-3">Provider Observations by Trust Level</h3>
        <div class="space-y-2">
          {#each Object.entries(status.provider_observations_by_trust).sort() as [trust, count]}
            <div class="flex items-center justify-between text-sm">
              <Badge color={trust === 'authoritative' ? 'green' : trust === 'inferred' ? 'blue' : trust === 'ambiguous' ? 'yellow' : trust === 'contradicted' ? 'red' : 'gray'}>
                {trust}
              </Badge>
              <span class="font-mono text-gray-700 dark:text-gray-300">{count}</span>
            </div>
          {/each}
        </div>
      </Card>
    {/if}

    <!-- Metadata -->
    <div class="text-xs text-gray-400 space-y-1">
      <div>Truth audit interval: {status.truth_audit_interval}</div>
      <div>Provider silence total: {status.provider_silence_total}</div>
      <div>Provider contradictions total: {status.provider_contradictions_total}</div>
      <div>Queried at: {status.queried_at}</div>
    </div>

    <!-- Limitations -->
    <Alert color="gray">
      <span class="font-semibold">Limitations: </span>
      {status.limitations?.join(" · ")}
    </Alert>
  {/if}
</div>
