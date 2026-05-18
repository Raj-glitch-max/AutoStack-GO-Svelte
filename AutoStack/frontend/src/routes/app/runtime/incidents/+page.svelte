<script lang="ts">
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { client } from "$lib/pocketbase";
  import { Badge, Input, Select, Button, Spinner, Alert, Table, TableHead, TableHeadCell, TableBody, TableBodyRow, TableBodyCell } from "flowbite-svelte";
  import { RefreshCw } from "lucide-svelte";

  let loading = true;
  let error = "";
  let incidents: any[] = [];
  let count = 0;

  let filterStatus = "open,escalating,acknowledged,reopened";
  let limit = 100;

  const statusOptions = [
    { value: "open,escalating,acknowledged,reopened", name: "Active (open/escalating)" },
    { value: "resolved", name: "Resolved" },
    { value: "", name: "All" },
  ];

  async function fetchIncidents() {
    loading = true;
    error = "";
    try {
      const params: Record<string, string> = { limit: String(limit) };
      // The API doesn't support multi-value status yet; use "all" if not single value
      if (filterStatus && !filterStatus.includes(",")) params.status = filterStatus;
      const qs = new URLSearchParams(params).toString();
      const res = await client.send(`/api/v1/reconciler/incidents?${qs}`, { method: "GET" });
      incidents = res.incidents ?? [];
      count = res.count ?? 0;
    } catch (e: any) {
      error = e?.message ?? "Failed to load incidents";
    } finally {
      loading = false;
    }
  }

  onMount(fetchIncidents);

  function severityColor(sev: string) {
    switch (sev) {
      case "critical": return "red";
      case "high": return "orange";
      case "medium": return "yellow";
      default: return "gray";
    }
  }

  function statusColor(s: string) {
    switch (s) {
      case "open": return "blue";
      case "escalating": return "red";
      case "acknowledged": return "yellow";
      case "resolved": return "green";
      case "reopened": return "orange";
      case "suppressed": return "dark";
      default: return "gray";
    }
  }
</script>

<div class="max-w-screen-2xl mx-auto px-4 pt-16 pb-8 space-y-4">
  <div class="flex items-center justify-between">
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Incidents</h1>
    <button on:click={fetchIncidents} class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400 hover:text-gray-900">
      <RefreshCw class="w-4 h-4" /> Refresh
    </button>
  </div>

  <p class="text-sm text-gray-500 dark:text-gray-400">
    Incidents are persisted operational events. Click an incident ID to view its full narrative,
    related provider observations, and the escalation chain. The runtime never auto-resolves incidents —
    operator action is required.
  </p>

  <div class="grid grid-cols-2 gap-3 max-w-sm">
    <Select size="sm" bind:value={filterStatus} items={statusOptions} on:change={fetchIncidents} />
    <Button size="sm" color="primary" on:click={fetchIncidents}>Refresh</Button>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><Spinner /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else}
    <div class="text-sm text-gray-500">Showing {incidents.length} incident(s)</div>

    <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
      <Table striped={true} hoverable={true}>
        <TableHead>
          <TableHeadCell>ID</TableHeadCell>
          <TableHeadCell>Source</TableHeadCell>
          <TableHeadCell>Severity</TableHeadCell>
          <TableHeadCell>Status</TableHeadCell>
          <TableHeadCell>Escalations</TableHeadCell>
          <TableHeadCell>First Seen</TableHeadCell>
          <TableHeadCell>Last Seen</TableHeadCell>
          <TableHeadCell>Summary</TableHeadCell>
        </TableHead>
        <TableBody>
          {#each incidents as inc}
            <TableBodyRow
              class="cursor-pointer"
              on:click={() => goto(`/app/runtime/incidents/${inc.id}`)}
            >
              <TableBodyCell class="font-mono text-xs">{inc.id?.slice(0, 8)}…</TableBodyCell>
              <TableBodyCell class="text-xs">{inc.source}</TableBodyCell>
              <TableBodyCell><Badge color={severityColor(inc.severity)}>{inc.severity}</Badge></TableBodyCell>
              <TableBodyCell><Badge color={statusColor(inc.status)}>{inc.status}</Badge></TableBodyCell>
              <TableBodyCell class="text-center font-mono">
                {#if inc.escalation_count > 5}
                  <span class="text-red-600 font-bold">{inc.escalation_count}</span>
                {:else}
                  {inc.escalation_count}
                {/if}
              </TableBodyCell>
              <TableBodyCell class="text-xs whitespace-nowrap">{inc.first_seen_at}</TableBodyCell>
              <TableBodyCell class="text-xs whitespace-nowrap">{inc.last_seen_at}</TableBodyCell>
              <TableBodyCell class="text-xs max-w-xs truncate" title={inc.forensic_summary}>
                {inc.forensic_summary}
              </TableBodyCell>
            </TableBodyRow>
          {/each}
          {#if incidents.length === 0}
            <TableBodyRow>
              <TableBodyCell colspan={8} class="text-center text-gray-400 py-8">
                No incidents match the current filter.
              </TableBodyCell>
            </TableBodyRow>
          {/if}
        </TableBody>
      </Table>
    </div>
  {/if}
</div>
