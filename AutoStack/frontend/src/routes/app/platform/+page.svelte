<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";

  let loading = true;
  let error = "";
  let overview: any = null;
  let executions: any[] = [];
  let providers: any = null;
  let sourceFilter: "all" | "live" | "demo" = "all";

  async function fetchAll() {
    loading = true;
    error = "";
    try {
      const [ov, ex, pv] = await Promise.all([
        client.send("/api/v1/platform/overview", { method: "GET" }),
        client.send("/api/v1/platform/executions", { method: "GET" }),
        client.send("/api/v1/platform/providers", { method: "GET" }),
      ]);
      overview = ov;
      executions = ex ?? [];
      providers = pv;
    } catch (e: any) {
      error = e?.message ?? "Failed to load platform overview";
    } finally {
      loading = false;
    }
  }

  onMount(fetchAll);

  function stateColor(state: string): string {
    switch (state) {
      case "verified": return "green";
      case "contradicted": return "red";
      case "pending": return "yellow";
      case "stale": return "orange";
      default: return "gray";
    }
  }

  function confidenceColor(c: number): string {
    if (c >= 0.9) return "green";
    if (c >= 0.7) return "yellow";
    if (c > 0) return "orange";
    return "gray";
  }

  function formatPct(v: number): string {
    return (v * 100).toFixed(0) + "%";
  }
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-8">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Platform Overview</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Aggregate health across all active executions.
      </p>
    </div>
    <button
      on:click={fetchAll}
      class="rounded-md bg-primary-600 px-3 py-1.5 text-sm text-white hover:bg-primary-700"
    >
      Refresh
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if overview}
    <!-- top-line metric cards -->
    <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
      {#each [
        { label: "Active Executions", value: overview.active_executions, color: "blue" },
        { label: "Verified", value: overview.verified_count, color: "green" },
        { label: "Contradicted", value: overview.contradicted_count, color: "red" },
        { label: "Pending Approvals", value: overview.pending_approvals, color: "yellow" },
        { label: "Overrides", value: overview.override_count, color: "purple" },
      ] as stat}
        <Card class="text-center">
          <p class="text-3xl font-bold text-gray-900 dark:text-white">{stat.value}</p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{stat.label}</p>
        </Card>
      {/each}
    </div>

    <!-- execution cards -->
    {#if executions.length > 0}
      <section class="space-y-3">
        <div class="flex items-baseline justify-between gap-3 flex-wrap">
          <h2 class="text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            Executions ({executions.filter((e) => sourceFilter === 'all' || e.source === sourceFilter).length}{sourceFilter === 'all' ? '' : ` of ${executions.length}`})
          </h2>
          <div class="flex gap-1">
            {#each [
              { v: 'all',  label: 'All' },
              { v: 'live', label: 'Live only' },
              { v: 'demo', label: 'Demo only' },
            ] as opt}
              <button
                on:click={() => (sourceFilter = opt.v)}
                class="px-2.5 py-0.5 text-xs rounded {sourceFilter === opt.v
                  ? 'bg-primary-600 text-white'
                  : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600'}"
              >{opt.label}</button>
            {/each}
          </div>
        </div>
        {#if executions.some((e) => e.source === 'demo')}
          <div class="rounded-md border border-orange-200 bg-orange-50 dark:bg-orange-900/20 px-3 py-2 text-xs text-orange-800 dark:text-orange-300">
            <strong>DEMO data present.</strong>
            Seeded scenarios are tagged with a yellow <span class="inline-block px-1.5 py-0.5 rounded bg-yellow-100 text-yellow-800 text-[10px] font-mono">DEMO</span> badge and are NOT produced by a live execution.
            Use the filter to show only live executions.
          </div>
        {/if}
        <div class="grid gap-3 sm:grid-cols-2">
          {#each executions.filter((e) => sourceFilter === 'all' || e.source === sourceFilter) as ex}
            <Card padding="md" class="hover:ring-2 hover:ring-primary-400 transition-shadow {ex.source === 'demo' ? 'opacity-95' : ''}">
              <div class="flex items-start justify-between gap-2">
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="text-sm font-mono text-gray-800 dark:text-gray-200 truncate">
                      {ex.execution_id}
                    </span>
                    {#if ex.source === 'demo'}
                      <Badge color="yellow" class="text-[10px] font-mono">DEMO</Badge>
                    {:else}
                      <Badge color="green" class="text-[10px] font-mono">LIVE</Badge>
                    {/if}
                    <Badge color={stateColor(ex.verification_state)} class="text-xs">
                      {ex.verification_state}
                    </Badge>
                    {#if ex.has_contradictions}
                      <Badge color="red" class="text-xs">contradicted</Badge>
                    {/if}
                    {#if ex.has_overrides}
                      <Badge color="purple" class="text-xs">override</Badge>
                    {/if}
                  </div>
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                    Stages: <span class="font-semibold text-gray-700 dark:text-gray-300">{ex.completed_stages}/{ex.total_stages}</span>
                    {#if ex.confidence_score > 0}
                      &middot; Confidence:
                      <Badge color={confidenceColor(ex.confidence_score)} class="text-xs">
                        {formatPct(ex.confidence_score)}
                      </Badge>
                    {/if}
                    &middot; Audit entries: {ex.audit_entry_count}
                  </p>
                </div>
              </div>
              <div class="mt-3 flex flex-wrap gap-2 text-xs">
                <a href="/app/platform/timeline?id={ex.execution_id}"
                   class="rounded border border-gray-200 dark:border-gray-700 px-2.5 py-1 text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700">
                  Timeline →
                </a>
                <a href="/app/platform/forensics?id={ex.execution_id}"
                   class="rounded border border-gray-200 dark:border-gray-700 px-2.5 py-1 text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700">
                  Forensics →
                </a>
                <a href="/app/platform/governance?id={ex.execution_id}"
                   class="rounded border border-gray-200 dark:border-gray-700 px-2.5 py-1 text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700">
                  Governance →
                </a>
                <a href="/app/platform/replay?id={ex.execution_id}"
                   class="rounded border border-gray-200 dark:border-gray-700 px-2.5 py-1 text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700">
                  Replay →
                </a>
              </div>
            </Card>
          {/each}
        </div>
      </section>
    {/if}

    <!-- providers panel -->
    {#if providers?.capabilities?.length}
      <section class="space-y-3">
        <h2 class="text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Provider Capabilities
        </h2>
        <div class="grid gap-3 sm:grid-cols-3">
          {#each providers.capabilities as cap}
            <Card padding="md">
              <p class="text-sm font-semibold text-gray-800 dark:text-gray-200">{cap.provider}</p>
              <ul class="mt-2 space-y-0.5 text-xs text-gray-600 dark:text-gray-400">
                <li>rollback: <Badge color={cap.supports_rollback ? "green" : "gray"} class="text-xs">{cap.supports_rollback ? "yes" : "no"}</Badge></li>
                <li>traffic shift: <Badge color={cap.supports_traffic_shift ? "green" : "gray"} class="text-xs">{cap.supports_traffic_shift ? "yes" : "no"}</Badge></li>
                <li>health probe: <Badge color={cap.supports_health_probe ? "green" : "gray"} class="text-xs">{cap.supports_health_probe ? "yes" : "no"}</Badge></li>
              </ul>
            </Card>
          {/each}
        </div>
      </section>
    {/if}

    {#if overview.limitations?.length}
      <div class="rounded-md border border-yellow-200 bg-yellow-50 dark:bg-yellow-900/20 p-4 space-y-1">
        <p class="text-xs font-semibold uppercase tracking-wide text-yellow-700 dark:text-yellow-400">Limitations</p>
        {#each overview.limitations as lim}
          <p class="text-xs text-yellow-800 dark:text-yellow-300">{lim}</p>
        {/each}
      </div>
    {/if}

    <p class="text-xs text-gray-400">Generated at: {overview.generated_at}</p>
  {/if}
</div>
