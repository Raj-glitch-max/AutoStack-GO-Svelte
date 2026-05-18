<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";

  let loading = true;
  let error = "";
  let data: any = null;

  const executionID = $page.params.id;

  async function fetchDetail() {
    loading = true;
    error = "";
    try {
      data = await client.send(`/api/v1/platform/executions/${executionID}`, { method: "GET" });
    } catch (e: any) {
      error = e?.message ?? "Failed to load execution detail";
    } finally {
      loading = false;
    }
  }

  onMount(fetchDetail);

  const stateColor: Record<string, string> = {
    verified: "green",
    partially_verified: "yellow",
    contradicted: "red",
    stale: "yellow",
    unverifiable: "red",
    stabilizing: "blue",
    acknowledged: "blue",
    submitted: "gray",
  };

  const effectColor: Record<string, string> = {
    allow: "green",
    deny: "red",
    "require-approval": "yellow",
    "require-multi-approval": "orange",
  };
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div class="flex items-center gap-3">
    <a
      href="/app/platform/executions"
      class="text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
    >← Executions</a>
    <span class="text-gray-300 dark:text-gray-600">/</span>
    <span class="font-mono text-sm text-gray-700 dark:text-gray-300">{executionID}</span>
  </div>

  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Execution Detail</h1>
    <button
      on:click={fetchDetail}
      class="rounded-md bg-primary-600 px-3 py-1.5 text-sm text-white hover:bg-primary-700"
    >
      Refresh
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if data}
    <!-- Verification state -->
    {#if data.verification_snapshot}
      {@const vs = data.verification_snapshot}
      <Card>
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300">Verification</h3>
          <Badge color={stateColor[vs.state] ?? "gray"}>{vs.state}</Badge>
        </div>
        <div class="grid grid-cols-2 gap-4 text-xs">
          <div>
            <p class="text-gray-500">Confidence</p>
            <p class="font-semibold text-gray-800 dark:text-gray-200">
              {(vs.confidence?.execution_confidence * 100).toFixed(0)}%
            </p>
          </div>
          <div>
            <p class="text-gray-500">Schema</p>
            <p class="font-mono text-gray-700 dark:text-gray-300">{vs.schema_version}</p>
          </div>
          {#if vs.recommendation}
            <div class="col-span-2">
              <p class="text-gray-500">Recommendation</p>
              <p class="text-gray-700 dark:text-gray-300">{vs.recommendation}</p>
            </div>
          {/if}
        </div>
        {#if vs.limitations?.length}
          <div class="mt-3 space-y-1">
            {#each vs.limitations as lim}
              <p class="text-xs text-yellow-700 dark:text-yellow-400">{lim}</p>
            {/each}
          </div>
        {/if}
      </Card>
    {/if}

    <!-- Governance summary -->
    {#if data.governance_snapshot}
      {@const gs = data.governance_snapshot}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Governance</h3>
        <div class="grid grid-cols-3 gap-4 text-xs">
          <div>
            <p class="text-gray-500">Policy decisions</p>
            <p class="font-semibold text-gray-800 dark:text-gray-200">{gs.decisions?.length ?? 0}</p>
          </div>
          <div>
            <p class="text-gray-500">Overrides</p>
            <p class="font-semibold text-gray-800 dark:text-gray-200">{gs.overrides?.length ?? 0}</p>
          </div>
          <div>
            <p class="text-gray-500">Audit entries</p>
            <p class="font-semibold text-gray-800 dark:text-gray-200">{gs.audit_chain?.entries?.length ?? 0}</p>
          </div>
        </div>
        {#if (gs.decisions ?? []).length > 0}
          <div class="mt-3 space-y-1">
            {#each gs.decisions.slice(0, 5) as dec}
              <div class="flex items-center gap-2">
                <Badge color={effectColor[dec.effect] ?? "gray"} class="text-xs">{dec.effect}</Badge>
                <span class="text-xs font-mono text-gray-600 dark:text-gray-400">{dec.policy_id}</span>
              </div>
            {/each}
            {#if gs.decisions.length > 5}
              <p class="text-xs text-gray-400">+{gs.decisions.length - 5} more —
                <a href="/app/platform/governance?id={executionID}" class="text-primary-600 hover:underline">view all</a>
              </p>
            {/if}
          </div>
        {/if}
      </Card>
    {/if}

    <!-- Scheduling summary -->
    {#if data.scheduling_snapshot}
      {@const ss = data.scheduling_snapshot}
      <Card>
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300">Scheduler</h3>
          <Badge color={ss.scheduler_state === "running" ? "green" : ss.scheduler_state === "paused" ? "yellow" : "gray"}>
            {ss.scheduler_state}
          </Badge>
        </div>
        <div class="text-xs space-y-1">
          <p class="text-gray-500">
            Queue: <span class="font-semibold text-gray-800 dark:text-gray-200">{ss.queue?.entries?.length ?? 0} stages</span>
          </p>
          <p class="text-gray-500">
            Replay order: <span class="font-semibold text-gray-800 dark:text-gray-200">{ss.replay_order?.length ?? 0} recorded</span>
          </p>
        </div>
      </Card>
    {/if}

    <!-- Topology summary -->
    {#if data.topology_snapshot}
      {@const ts = data.topology_snapshot}
      <Card>
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300">Topology</h3>
          {#if ts.cycle_detected}
            <Badge color="red">Cycle Detected</Badge>
          {:else}
            <Badge color="green">Acyclic</Badge>
          {/if}
        </div>
        <div class="text-xs space-y-1">
          <p class="text-gray-500">
            Providers: <span class="font-semibold text-gray-800 dark:text-gray-200">{ts.nodes?.length ?? 0}</span>
          </p>
          <p class="text-gray-500">
            Cross-provider edges: <span class="font-semibold text-gray-800 dark:text-gray-200">
              {(ts.edges ?? []).filter((e) => e.cross_provider).length}
            </span>
          </p>
        </div>
      </Card>
    {/if}

    <!-- Quick navigation -->
    <div class="flex flex-wrap gap-3 pt-2">
      <a href="/app/platform/governance?id={executionID}"
        class="rounded-md border border-gray-200 dark:border-gray-700 px-3 py-1.5 text-xs text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-800">
        Full Governance →
      </a>
      <a href="/app/platform/topology?id={executionID}"
        class="rounded-md border border-gray-200 dark:border-gray-700 px-3 py-1.5 text-xs text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-800">
        Topology →
      </a>
      <a href="/app/platform/scheduler?id={executionID}"
        class="rounded-md border border-gray-200 dark:border-gray-700 px-3 py-1.5 text-xs text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-800">
        Scheduler →
      </a>
      <a href="/app/platform/replay?id={executionID}"
        class="rounded-md border border-gray-200 dark:border-gray-700 px-3 py-1.5 text-xs text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-800">
        Replay →
      </a>
    </div>

    {#if data.limitations?.length}
      <div class="rounded-md border border-yellow-200 bg-yellow-50 dark:bg-yellow-900/20 p-4 space-y-1">
        <p class="text-xs font-semibold uppercase tracking-wide text-yellow-700 dark:text-yellow-400">Limitations</p>
        {#each data.limitations as lim}
          <p class="text-xs text-yellow-800 dark:text-yellow-300">{lim}</p>
        {/each}
      </div>
    {/if}
  {/if}
</div>
