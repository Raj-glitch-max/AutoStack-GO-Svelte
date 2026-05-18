<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";

  import { page } from "$app/stores";

  let loading = true;
  let error = "";
  let data: any = null;
  let executionID = $page.url.searchParams.get("id") ?? "";

  async function fetchScheduler() {
    if (!executionID.trim()) {
      error = "Enter an execution ID";
      return;
    }
    loading = true;
    error = "";
    try {
      data = await client.send(`/api/v1/platform/scheduler/${executionID.trim()}`, { method: "GET" });
    } catch (e: any) {
      error = e?.message ?? "Failed to load scheduler state";
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (executionID) {
      fetchScheduler();
    } else {
      loading = false;
    }
  });

  const stateColor: Record<string, string> = {
    running: "green",
    paused: "yellow",
    drained: "gray",
    blocked: "red",
  };

  const riskColor: Record<string, string> = {
    low: "green",
    medium: "yellow",
    high: "orange",
    critical: "red",
  };
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Scheduler Surface</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Deterministic stage queue, concurrency groups, and fairness state for an execution.
    </p>
  </div>

  <div class="flex gap-3">
    <input
      bind:value={executionID}
      placeholder="Execution ID"
      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
    />
    <button
      on:click={fetchScheduler}
      class="rounded-md bg-primary-600 px-4 py-2 text-sm text-white hover:bg-primary-700"
    >
      Load
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if data}
    <div class="flex items-center gap-3">
      <Badge color={stateColor[data.snapshot?.scheduler_state] ?? "gray"}>
        {data.snapshot?.scheduler_state ?? "unknown"}
      </Badge>
      <span class="text-xs text-gray-500">
        Queue: {data.snapshot?.queue?.entries?.length ?? 0} entries ·
        Replay order: {data.snapshot?.replay_order?.length ?? 0} stages
      </span>
    </div>

    <div class="grid gap-6 md:grid-cols-2">
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Stage Queue</h3>
        {#each data.snapshot?.queue?.entries ?? [] as entry}
          <div class="py-1.5 border-b border-gray-100 dark:border-gray-700 last:border-0 flex items-center justify-between">
            <div>
              <span class="text-xs font-mono text-gray-800 dark:text-gray-200">{entry.stage_id}</span>
              <span class="ml-2 text-xs text-gray-400">depth {entry.dependency_depth}</span>
            </div>
            <Badge color={riskColor[entry.risk_level] ?? "gray"} class="text-xs">
              {entry.risk_level}
            </Badge>
          </div>
        {:else}
          <p class="text-xs text-gray-400">Queue is empty.</p>
        {/each}
      </Card>

      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Concurrency Groups</h3>
        {#each data.snapshot?.concurrency_groups ?? [] as group}
          <div class="py-1.5 border-b border-gray-100 dark:border-gray-700 last:border-0">
            <span class="text-xs font-mono text-gray-700 dark:text-gray-300">{group.group_id}</span>
            <div class="flex flex-wrap gap-1 mt-1">
              {#each group.stage_ids as sid}
                <Badge color="blue" class="text-xs font-mono">{sid}</Badge>
              {/each}
            </div>
            <p class="text-xs text-gray-400 mt-0.5">{group.rationale}</p>
          </div>
        {:else}
          <p class="text-xs text-gray-400">No concurrent groups (linear dependency chain).</p>
        {/each}
      </Card>
    </div>

    <!-- Blocked / starved stages -->
    {#if (data.snapshot?.fairness_state?.blocked_stages ?? []).length > 0}
      <Card>
        <h3 class="text-sm font-semibold text-red-700 dark:text-red-400 mb-3">Blocked / Starved Stages</h3>
        {#each data.snapshot.fairness_state.blocked_stages as bs}
          <div class="py-1.5 border-b border-gray-100 dark:border-gray-700 last:border-0 flex items-center justify-between">
            <span class="text-xs font-mono text-gray-700 dark:text-gray-300">{bs.stage_id}</span>
            <div class="flex items-center gap-2">
              <Badge color="red" class="text-xs">{bs.block_reason}</Badge>
              <span class="text-xs text-gray-400">deferred {bs.deferred_count}×</span>
            </div>
          </div>
        {/each}
      </Card>
    {/if}

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
