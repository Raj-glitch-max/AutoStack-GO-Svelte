<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";

  let loading = true;
  let error = "";
  let data: any = null;

  async function fetchProviders() {
    loading = true;
    error = "";
    try {
      data = await client.send("/api/v1/platform/providers", { method: "GET" });
    } catch (e: any) {
      error = e?.message ?? "Failed to load provider capabilities";
    } finally {
      loading = false;
    }
  }

  onMount(fetchProviders);

  const capLabels: Record<string, string> = {
    supports_rollback: "Rollback",
    supports_cancellation: "Cancellation",
    supports_traffic_shift: "Traffic Shift",
    supports_health_check: "Health Check",
    supports_secrets: "Secrets",
    supports_custom_domains: "Custom Domains",
  };
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Provider Capabilities</h1>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        Static capability matrix for supported cloud providers.
      </p>
    </div>
    <button
      on:click={fetchProviders}
      class="rounded-md bg-primary-600 px-3 py-1.5 text-sm text-white hover:bg-primary-700"
    >
      Refresh
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if data}
    <div class="grid gap-6 md:grid-cols-3">
      {#each data.capabilities ?? [] as cap}
        <Card>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white mb-3">{cap.provider}</h3>
          <div class="space-y-1.5">
            {#each Object.entries(capLabels) as [key, label]}
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-600 dark:text-gray-400">{label}</span>
                <Badge color={cap[key] ? "green" : "gray"} class="text-xs">
                  {cap[key] ? "Yes" : "No"}
                </Badge>
              </div>
            {/each}
          </div>
          {#if cap.limitations?.length}
            <div class="mt-3 pt-3 border-t border-gray-100 dark:border-gray-700 space-y-1">
              {#each cap.limitations as lim}
                <p class="text-xs text-yellow-700 dark:text-yellow-400">{lim}</p>
              {/each}
            </div>
          {/if}
        </Card>
      {/each}
    </div>

    {#if data.limitations?.length}
      <div class="rounded-md border border-yellow-200 bg-yellow-50 dark:bg-yellow-900/20 p-4 space-y-1">
        <p class="text-xs font-semibold uppercase tracking-wide text-yellow-700 dark:text-yellow-400">Limitations</p>
        {#each data.limitations as lim}
          <p class="text-xs text-yellow-800 dark:text-yellow-300">{lim}</p>
        {/each}
      </div>
    {/if}
  {/if}
</div>
