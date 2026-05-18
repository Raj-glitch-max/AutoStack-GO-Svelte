<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";

  let loading = true;
  let error = "";
  let accounts: any[] = [];
  let providers: any = null;

  async function fetchAll() {
    loading = true;
    error = "";
    try {
      const [accs, pv] = await Promise.all([
        client.send("/api/v1/cloud-accounts", { method: "GET" }),
        client.send("/api/v1/platform/providers", { method: "GET" }),
      ]);
      accounts = Array.isArray(accs) ? accs : [];
      providers = pv;
    } catch (e: any) {
      error = e?.message ?? "Failed to load cloud overview";
    } finally {
      loading = false;
    }
  }

  onMount(fetchAll);

  function statusColor(s: string): string {
    switch (s) {
      case "active": return "green";
      case "validating": return "yellow";
      case "error": return "red";
      case "revoked": return "gray";
      default: return "gray";
    }
  }
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-8">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Cloud</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Connect AWS, GCP, or Azure accounts and deploy containers to managed cloud runtimes.
      The Kubernetes operator path is unchanged — cloud is an additive deployment surface.
    </p>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else}
    <!-- Status summary -->
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
      <Card class="text-center">
        <p class="text-3xl font-bold text-gray-900 dark:text-white">{accounts.length}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Cloud accounts connected</p>
      </Card>
      <Card class="text-center">
        <p class="text-3xl font-bold text-gray-900 dark:text-white">
          {accounts.filter((a) => a.status === "active").length}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Validated &amp; ready</p>
      </Card>
      <Card class="text-center">
        <p class="text-3xl font-bold text-gray-900 dark:text-white">
          {providers?.capabilities?.length ?? 0}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">Supported providers</p>
      </Card>
    </div>

    <!-- Recently connected accounts -->
    {#if accounts.length > 0}
      <section class="space-y-3">
        <h2 class="text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Connected accounts
        </h2>
        <div class="grid gap-3 sm:grid-cols-2">
          {#each accounts as acc}
            <Card padding="md">
              <div class="flex items-start justify-between gap-2">
                <div class="min-w-0 flex-1">
                  <p class="text-sm font-semibold text-gray-800 dark:text-gray-200">{acc.name}</p>
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {acc.provider} &middot; {acc.region || "no region"}
                  </p>
                  {#if acc.validation_error}
                    <p class="mt-1 text-xs text-red-600 dark:text-red-400 line-clamp-2">
                      {acc.validation_error}
                    </p>
                  {/if}
                </div>
                <Badge color={statusColor(acc.status)} class="text-xs shrink-0">{acc.status}</Badge>
              </div>
              <div class="mt-3 flex gap-2 text-xs">
                <a href="/app/cloud/accounts"
                   class="rounded border border-gray-200 dark:border-gray-700 px-2.5 py-1 text-gray-600 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700">
                  Manage →
                </a>
              </div>
            </Card>
          {/each}
        </div>
      </section>
    {:else}
      <Card padding="lg" class="text-center">
        <p class="text-base font-semibold text-gray-800 dark:text-gray-200">No cloud accounts yet</p>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          Connect a cloud account to deploy containers without managing Kubernetes.
        </p>
        <a href="/app/cloud/accounts"
           class="mt-4 inline-block rounded-md bg-primary-600 px-4 py-2 text-sm text-white hover:bg-primary-700">
          Connect your first account →
        </a>
      </Card>
    {/if}

    <!-- Providers we support -->
    {#if providers?.capabilities?.length}
      <section class="space-y-3">
        <h2 class="text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
          Supported providers
        </h2>
        <div class="grid gap-3 sm:grid-cols-3">
          {#each providers.capabilities as cap}
            <Card padding="md">
              <p class="text-sm font-semibold text-gray-800 dark:text-gray-200">{cap.provider}</p>
              <ul class="mt-2 space-y-0.5 text-xs text-gray-600 dark:text-gray-400">
                <li>rollback: <Badge color={cap.supports_rollback ? "green" : "gray"} class="text-xs">{cap.supports_rollback ? "yes" : "no"}</Badge></li>
                <li>traffic shift: <Badge color={cap.supports_traffic_shift ? "green" : "gray"} class="text-xs">{cap.supports_traffic_shift ? "yes" : "no"}</Badge></li>
                <li>cancellation: <Badge color={cap.supports_cancellation ? "green" : "gray"} class="text-xs">{cap.supports_cancellation ? "yes" : "no"}</Badge></li>
              </ul>
              {#if cap.limitations?.length}
                <ul class="mt-2 space-y-0.5 text-xs text-yellow-700 dark:text-yellow-400">
                  {#each cap.limitations as lim}
                    <li>&bull; {lim}</li>
                  {/each}
                </ul>
              {/if}
            </Card>
          {/each}
        </div>
      </section>
    {/if}

    <!-- What's wired / what's coming -->
    <div class="rounded-md border border-blue-200 bg-blue-50 dark:bg-blue-900/20 p-4 space-y-1">
      <p class="text-xs font-semibold uppercase tracking-wide text-blue-700 dark:text-blue-400">
        What's wired in this release
      </p>
      <p class="text-xs text-blue-800 dark:text-blue-300">
        ✓ Connect cloud accounts (AWS / GCP / Azure) &mdash; credentials encrypted at rest (AES-256-GCM)<br />
        ✓ Live credential validation against the provider's API (STS / IAM / RBAC check)<br />
        ✓ Provider capability discovery (rollback, traffic shift, cancellation, health probes)<br />
        ✓ Deployment target lifecycle (create / observe / reconcile) via the existing reconciler<br />
        ⌛ First-deployment guided wizard &mdash; preview, see "First Deployment" tab<br />
        ⌛ Cost estimate UI &mdash; backend endpoint exists, wire UI in next pass<br />
        ⌛ DNS / domain attachment, registry credentials UI
      </p>
    </div>
  {/if}
</div>
