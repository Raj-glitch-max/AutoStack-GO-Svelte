<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert, Button } from "flowbite-svelte";
  import { RefreshCw } from "lucide-svelte";

  type Deployment = {
    id: string; name: string; image: string; state: string;
    host_port: number; container_port: number;
    container_id?: string; health_url?: string;
    last_error?: string;
    started_at?: string; last_health_check_at?: string;
    restart_count?: number; uptime_seconds?: number;
  };
  type ObsResp = {
    observations: Array<{ kind: string; confidence: number; evidence: string; source: string; trust: string }>;
    lease_holder: string;
    lease_live: boolean;
  };

  let loading = true;
  let error = "";
  let deployments: Deployment[] = [];
  let obs: Record<string, ObsResp> = {};
  let engineHealth: { available: boolean; reason: string } | null = null;
  let pollHandle: ReturnType<typeof setInterval> | null = null;

  async function fetchAll() {
    try {
      [engineHealth, deployments] = await Promise.all([
        client.send("/api/v1/runtime/health", { method: "GET" }),
        client.send("/api/v1/runtime/deployments", { method: "GET" }) as Promise<Deployment[]>,
      ]);
      // Pull observations in parallel — bounded by number of deployments.
      const obsResults = await Promise.allSettled(
        deployments.map((d) =>
          client.send(`/api/v1/runtime/deployments/${d.id}/observations`, { method: "GET" })
        )
      );
      const next: Record<string, ObsResp> = {};
      obsResults.forEach((r, i) => {
        if (r.status === "fulfilled") next[deployments[i].id] = r.value as ObsResp;
      });
      obs = next;
    } catch (e: any) {
      error = e?.message ?? "Failed to load topology";
    } finally {
      loading = false;
    }
  }

  onMount(async () => {
    await fetchAll();
    pollHandle = setInterval(fetchAll, 5000);
  });
  onDestroy(() => { if (pollHandle) clearInterval(pollHandle); });

  function stateColor(s: string): string {
    switch (s) {
      case "certified":
      case "rolled_back": return "green";
      case "stabilized":
      case "verifying":   return "blue";
      case "deploying":
      case "provisioning":
      case "validating":
      case "planned":     return "yellow";
      case "rollback_pending":
      case "rolling_back": return "orange";
      case "contradicted":
      case "failed":      return "red";
      case "destroyed":   return "gray";
      default:            return "gray";
    }
  }

  function formatUptime(s?: number): string {
    if (!s || s <= 0) return "—";
    if (s < 60) return `${s}s`;
    if (s < 3600) return `${Math.floor(s/60)}m ${s%60}s`;
    if (s < 86400) return `${Math.floor(s/3600)}h ${Math.floor((s%3600)/60)}m`;
    return `${Math.floor(s/86400)}d ${Math.floor((s%86400)/3600)}h`;
  }

  function formatTs(ts?: string): string {
    if (!ts) return "—";
    try { return new Date(ts).toLocaleString(); } catch { return ts; }
  }

  function lastContradictionEvidence(d: Deployment): string {
    const o = obs[d.id]?.observations ?? [];
    const c = o.find((x) => x.kind === "container_missing" || x.kind === "container_not_running" || x.kind === "image_mismatch");
    return c?.evidence ?? "";
  }

  // Group by state for the topology view.
  $: byState = (() => {
    const groups: Record<string, Deployment[]> = {};
    for (const d of deployments) {
      (groups[d.state] = groups[d.state] || []).push(d);
    }
    return groups;
  })();

  $: orderedStates = [
    "contradicted", "failed",                                   // attention
    "certified", "stabilized", "verifying",                     // healthy / late lifecycle
    "deploying", "provisioning", "validating", "planned",       // in progress
    "rollback_pending", "rolling_back", "rolled_back",          // rollback
    "destroyed",                                                // terminated
  ].filter((s) => byState[s]);
</script>

<div class="max-w-6xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div class="flex items-start justify-between gap-3">
    <div>
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Runtime Topology</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Live view of all REAL runtime deployments grouped by state. Derived continuously from the
        runtime engine + reconciler. Updates every 5 seconds. Demo-only data is NOT shown here.
      </p>
    </div>
    <Button color="alternative" size="sm" on:click={fetchAll} disabled={loading}>
      <RefreshCw class="w-4 h-4 mr-1.5 {loading ? 'animate-spin' : ''}" /> Refresh
    </Button>
  </div>

  {#if engineHealth && !engineHealth.available}
    <Alert color="red">
      <strong>Runtime engine offline.</strong>
      <p class="mt-1 text-xs">{engineHealth.reason}</p>
      <p class="mt-1 text-xs">Topology will be empty until the engine can reach the local Docker daemon.</p>
    </Alert>
  {/if}

  {#if loading && deployments.length === 0}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if deployments.length === 0}
    <Card padding="lg" class="text-center">
      <p class="text-base font-semibold text-gray-800 dark:text-gray-200">No runtime deployments</p>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Topology shows REAL deployments only. Create one to see it appear here.
      </p>
      <a href="/app/runtime/deployments"
         class="mt-4 inline-block rounded-md bg-primary-600 px-4 py-2 text-sm text-white hover:bg-primary-700">
        Create runtime deployment →
      </a>
    </Card>
  {:else}
    <!-- Summary band -->
    <div class="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-6 gap-3">
      {#each [
        { label: "Total",          count: deployments.length,                                            color: "blue"   },
        { label: "Certified",      count: (byState["certified"]?.length ?? 0),                          color: "green"  },
        { label: "In flight",      count: (deployments.filter((d) => ["planned","validating","provisioning","deploying","verifying","stabilized"].includes(d.state)).length), color: "yellow" },
        { label: "Contradicted",   count: (byState["contradicted"]?.length ?? 0),                       color: "red"    },
        { label: "Rolling back",   count: ((byState["rollback_pending"]?.length ?? 0) + (byState["rolling_back"]?.length ?? 0)), color: "orange" },
        { label: "Failed",         count: (byState["failed"]?.length ?? 0),                             color: "red"    },
      ] as s}
        <Card class="text-center" padding="sm">
          <p class="text-2xl font-bold text-gray-900 dark:text-white">{s.count}</p>
          <p class="mt-0.5 text-xs text-gray-500">{s.label}</p>
        </Card>
      {/each}
    </div>

    <!-- Group by state -->
    {#each orderedStates as state}
      <section class="space-y-2">
        <div class="flex items-center gap-2">
          <Badge color={stateColor(state)} class="text-xs">{state}</Badge>
          <span class="text-xs text-gray-500">{byState[state].length} deployment(s)</span>
        </div>
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {#each byState[state] as d}
            <Card padding="md" class="border-l-4
              {state === 'contradicted' ? 'border-l-red-500' :
               state === 'failed' ? 'border-l-red-700' :
               state === 'certified' ? 'border-l-green-500' :
               state === 'rolled_back' ? 'border-l-green-300' :
               state === 'destroyed' ? 'border-l-gray-400' :
               'border-l-yellow-400'}">
              <div class="flex items-start justify-between gap-2">
                <p class="text-sm font-semibold text-gray-800 dark:text-gray-200 truncate flex-1">{d.name}</p>
                <Badge color="dark" class="text-[10px] font-mono">LOCAL</Badge>
              </div>
              <p class="text-xs text-gray-500 mt-0.5 truncate">
                <code>{d.image}</code>
              </p>
              {#if d.container_id}
                <p class="text-xs text-gray-400 mt-0.5 truncate">
                  container: <code>{d.container_id.slice(0, 12)}</code>
                </p>
              {/if}

              <div class="mt-2 grid grid-cols-2 gap-x-2 gap-y-1 text-xs">
                {#if state === 'certified' && d.uptime_seconds}
                  <div><span class="text-gray-500">Uptime</span>: {formatUptime(d.uptime_seconds)}</div>
                {/if}
                {#if d.restart_count !== undefined && d.restart_count > 0}
                  <div><span class="text-gray-500">Restarts</span>: <span class="text-orange-600">{d.restart_count}</span></div>
                {/if}
                {#if d.health_url}
                  <div class="col-span-2 truncate">
                    <a href={d.health_url} target="_blank" rel="noopener" class="text-primary-600 hover:underline">
                      {d.health_url} ↗
                    </a>
                  </div>
                {/if}
              </div>

              {#if obs[d.id]}
                <div class="mt-2 text-xs">
                  <span class="text-gray-500">Lease:</span>
                  {#if obs[d.id].lease_live}
                    <Badge color="green" class="text-[10px]">live</Badge>
                  {:else}
                    <Badge color="gray" class="text-[10px]">stale</Badge>
                  {/if}
                  <span class="font-mono text-gray-400 text-[10px] truncate inline-block max-w-[150px] align-middle">{obs[d.id].lease_holder?.split('-pid')[1]?.split('-')[0] ?? obs[d.id].lease_holder?.slice(-12)}</span>
                </div>
              {/if}

              {#if state === 'contradicted'}
                {@const evidence = lastContradictionEvidence(d)}
                {#if evidence}
                  <div class="mt-2 rounded bg-red-50 dark:bg-red-900/20 px-2 py-1 text-xs text-red-700 dark:text-red-300">
                    {evidence}
                  </div>
                {/if}
              {/if}

              <div class="mt-2 flex gap-2 text-xs">
                <a href="/app/runtime/deployments" class="text-primary-600 hover:underline">Detail →</a>
              </div>
            </Card>
          {/each}
        </div>
      </section>
    {/each}
  {/if}

  <div class="rounded-md border border-blue-200 bg-blue-50 dark:bg-blue-900/20 p-3">
    <p class="text-xs text-blue-800 dark:text-blue-300">
      <strong>Topology source:</strong> this view aggregates <code>runtime_deployments</code> +
      <code>runtime_observations</code> + lease state.  Every value here is from a real
      <code>docker inspect</code> result or a real lifecycle transition — nothing is synthesized.
      Cloud deployments (when implemented) would appear here with a separate badge; for now only
      <code>LOCAL</code> docker deployments exist.
    </p>
  </div>
</div>
