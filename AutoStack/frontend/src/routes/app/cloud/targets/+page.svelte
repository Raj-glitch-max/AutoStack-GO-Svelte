<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert, Button } from "flowbite-svelte";
  import { RefreshCw } from "lucide-svelte";

  let loading = true;
  let error = "";
  let targets: any[] = [];

  async function fetchTargets() {
    loading = true;
    error = "";
    try {
      // deployment_targets is a regular PocketBase collection — list directly.
      const res = await client.collection("deployment_targets")
        .getList(1, 100, { sort: "-created" });
      targets = res?.items ?? [];
    } catch (e: any) {
      error = e?.message ?? "Failed to load deployment targets";
    } finally {
      loading = false;
    }
  }

  onMount(fetchTargets);

  function statusColor(s: string): string {
    switch ((s || "").toLowerCase()) {
      case "running": case "active": case "ok": return "green";
      case "pending": case "validating": case "creating": return "yellow";
      case "failed": case "error": return "red";
      case "retry_pending": return "orange";
      default: return "gray";
    }
  }

  function formatTs(ts: string): string {
    if (!ts) return "—";
    try { return new Date(ts).toLocaleString(); } catch { return ts; }
  }
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div class="flex items-start justify-between gap-3">
    <div>
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Deployment Targets</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Reconciled cloud deployment state. Each target is owned by the cloud reconciler and
        reflects the last observed cloud-side reality, not just the desired spec.
      </p>
    </div>
    <Button color="alternative" size="sm" on:click={fetchTargets} disabled={loading}>
      <RefreshCw class="w-4 h-4 mr-1.5 {loading ? 'animate-spin' : ''}" />
      Refresh
    </Button>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if targets.length === 0}
    <Card padding="lg" class="text-center">
      <p class="text-base font-semibold text-gray-800 dark:text-gray-200">No deployment targets yet</p>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Targets appear here after you trigger a cloud deployment from a project.
      </p>
      <a href="/app/cloud/deploy"
         class="mt-4 inline-block rounded-md bg-primary-600 px-4 py-2 text-sm text-white hover:bg-primary-700">
        Start a deployment →
      </a>
    </Card>
  {:else}
    <div class="space-y-2">
      {#each targets as t}
        <Card padding="md">
          <div class="flex items-start justify-between gap-3 flex-wrap">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2 flex-wrap">
                <p class="text-sm font-semibold text-gray-800 dark:text-gray-200 truncate">
                  {t.name || t.id}
                </p>
                <Badge color={statusColor(t.status)} class="text-xs">{t.status || "unknown"}</Badge>
                {#if t.provider}
                  <Badge color="blue" class="text-xs">{t.provider}</Badge>
                {/if}
                {#if t.lifecycle_ambiguous}
                  <Badge color="orange" class="text-xs">ambiguous</Badge>
                {/if}
              </div>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                created: {formatTs(t.created)}
                {#if t.next_retry_at} &middot; next retry: {formatTs(t.next_retry_at)}{/if}
                {#if t.current_operation} &middot; op: <code class="font-mono">{t.current_operation}</code>{/if}
              </p>
              {#if t.last_error}
                <p class="mt-1 text-xs text-red-600 dark:text-red-400">{t.last_error}</p>
              {/if}
            </div>
          </div>
        </Card>
      {/each}
    </div>
  {/if}

  <div class="rounded-md border border-blue-200 bg-blue-50 dark:bg-blue-900/20 p-4 space-y-1">
    <p class="text-xs font-semibold uppercase tracking-wide text-blue-700 dark:text-blue-400">
      How reconciliation works
    </p>
    <p class="text-xs text-blue-800 dark:text-blue-300">
      The cloud reconciler runs continuously in the background. It owns one operation per target
      via a lease, observes provider-side state, persists drift evidence, and surfaces
      contradictions to operators. Reconciliation does not auto-rollback; operators decide.
    </p>
  </div>
</div>
