<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";

  import { page } from "$app/stores";

  let loading = true;
  let error = "";
  let data: any = null;
  let executionID = $page.url.searchParams.get("id") ?? "";

  async function fetchTopology() {
    if (!executionID.trim()) {
      error = "Enter an execution ID";
      return;
    }
    loading = true;
    error = "";
    try {
      data = await client.send(`/api/v1/platform/topology/${executionID.trim()}`, { method: "GET" });
    } catch (e: any) {
      error = e?.message ?? "Failed to load topology";
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (executionID) {
      fetchTopology();
    } else {
      loading = false;
    }
  });
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Topology Visualization</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Multi-provider deployment topology, failure domains, and cross-provider dependencies.
    </p>
  </div>

  <div class="flex gap-3">
    <input
      bind:value={executionID}
      placeholder="Execution ID"
      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
    />
    <button
      on:click={fetchTopology}
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
    <div class="grid gap-6 md:grid-cols-2">
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Provider Nodes</h3>
        {#each data.snapshot?.nodes ?? [] as node}
          <div class="flex items-center justify-between py-1.5 border-b border-gray-100 dark:border-gray-700 last:border-0">
            <span class="text-sm font-mono text-gray-800 dark:text-gray-200">{node.provider_id}</span>
            <Badge color="blue">{node.node_count} node{node.node_count !== 1 ? "s" : ""}</Badge>
          </div>
        {:else}
          <p class="text-xs text-gray-400">No provider nodes.</p>
        {/each}
      </Card>

      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Cross-Provider Edges</h3>
        {#each data.snapshot?.edges ?? [] as edge}
          <div class="py-1.5 border-b border-gray-100 dark:border-gray-700 last:border-0">
            <div class="flex items-center gap-2">
              <span class="text-xs font-mono text-gray-700 dark:text-gray-300">{edge.from_provider} → {edge.to_provider}</span>
              {#if edge.cross_provider}
                <Badge color="yellow" class="text-xs">cross-provider</Badge>
              {/if}
            </div>
            <p class="text-xs text-gray-500 mt-0.5">{edge.explanation}</p>
          </div>
        {:else}
          <p class="text-xs text-gray-400">No topology edges.</p>
        {/each}
      </Card>
    </div>

    {#if data.snapshot?.cycle_detected}
      <Alert color="red">
        <strong>Cycle Detected</strong> — The execution plan contains a dependency cycle. Execution cannot proceed safely.
      </Alert>
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
