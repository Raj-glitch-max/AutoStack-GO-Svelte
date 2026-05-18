<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";

  let loading = true;
  let error = "";
  let executions: any[] = [];

  async function fetchDrifts() {
    loading = true;
    error = "";
    try {
      const all = await client.send("/api/v1/platform/executions", { method: "GET" });
      // Surface executions with contradictions or stale state as "drifts"
      executions = (all ?? []).filter(
        (e: any) => e.has_contradictions || e.verification_state === "stale" || e.verification_state === "contradicted"
      );
    } catch (e: any) {
      error = e?.message ?? "Failed to load drift data";
    } finally {
      loading = false;
    }
  }

  onMount(fetchDrifts);

  const stateColor: Record<string, string> = {
    contradicted: "red",
    stale: "yellow",
    unverifiable: "red",
  };
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Drift Dashboard</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Executions with provider state contradictions or stale observations.
        No automatic remediation — operator review required.
      </p>
    </div>
    <button
      on:click={fetchDrifts}
      class="rounded-md bg-primary-600 px-3 py-1.5 text-sm text-white hover:bg-primary-700"
    >
      Refresh
    </button>
  </div>

  <div class="rounded-md border border-yellow-200 bg-yellow-50 dark:bg-yellow-900/20 p-3">
    <p class="text-xs text-yellow-800 dark:text-yellow-300">
      Drift detection is read-only. AutoStack does not automatically correct drift.
      All remediation actions require explicit operator approval.
    </p>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if executions.length === 0}
    <Card>
      <div class="text-center py-8">
        <p class="text-sm font-medium text-green-700 dark:text-green-400">No drifts detected</p>
        <p class="text-xs text-gray-500 mt-1">All executions are within expected verification state.</p>
      </div>
    </Card>
  {:else}
    <div class="space-y-3">
      {#each executions as ex}
        <Card>
          <div class="flex items-start justify-between">
            <div>
              <p class="text-sm font-mono font-medium text-gray-900 dark:text-white">{ex.execution_id}</p>
              <p class="text-xs text-gray-500 mt-0.5">
                Confidence: {(ex.confidence_score * 100).toFixed(0)}% ·
                Stages: {ex.completed_stages}/{ex.total_stages}
              </p>
            </div>
            <div class="flex gap-2">
              <Badge color={stateColor[ex.verification_state] ?? "gray"}>
                {ex.verification_state}
              </Badge>
              {#if ex.has_contradictions}
                <Badge color="red">contradiction</Badge>
              {/if}
            </div>
          </div>
          <div class="mt-3 flex gap-2">
            <a
              href="/app/platform/executions/{ex.execution_id}"
              class="text-xs text-primary-600 hover:underline dark:text-primary-400"
            >View execution detail →</a>
            <a
              href="/app/platform/governance?id={ex.execution_id}"
              class="text-xs text-primary-600 hover:underline dark:text-primary-400"
            >View governance →</a>
          </div>
        </Card>
      {/each}
    </div>
  {/if}
</div>
