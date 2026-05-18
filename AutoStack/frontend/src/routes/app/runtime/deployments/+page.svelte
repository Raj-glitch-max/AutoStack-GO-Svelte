<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert, Button, Input, Label, Modal } from "flowbite-svelte";
  import { Plus, RefreshCw, RotateCcw, Trash2, Terminal } from "lucide-svelte";
  import toast from "svelte-french-toast";

  type Deployment = {
    id: string; name: string; image: string; state: string;
    host_port: number; container_port: number;
    container_id?: string; container_name?: string; health_url?: string;
    last_error?: string; previous_image?: string;
    created_at: string; updated_at: string;
  };
  type Event = {
    sequence: number; from_state: string; to_state: string;
    event_type: string; detail: string; error?: string; occurred_at: string;
  };

  let loading = true;
  let error = "";
  let deployments: Deployment[] = [];
  let engineHealth: { available: boolean; reason: string } | null = null;

  let createOpen = false;
  let creating = false;
  let newName = "";
  let newImage = "nginx:alpine";
  let newHostPort = 8765;
  let newContainerPort = 80;

  let detailOpen = false;
  let selected: Deployment | null = null;
  let selectedEvents: Event[] = [];
  let selectedLogs = "";
  let detailLoading = false;

  let pollHandle: ReturnType<typeof setInterval> | null = null;

  async function fetchHealth() {
    try {
      engineHealth = await client.send("/api/v1/runtime/health", { method: "GET" });
    } catch {
      engineHealth = { available: false, reason: "engine probe failed" };
    }
  }

  async function fetchList() {
    try {
      const list = await client.send("/api/v1/runtime/deployments", { method: "GET" });
      deployments = Array.isArray(list) ? list : [];
    } catch (e: any) {
      error = e?.message ?? "Failed to list runtime deployments";
    } finally {
      loading = false;
    }
  }

  async function loadDetail(d: Deployment) {
    selected = d;
    detailOpen = true;
    detailLoading = true;
    selectedEvents = [];
    selectedLogs = "";
    try {
      const [ev, lg] = await Promise.all([
        client.send(`/api/v1/runtime/deployments/${d.id}/events`, { method: "GET" }),
        client.send(`/api/v1/runtime/deployments/${d.id}/logs`, { method: "GET" }),
      ]);
      selectedEvents = Array.isArray(ev) ? ev : [];
      selectedLogs = (lg as any)?.logs ?? "";
    } catch (e: any) {
      toast.error(e?.message ?? "Failed to load deployment detail");
    } finally {
      detailLoading = false;
    }
  }

  async function refreshDetail() {
    if (selected) await loadDetail(selected);
  }

  async function createDeployment() {
    if (!newName.trim() || !newImage.trim()) {
      toast.error("Name and image are required");
      return;
    }
    creating = true;
    try {
      await client.send("/api/v1/runtime/deployments", {
        method: "POST",
        body: JSON.stringify({
          name: newName.trim(),
          image: newImage.trim(),
          host_port: Number(newHostPort) || 0,
          container_port: Number(newContainerPort) || 0,
        }),
      });
      toast.success(`Deployment "${newName}" queued`);
      createOpen = false;
      newName = "";
      await fetchList();
    } catch (e: any) {
      const msg = e?.response?.error ?? e?.message ?? "Create failed";
      toast.error(msg);
    } finally {
      creating = false;
    }
  }

  async function rollback(d: Deployment) {
    if (!d.previous_image) {
      toast.error("No previous_image recorded — cannot roll back. Set one via PocketBase admin first.");
      return;
    }
    try {
      await client.send(`/api/v1/runtime/deployments/${d.id}/rollback`, {
        method: "POST",
        body: JSON.stringify({ target_image: d.previous_image, reason: "operator rollback from UI" }),
      });
      toast.success("Rollback queued");
      await fetchList();
    } catch (e: any) {
      toast.error(e?.response?.error ?? e?.message ?? "Rollback failed");
    }
  }

  async function destroy(d: Deployment) {
    if (!confirm(`Destroy deployment "${d.name}"? The container will be stopped and removed.`)) return;
    try {
      await client.send(`/api/v1/runtime/deployments/${d.id}?reason=operator-destroy`, { method: "DELETE" });
      toast.success("Deployment destroyed");
      detailOpen = false;
      await fetchList();
    } catch (e: any) {
      toast.error(e?.response?.error ?? e?.message ?? "Destroy failed");
    }
  }

  onMount(async () => {
    await fetchHealth();
    await fetchList();
    // Live polling — refresh list + open detail every 3s.
    pollHandle = setInterval(async () => {
      await fetchList();
      if (detailOpen && selected) {
        const updated = deployments.find((d) => d.id === selected!.id);
        if (updated) {
          selected = updated;
          // Refresh events + logs less aggressively
          try {
            const [ev, lg] = await Promise.all([
              client.send(`/api/v1/runtime/deployments/${selected.id}/events`, { method: "GET" }),
              client.send(`/api/v1/runtime/deployments/${selected.id}/logs`, { method: "GET" }),
            ]);
            selectedEvents = Array.isArray(ev) ? ev : selectedEvents;
            selectedLogs = (lg as any)?.logs ?? selectedLogs;
          } catch { /* swallow during polling */ }
        }
      }
    }, 3000);
  });
  onDestroy(() => { if (pollHandle) clearInterval(pollHandle); });

  function stateColor(s: string): string {
    switch (s) {
      case "certified":
      case "rolled_back":
        return "green";
      case "stabilized":
      case "verifying":
        return "blue";
      case "deploying":
      case "provisioning":
      case "validating":
      case "planned":
        return "yellow";
      case "rollback_pending":
      case "rolling_back":
        return "orange";
      case "contradicted":
      case "failed":
        return "red";
      case "destroyed":
        return "gray";
      default:
        return "gray";
    }
  }

  function formatTs(ts: string): string {
    if (!ts) return "—";
    try { return new Date(ts).toLocaleString(); } catch { return ts; }
  }
</script>

<div class="max-w-6xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div class="flex items-start justify-between gap-3">
    <div>
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Runtime Deployments</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Real Docker deployments managed by the local runtime engine. Each lifecycle transition is a
        real subprocess (<code>docker pull</code>, <code>docker run</code>, <code>docker inspect</code>,
        <code>docker stop</code>) and is recorded as an append-only event.
      </p>
    </div>
    <div class="flex gap-2">
      <Button color="alternative" size="sm" on:click={async () => { await fetchHealth(); await fetchList(); }}>
        <RefreshCw class="w-4 h-4 mr-1.5" />
        Refresh
      </Button>
      <Button color="primary" size="sm" on:click={() => (createOpen = true)} disabled={!engineHealth?.available}>
        <Plus class="w-4 h-4 mr-1.5" />
        New Deployment
      </Button>
    </div>
  </div>

  <!-- Engine health banner -->
  {#if engineHealth && !engineHealth.available}
    <Alert color="red">
      <p class="text-sm font-semibold">Runtime engine offline.</p>
      <p class="text-xs mt-1">{engineHealth.reason}</p>
      <p class="text-xs mt-1">
        The engine requires the local Docker daemon to be reachable from the AutoStack process.
        If you're running AutoStack in a container, mount <code>/var/run/docker.sock</code> or
        run AutoStack natively on a host with Docker installed.
      </p>
    </Alert>
  {:else if engineHealth?.available}
    <Alert color="green" class="py-2">
      <p class="text-xs">
        <strong>Runtime engine ready.</strong> Docker daemon reachable. Deployments will execute through
        real container lifecycle.
      </p>
    </Alert>
  {/if}

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if deployments.length === 0}
    <Card padding="lg" class="text-center">
      <p class="text-base font-semibold text-gray-800 dark:text-gray-200">No runtime deployments yet</p>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Create one to see the lifecycle in action: planned → validating → provisioning → deploying →
        verifying → stabilized → certified.
      </p>
      <Button color="primary" size="sm" class="mt-4" on:click={() => (createOpen = true)} disabled={!engineHealth?.available}>
        <Plus class="w-4 h-4 mr-1.5" />
        Create your first deployment
      </Button>
    </Card>
  {:else}
    <div class="space-y-2">
      {#each deployments as d}
        <Card padding="md" class="hover:ring-2 hover:ring-primary-400 transition-shadow cursor-pointer" on:click={() => loadDetail(d)}>
          <div class="flex items-start justify-between gap-3 flex-wrap">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2 flex-wrap">
                <p class="text-sm font-semibold text-gray-800 dark:text-gray-200 truncate">{d.name}</p>
                <Badge color={stateColor(d.state)} class="text-xs">{d.state}</Badge>
                {#if d.container_id}
                  <code class="text-xs text-gray-500">{d.container_id.slice(0, 12)}</code>
                {/if}
              </div>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                image: <code>{d.image}</code>
                {#if d.host_port > 0} &middot; port: <code>{d.host_port}:{d.container_port}</code>{/if}
                &middot; created: {formatTs(d.created_at)}
              </p>
              {#if d.last_error}
                <p class="mt-1 text-xs text-red-600 dark:text-red-400 line-clamp-2">{d.last_error}</p>
              {/if}
              {#if d.health_url && (d.state === "certified" || d.state === "stabilized")}
                <p class="mt-1 text-xs">
                  <a href={d.health_url} target="_blank" rel="noopener" class="text-primary-600 hover:underline">
                    {d.health_url} ↗
                  </a>
                </p>
              {/if}
            </div>
          </div>
        </Card>
      {/each}
    </div>
  {/if}

  <div class="rounded-md border border-blue-200 bg-blue-50 dark:bg-blue-900/20 p-4 space-y-1">
    <p class="text-xs font-semibold uppercase tracking-wide text-blue-700 dark:text-blue-400">
      How this is different from the seeded platform scenarios
    </p>
    <p class="text-xs text-blue-800 dark:text-blue-300">
      The Platform → Timeline / Forensics surfaces show <em>seeded demo executions</em> — useful for
      learning the model. This page shows <em>real Docker containers</em> the engine actually started.
      Lifecycle transitions here are caused by real subprocess outcomes, not scripted scenarios.
    </p>
  </div>
</div>

<Modal bind:open={createOpen} size="md" title="Create runtime deployment" autoclose={false}>
  <div class="space-y-4">
    <Label class="space-y-1.5"><span>Deployment name</span>
      <Input bind:value={newName} placeholder="my-nginx" />
    </Label>
    <Label class="space-y-1.5"><span>Container image</span>
      <Input bind:value={newImage} placeholder="nginx:alpine" />
    </Label>
    <div class="grid grid-cols-2 gap-3">
      <Label class="space-y-1.5"><span>Host port</span>
        <Input type="number" bind:value={newHostPort} min="0" max="65535" />
      </Label>
      <Label class="space-y-1.5"><span>Container port</span>
        <Input type="number" bind:value={newContainerPort} min="0" max="65535" />
      </Label>
    </div>
    <Alert color="yellow" class="py-2">
      <p class="text-xs">
        The engine will run <code>docker pull</code> and <code>docker run -d --name &lt;auto&gt; ...</code>.
        Make sure the host port is not already in use.
      </p>
    </Alert>
  </div>
  <svelte:fragment slot="footer">
    <div class="flex gap-2 justify-end w-full">
      <Button color="alternative" on:click={() => (createOpen = false)} disabled={creating}>Cancel</Button>
      <Button color="primary" on:click={createDeployment} disabled={creating}>
        {creating ? "Creating…" : "Create & deploy"}
      </Button>
    </div>
  </svelte:fragment>
</Modal>

<Modal bind:open={detailOpen} size="xl" title={selected?.name ?? "Deployment"} autoclose={false}>
  {#if selected}
    <div class="space-y-4">
      <div class="flex items-center gap-3 flex-wrap">
        <Badge color={stateColor(selected.state)}>{selected.state}</Badge>
        <code class="text-xs">{selected.id}</code>
        {#if selected.container_id}
          <code class="text-xs text-gray-500">container: {selected.container_id.slice(0, 12)}</code>
        {/if}
        {#if selected.health_url}
          <a href={selected.health_url} target="_blank" rel="noopener" class="text-xs text-primary-600 hover:underline">
            {selected.health_url} ↗
          </a>
        {/if}
      </div>
      {#if selected.last_error}
        <Alert color="red" class="text-xs">{selected.last_error}</Alert>
      {/if}

      <!-- Event log -->
      <div>
        <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400 mb-2">
          Event log ({selectedEvents.length})
        </h3>
        {#if detailLoading}
          <div class="flex justify-center py-4"><Spinner size="6" /></div>
        {:else if selectedEvents.length === 0}
          <p class="text-xs text-gray-500">No events yet.</p>
        {:else}
          <div class="space-y-1 max-h-72 overflow-y-auto rounded border border-gray-200 dark:border-gray-700 p-2">
            {#each selectedEvents as ev}
              <div class="text-xs flex items-baseline gap-2 py-0.5 border-b border-gray-50 dark:border-gray-700 last:border-0">
                <span class="font-mono text-gray-400 w-6 text-right">#{ev.sequence}</span>
                <span class="font-mono text-gray-500 w-32 truncate">{ev.from_state || "—"} → {ev.to_state}</span>
                <span class="font-mono text-primary-600 dark:text-primary-400 w-44 truncate">{ev.event_type}</span>
                <span class="text-gray-600 dark:text-gray-300 flex-1 truncate" title={ev.detail}>{ev.detail}</span>
                <span class="text-gray-400 text-[10px] whitespace-nowrap">{formatTs(ev.occurred_at)}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Logs -->
      <div>
        <div class="flex items-center justify-between mb-2">
          <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400 flex items-center gap-1.5">
            <Terminal class="w-3 h-3" /> Container logs (last 200 lines)
          </h3>
          <Button color="alternative" size="xs" on:click={refreshDetail}>
            <RefreshCw class="w-3 h-3 mr-1" /> Refresh
          </Button>
        </div>
        {#if selectedLogs}
          <pre class="text-xs font-mono p-3 bg-gray-50 dark:bg-gray-800 rounded border border-gray-200 dark:border-gray-700 max-h-48 overflow-auto whitespace-pre-wrap">{selectedLogs}</pre>
        {:else}
          <p class="text-xs text-gray-500">No logs yet.</p>
        {/if}
      </div>
    </div>
  {/if}
  <svelte:fragment slot="footer">
    <div class="flex gap-2 justify-between w-full">
      <div class="flex gap-2">
        {#if selected}
          <Button color="alternative" size="sm" on:click={() => rollback(selected)}>
            <RotateCcw class="w-4 h-4 mr-1.5" /> Rollback
          </Button>
          <Button color="red" size="sm" on:click={() => destroy(selected)}>
            <Trash2 class="w-4 h-4 mr-1.5" /> Destroy
          </Button>
        {/if}
      </div>
      <Button color="alternative" size="sm" on:click={() => (detailOpen = false)}>Close</Button>
    </div>
  </svelte:fragment>
</Modal>
