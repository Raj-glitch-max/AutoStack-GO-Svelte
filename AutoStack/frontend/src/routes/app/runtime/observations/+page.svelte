<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Input, Select, Button, Spinner, Alert, Table, TableHead, TableHeadCell, TableBody, TableBodyRow, TableBodyCell } from "flowbite-svelte";
  import { RefreshCw } from "lucide-svelte";

  let loading = true;
  let error = "";
  let observations: any[] = [];
  let count = 0;
  let queriedAt = "";

  // Filters
  let filterTarget = "";
  let filterProvider = "";
  let filterTrust = "";
  let filterSource = "";
  let filterContradiction = "";
  let filterSince = "";
  let limit = 100;

  const trustOptions = [
    { value: "", name: "All trust levels" },
    { value: "authoritative", name: "Authoritative" },
    { value: "inferred", name: "Inferred" },
    { value: "ambiguous", name: "Ambiguous" },
    { value: "stale", name: "Stale" },
    { value: "contradicted", name: "Contradicted" },
  ];

  async function fetchObservations() {
    loading = true;
    error = "";
    try {
      const params: Record<string, string> = { limit: String(limit) };
      if (filterTarget) params.target = filterTarget;
      if (filterProvider) params.provider = filterProvider;
      if (filterTrust) params.trust = filterTrust;
      if (filterSource) params.source = filterSource;
      if (filterContradiction) params.contradiction = filterContradiction;
      if (filterSince) params.since = filterSince;
      const qs = new URLSearchParams(params).toString();
      const res = await client.send(`/api/v1/reconciler/observations?${qs}`, { method: "GET" });
      observations = res.observations ?? [];
      count = res.count ?? 0;
      queriedAt = res.queried_at ?? "";
    } catch (e: any) {
      error = e?.message ?? "Failed to load observations";
    } finally {
      loading = false;
    }
  }

  onMount(fetchObservations);

  function trustBadgeColor(trust: string) {
    switch (trust) {
      case "authoritative": return "green";
      case "inferred": return "blue";
      case "ambiguous": return "yellow";
      case "stale": return "dark";
      case "contradicted": return "red";
      default: return "gray";
    }
  }
</script>

<div class="max-w-screen-2xl mx-auto px-4 pt-16 pb-8 space-y-4">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Provider Observations</h1>
    <button on:click={fetchObservations} class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900">
      <RefreshCw class="w-4 h-4" /> Refresh
    </button>
  </div>

  <p class="text-sm text-gray-500 dark:text-gray-400">
    Every provider state read is persisted here before influencing runtime behavior.
    Contradiction-tagged rows (highlighted) are immortal — they are never purged by retention.
    Unknown provider states are preserved verbatim; trust is downgraded to ambiguous.
  </p>

  <!-- Filters -->
  <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
    <Input size="sm" placeholder="Filter by target ID" bind:value={filterTarget} />
    <Input size="sm" placeholder="Filter by provider (aws-ecs, cloud-run…)" bind:value={filterProvider} />
    <Select size="sm" bind:value={filterTrust} items={trustOptions} />
    <Input size="sm" placeholder="Filter by source (poll, deploy-result…)" bind:value={filterSource} />
    <Select size="sm" bind:value={filterContradiction} items={[
      {value: "", name: "All"},
      {value: "true", name: "Contradiction only"}
    ]} />
    <Input size="sm" type="datetime-local" bind:value={filterSince} />
    <Button size="sm" color="primary" on:click={fetchObservations}>Apply</Button>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><Spinner /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else}
    <div class="text-sm text-gray-500">Showing {count} observation(s) · {queriedAt}</div>

    <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
      <Table striped={true}>
        <TableHead>
          <TableHeadCell>Observed At</TableHeadCell>
          <TableHeadCell>Target</TableHeadCell>
          <TableHeadCell>Provider</TableHeadCell>
          <TableHeadCell>Trust</TableHeadCell>
          <TableHeadCell>Provider State</TableHeadCell>
          <TableHeadCell>Expected State</TableHeadCell>
          <TableHeadCell>Source</TableHeadCell>
          <TableHeadCell>Contradiction</TableHeadCell>
        </TableHead>
        <TableBody>
          {#each observations as obs}
            <TableBodyRow class={obs.contradiction_detected ? "bg-red-50 dark:bg-red-900/20" : ""}>
              <TableBodyCell class="font-mono text-xs whitespace-nowrap">{obs.observed_at}</TableBodyCell>
              <TableBodyCell class="font-mono text-xs max-w-xs truncate" title={obs.target}>{obs.target}</TableBodyCell>
              <TableBodyCell class="text-sm">{obs.provider}</TableBodyCell>
              <TableBodyCell>
                <Badge color={trustBadgeColor(obs.observation_trust)} title="Why trusted? Source: {obs.observation_source}">
                  {obs.observation_trust}
                </Badge>
              </TableBodyCell>
              <TableBodyCell class="font-mono text-sm">{obs.provider_state || "—"}</TableBodyCell>
              <TableBodyCell class="font-mono text-sm">{obs.runtime_expected_state || "—"}</TableBodyCell>
              <TableBodyCell class="text-xs">{obs.observation_source}</TableBodyCell>
              <TableBodyCell>
                {#if obs.contradiction_detected}
                  <Badge color="red">YES</Badge>
                {:else}
                  <span class="text-gray-400 text-xs">no</span>
                {/if}
              </TableBodyCell>
            </TableBodyRow>
          {/each}
          {#if observations.length === 0}
            <TableBodyRow>
              <TableBodyCell colspan={8} class="text-center text-gray-400 py-8">
                No observations match the current filters.
              </TableBodyCell>
            </TableBodyRow>
          {/if}
        </TableBody>
      </Table>
    </div>
  {/if}
</div>
