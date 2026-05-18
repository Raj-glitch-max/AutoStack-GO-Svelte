<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Spinner, Alert, Table, TableHead, TableHeadCell, TableBody, TableBodyRow, TableBodyCell } from "flowbite-svelte";

  let loading = true;
  let error = "";
  let executions: any[] = [];

  async function fetchExecutions() {
    loading = true;
    error = "";
    try {
      executions = await client.send("/api/v1/platform/executions", { method: "GET" });
    } catch (e: any) {
      error = e?.message ?? "Failed to load executions";
    } finally {
      loading = false;
    }
  }

  onMount(fetchExecutions);

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
</script>

<div class="max-w-6xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Execution Explorer</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        All executions with verification state and audit summary.
      </p>
    </div>
    <button
      on:click={fetchExecutions}
      class="rounded-md bg-primary-600 px-3 py-1.5 text-sm text-white hover:bg-primary-700"
    >
      Refresh
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if executions.length === 0}
    <p class="text-sm text-gray-500 dark:text-gray-400">No executions found.</p>
  {:else}
    <Table>
      <TableHead>
        <TableHeadCell>Source</TableHeadCell>
        <TableHeadCell>Execution ID</TableHeadCell>
        <TableHeadCell>State</TableHeadCell>
        <TableHeadCell>Confidence</TableHeadCell>
        <TableHeadCell>Stages</TableHeadCell>
        <TableHeadCell>Audit Entries</TableHeadCell>
        <TableHeadCell>Flags</TableHeadCell>
      </TableHead>
      <TableBody>
        {#each executions as ex}
          <TableBodyRow>
            <TableBodyCell>
              {#if ex.source === 'demo'}
                <Badge color="yellow" class="text-[10px] font-mono">DEMO</Badge>
              {:else}
                <Badge color="green" class="text-[10px] font-mono">LIVE</Badge>
              {/if}
            </TableBodyCell>
            <TableBodyCell>
              <a
                href="/app/platform/executions/{ex.execution_id}"
                class="font-mono text-xs text-primary-600 hover:underline dark:text-primary-400"
              >{ex.execution_id}</a>
            </TableBodyCell>
            <TableBodyCell>
              <Badge color={stateColor[ex.verification_state] ?? "gray"}>
                {ex.verification_state}
              </Badge>
            </TableBodyCell>
            <TableBodyCell>
              <span class="font-mono text-sm">{(ex.confidence_score * 100).toFixed(0)}%</span>
            </TableBodyCell>
            <TableBodyCell>
              {ex.completed_stages} / {ex.total_stages}
            </TableBodyCell>
            <TableBodyCell>{ex.audit_entry_count}</TableBodyCell>
            <TableBodyCell>
              <div class="flex gap-1">
                {#if ex.has_contradictions}
                  <Badge color="red" class="text-xs">contradiction</Badge>
                {/if}
                {#if ex.has_overrides}
                  <Badge color="purple" class="text-xs">override</Badge>
                {/if}
              </div>
            </TableBodyCell>
          </TableBodyRow>
        {/each}
      </TableBody>
    </Table>
  {/if}
</div>
