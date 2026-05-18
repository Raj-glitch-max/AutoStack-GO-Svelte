<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Input, Select, Button, Spinner, Alert, Table, TableHead, TableHeadCell, TableBody, TableBodyRow, TableBodyCell } from "flowbite-svelte";
  import { RefreshCw, ArrowUpCircle } from "lucide-svelte";

  let loading = true;
  let error = "";
  let drifts: any[] = [];
  let count = 0;
  let queriedAt = "";

  let filterTarget = "";
  let filterProvider = "";
  let filterSeverity = "";
  let filterUnresolved = "true";

  const severityOptions = [
    { value: "", name: "All severities" },
    { value: "warning", name: "Warning" },
    { value: "high", name: "High" },
    { value: "fatal", name: "Fatal" },
  ];

  async function fetchDrifts() {
    loading = true;
    error = "";
    try {
      const params: Record<string, string> = { limit: "200" };
      if (filterTarget) params.target = filterTarget;
      if (filterProvider) params.provider = filterProvider;
      if (filterSeverity) params.severity = filterSeverity;
      if (filterUnresolved) params.unresolved = filterUnresolved;
      const qs = new URLSearchParams(params).toString();
      const res = await client.send(`/api/v1/reconciler/drifts?${qs}`, { method: "GET" });
      drifts = res.drifts ?? [];
      count = res.count ?? 0;
      queriedAt = res.queried_at ?? "";
    } catch (e: any) {
      error = e?.message ?? "Failed to load drifts";
    } finally {
      loading = false;
    }
  }

  onMount(fetchDrifts);

  function isUnresolved(d: any) {
    return !d.resolved_at || d.resolved_at === "";
  }

  function severityColor(sev: string) {
    switch (sev) {
      case "fatal": return "red";
      case "high": return "orange";
      case "warning": return "yellow";
      default: return "gray";
    }
  }

  function driftAge(firstSeen: string): string {
    if (!firstSeen) return "—";
    const ms = Date.now() - new Date(firstSeen).getTime();
    const hours = Math.floor(ms / 3600000);
    if (hours > 24) return `${Math.floor(hours / 24)}d ${hours % 24}h`;
    if (hours > 0) return `${hours}h`;
    return `${Math.floor(ms / 60000)}m`;
  }
</script>

<div class="max-w-screen-2xl mx-auto px-4 pt-16 pb-8 space-y-4">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Provider Drift Records</h1>
    <button on:click={fetchDrifts} class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900">
      <RefreshCw class="w-4 h-4" /> Refresh
    </button>
  </div>

  <p class="text-sm text-gray-500 dark:text-gray-400">
    A drift record is opened when the provider's reported state contradicts the runtime's expected state.
    Unresolved drifts are never purged by retention — they are immortal until an operator resolves them.
    Ambiguous observations do not auto-resolve drifts; only a confirmed authoritative state can resolve them.
  </p>

  <!-- Filters -->
  <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
    <Input size="sm" placeholder="Filter by target ID" bind:value={filterTarget} />
    <Input size="sm" placeholder="Filter by provider" bind:value={filterProvider} />
    <Select size="sm" bind:value={filterSeverity} items={severityOptions} />
    <Select size="sm" bind:value={filterUnresolved} items={[
      {value: "true", name: "Unresolved only"},
      {value: "", name: "All (including resolved)"}
    ]} />
    <Button size="sm" color="primary" on:click={fetchDrifts}>Apply</Button>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><Spinner /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else}
    <div class="text-sm text-gray-500">Showing {count} drift record(s) · {queriedAt}</div>

    <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
      <Table striped={true}>
        <TableHead>
          <TableHeadCell>Target</TableHeadCell>
          <TableHeadCell>Provider</TableHeadCell>
          <TableHeadCell>Expected</TableHeadCell>
          <TableHeadCell>Observed</TableHeadCell>
          <TableHeadCell>Severity</TableHeadCell>
          <TableHeadCell>Drift Count</TableHeadCell>
          <TableHeadCell>Age</TableHeadCell>
          <TableHeadCell>Status</TableHeadCell>
        </TableHead>
        <TableBody>
          {#each drifts as d}
            {@const unresolved = isUnresolved(d)}
            <TableBodyRow class={unresolved ? "bg-orange-50 dark:bg-orange-900/10" : "opacity-60"}>
              <TableBodyCell class="font-mono text-xs max-w-xs truncate" title={d.target}>{d.target}</TableBodyCell>
              <TableBodyCell class="text-sm">{d.provider}</TableBodyCell>
              <TableBodyCell class="font-mono text-sm">{d.expected_state}</TableBodyCell>
              <TableBodyCell class="font-mono text-sm text-red-600 dark:text-red-400">{d.observed_state}</TableBodyCell>
              <TableBodyCell>
                <Badge color={severityColor(d.severity)}>{d.severity}</Badge>
              </TableBodyCell>
              <TableBodyCell class="text-center font-mono">
                {#if d.drift_count > 10}
                  <span class="text-red-600 font-bold">{d.drift_count}</span>
                {:else}
                  {d.drift_count}
                {/if}
              </TableBodyCell>
              <TableBodyCell class="text-sm whitespace-nowrap">
                {#if unresolved && driftAge(d.first_seen_at).includes('d')}
                  <span class="text-red-600 font-semibold flex items-center gap-1">
                    <ArrowUpCircle class="w-3 h-3" />{driftAge(d.first_seen_at)}
                  </span>
                {:else}
                  {driftAge(d.first_seen_at)}
                {/if}
              </TableBodyCell>
              <TableBodyCell>
                {#if unresolved}
                  <Badge color="red">OPEN</Badge>
                {:else}
                  <Badge color="green">resolved</Badge>
                {/if}
              </TableBodyCell>
            </TableBodyRow>
          {/each}
          {#if drifts.length === 0}
            <TableBodyRow>
              <TableBodyCell colspan={8} class="text-center text-gray-400 py-8">
                No drift records match the current filters.
              </TableBodyCell>
            </TableBodyRow>
          {/if}
        </TableBody>
      </Table>
    </div>
  {/if}
</div>
