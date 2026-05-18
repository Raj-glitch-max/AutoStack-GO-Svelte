<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert, Select } from "flowbite-svelte";

  let loading = true;
  let error = "";
  let data: any = null;
  let executionID = "";
  let executionList: Array<{ value: string; name: string; state: string }> = [];
  type TimelineTab = "execution" | "contradictions" | "replay" | "governance" | "audit";
  let activeTab: TimelineTab = "execution";
  function setTab(t: string) { activeTab = t as TimelineTab; }

  async function fetchExecutionList() {
    try {
      const list: any = await client.send("/api/v1/platform/executions", { method: "GET" });
      executionList = (list ?? []).map((e: any) => ({
        value: e.execution_id,
        name: `[${(e.source ?? 'live').toUpperCase()}]  ${e.execution_id}  (${e.verification_state})`,
        state: e.verification_state,
      }));
    } catch (e: any) {
      // List unavailable — leave dropdown empty; manual entry still works.
    }
  }

  async function fetchTimeline() {
    if (!executionID.trim()) {
      error = "Select or enter an execution ID";
      return;
    }
    loading = true;
    error = "";
    try {
      data = await client.send(`/api/v1/platform/timeline/${executionID.trim()}`, { method: "GET" });
    } catch (e: any) {
      error = e?.message ?? "Failed to load timeline";
    } finally {
      loading = false;
    }
  }

  onMount(async () => {
    await fetchExecutionList();
    const urlParam = new URL(window.location.href).searchParams.get("id");
    if (urlParam) {
      executionID = urlParam;
      await fetchTimeline();
    } else if (executionList.length > 0) {
      executionID = executionList[0].value;
      await fetchTimeline();
    } else {
      loading = false;
    }
  });

  $: if (executionID && executionList.length > 0) {
    // Reload when selection changes after first render.
  }

  function onSelect() {
    if (executionID) fetchTimeline();
  }

  const phaseColor: Record<string, string> = {
    planned: "blue",
    running: "yellow",
    completed: "green",
    failed: "red",
    blocked: "purple",
    rolled_back: "orange",
  };

  const contradictionColor: Record<string, string> = {
    resolved: "green",
    unresolved: "red",
    escalated: "orange",
  };

  const governanceColor: Record<string, string> = {
    allow: "green",
    deny: "red",
    "require-approval": "yellow",
    override: "purple",
  };

  function formatTs(ts: string): string {
    if (!ts) return "—";
    try { return new Date(ts).toLocaleString(); } catch { return ts; }
  }
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Operator Timeline</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Chronological view of execution phases, contradictions, replay history, governance decisions, and audit chain.
      Read-only — no mutations occur from this surface.
    </p>
  </div>

  <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
    {#if executionList.length > 0}
      <Select
        class="flex-1"
        bind:value={executionID}
        items={executionList}
        on:change={onSelect}
        size="sm"
      />
    {/if}
    <input
      bind:value={executionID}
      on:keydown={(e) => e.key === "Enter" && fetchTimeline()}
      placeholder="…or enter an execution ID manually"
      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
    />
    <button
      on:click={fetchTimeline}
      class="rounded-md bg-primary-600 px-4 py-2 text-sm text-white hover:bg-primary-700 whitespace-nowrap"
    >
      Load timeline
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if data}
    <!-- Tab bar -->
    <div class="border-b border-gray-200 dark:border-gray-700">
      <nav class="-mb-px flex gap-4">
        {#each [
          { key: "execution", label: "Execution" },
          { key: "contradictions", label: "Contradictions" },
          { key: "replay", label: "Replay" },
          { key: "governance", label: "Governance" },
          { key: "audit", label: "Audit Chain" },
        ] as tab}
          <button
            on:click={() => setTab(tab.key)}
            class="pb-2 text-sm font-medium border-b-2 transition-colors {activeTab === tab.key
              ? 'border-primary-600 text-primary-600 dark:text-primary-400'
              : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'}"
          >
            {tab.label}
          </button>
        {/each}
      </nav>
    </div>

    <!-- Execution Timeline -->
    {#if activeTab === "execution"}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-4">Execution Phases</h3>
        {#if (data.execution_phases ?? []).length === 0}
          <p class="text-xs text-gray-400">No execution phases recorded.</p>
        {:else}
          <ol class="relative border-l border-gray-200 dark:border-gray-700 ml-3 space-y-4">
            {#each data.execution_phases as phase}
              <li class="ml-4">
                <div class="absolute -left-1.5 mt-1.5 h-3 w-3 rounded-full border-2 border-white bg-gray-400 dark:border-gray-900"></div>
                <div class="flex items-center gap-2">
                  <Badge color={phaseColor[phase.status] ?? "gray"} class="text-xs">{phase.status}</Badge>
                  <span class="text-sm font-medium text-gray-800 dark:text-gray-200">{phase.stage_id}</span>
                </div>
                <p class="text-xs text-gray-400 mt-0.5">{formatTs(phase.started_at)} → {phase.ended_at ? formatTs(phase.ended_at) : "running"}</p>
                {#if phase.note}
                  <p class="text-xs text-gray-500 mt-0.5">{phase.note}</p>
                {/if}
              </li>
            {/each}
          </ol>
        {/if}
      </Card>
    {/if}

    <!-- Contradiction Timeline -->
    {#if activeTab === "contradictions"}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-4">Contradiction History</h3>
        {#if (data.contradictions ?? []).length === 0}
          <p class="text-xs text-gray-400">No contradictions recorded for this execution.</p>
        {:else}
          <div class="space-y-3">
            {#each data.contradictions as c}
              <div class="rounded-md border border-gray-100 dark:border-gray-700 p-3">
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <Badge color={contradictionColor[c.status] ?? "gray"} class="text-xs">{c.status}</Badge>
                    <span class="text-xs font-mono text-gray-700 dark:text-gray-300">{c.contradiction_id}</span>
                  </div>
                  <span class="text-xs text-gray-400">{formatTs(c.detected_at)}</span>
                </div>
                <p class="text-xs text-gray-600 dark:text-gray-400 mt-1">{c.description}</p>
                {#if c.resolution_note}
                  <p class="text-xs text-green-600 dark:text-green-400 mt-1">Resolution: {c.resolution_note}</p>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </Card>
    {/if}

    <!-- Replay History -->
    {#if activeTab === "replay"}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-4">Replay Manifest History</h3>
        {#if (data.replay_history ?? []).length === 0}
          <p class="text-xs text-gray-400">No replay manifests recorded.</p>
        {:else}
          {#each data.replay_history as manifest}
            <div class="py-2 border-b border-gray-100 dark:border-gray-700 last:border-0">
              <div class="flex items-center justify-between">
                <span class="text-xs font-mono text-gray-700 dark:text-gray-300">{manifest.manifest_id}</span>
                <Badge color={manifest.hash_valid ? "green" : "red"} class="text-xs">
                  {manifest.hash_valid ? "Hash OK" : "Hash FAIL"}
                </Badge>
              </div>
              <p class="text-xs text-gray-400 mt-0.5">
                {formatTs(manifest.created_at)} · {manifest.event_count} events ·
                hash: <span class="font-mono">{(manifest.manifest_hash ?? "").slice(0, 16)}…</span>
              </p>
            </div>
          {/each}
        {/if}
      </Card>
    {/if}

    <!-- Governance Timeline -->
    {#if activeTab === "governance"}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-4">Governance Decision Timeline</h3>
        {#if (data.governance_decisions ?? []).length === 0}
          <p class="text-xs text-gray-400">No governance decisions recorded.</p>
        {:else}
          <div class="space-y-2">
            {#each data.governance_decisions as dec}
              <div class="flex items-start gap-3 py-1.5 border-b border-gray-100 dark:border-gray-700 last:border-0">
                <Badge color={governanceColor[dec.effect] ?? "gray"} class="text-xs shrink-0">{dec.effect}</Badge>
                <div class="flex-1 min-w-0">
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-xs font-mono text-gray-700 dark:text-gray-300 truncate">{dec.policy_id}</span>
                    <span class="text-xs text-gray-400 shrink-0">{formatTs(dec.decided_at)}</span>
                  </div>
                  <p class="text-xs text-gray-500 mt-0.5">{dec.reason}</p>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </Card>
    {/if}

    <!-- Audit Chain -->
    {#if activeTab === "audit"}
      <Card>
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300">Audit Chain</h3>
          <Badge color={data.audit_chain_valid ? "green" : "red"}>
            {data.audit_chain_valid ? "Chain Intact" : "Chain Broken"}
          </Badge>
        </div>
        {#if data.audit_chain_reason}
          <p class="text-xs text-red-600 dark:text-red-400 mb-3">{data.audit_chain_reason}</p>
        {/if}
        <p class="text-xs text-gray-500 mb-3">{data.audit_entry_count ?? 0} entries</p>
        {#if (data.audit_entries ?? []).length === 0}
          <p class="text-xs text-gray-400">No audit entries for this execution.</p>
        {:else}
          <div class="space-y-1 max-h-64 overflow-y-auto">
            {#each data.audit_entries as entry}
              <div class="flex items-center gap-2 py-1 border-b border-gray-50 dark:border-gray-700 last:border-0">
                <span class="text-xs font-mono text-gray-400 w-24 shrink-0">{entry.event_type}</span>
                <span class="text-xs text-gray-600 dark:text-gray-400 truncate flex-1">{entry.outcome}</span>
                <span class="text-xs text-gray-400 shrink-0">{formatTs(entry.occurred_at)}</span>
              </div>
            {/each}
          </div>
        {/if}
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
  {:else}
    <div class="text-center py-16 text-gray-400 text-sm">Enter an execution ID to view its timeline.</div>
  {/if}
</div>
