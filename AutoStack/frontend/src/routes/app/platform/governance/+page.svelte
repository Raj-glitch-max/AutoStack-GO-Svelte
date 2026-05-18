<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";

  import { page } from "$app/stores";

  let loading = true;
  let error = "";
  let data: any = null;
  let executionID = $page.url.searchParams.get("id") ?? "";

  async function fetchGovernance() {
    if (!executionID.trim()) {
      error = "Enter an execution ID";
      return;
    }
    loading = true;
    error = "";
    try {
      data = await client.send(`/api/v1/platform/governance/${executionID.trim()}`, { method: "GET" });
    } catch (e: any) {
      error = e?.message ?? "Failed to load governance data";
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (executionID) {
      fetchGovernance();
    } else {
      loading = false;
    }
  });

  const effectColor: Record<string, string> = {
    allow: "green",
    deny: "red",
    "require-approval": "yellow",
    "require-multi-approval": "orange",
  };
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Governance Control Plane</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Policy decisions, RBAC audit, approval hierarchies, and override history.
    </p>
  </div>

  <div class="flex gap-3">
    <input
      bind:value={executionID}
      placeholder="Execution ID"
      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
    />
    <button
      on:click={fetchGovernance}
      class="rounded-md bg-primary-600 px-4 py-2 text-sm text-white hover:bg-primary-700"
    >
      Load
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if data}
    <!-- Audit chain integrity -->
    <Card>
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300">Audit Chain</h3>
        <Badge color={data.chain_valid ? "green" : "red"}>
          {data.chain_valid ? "Intact" : "Tampered"}
        </Badge>
      </div>
      {#if !data.chain_valid && data.chain_reason}
        <p class="mt-2 text-xs text-red-600 dark:text-red-400">{data.chain_reason}</p>
      {/if}
      <p class="mt-2 text-xs text-gray-500">
        {data.snapshot?.audit_chain?.entries?.length ?? 0} entries ·
        Chain hash: <span class="font-mono">{(data.snapshot?.audit_chain?.chain_hash ?? "").slice(0, 16)}…</span>
      </p>
    </Card>

    <div class="grid gap-6 md:grid-cols-2">
      <!-- Policy decisions -->
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Policy Decisions</h3>
        {#each data.snapshot?.decisions ?? [] as dec}
          <div class="py-1.5 border-b border-gray-100 dark:border-gray-700 last:border-0">
            <div class="flex items-center gap-2">
              <Badge color={effectColor[dec.effect] ?? "gray"} class="text-xs">{dec.effect}</Badge>
              <span class="text-xs font-mono text-gray-700 dark:text-gray-300">{dec.policy_id}</span>
            </div>
            <p class="text-xs text-gray-500 mt-0.5">{dec.reason}</p>
          </div>
        {:else}
          <p class="text-xs text-gray-400">No policy decisions recorded.</p>
        {/each}
      </Card>

      <!-- Overrides -->
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Overrides</h3>
        {#each data.snapshot?.overrides ?? [] as ov}
          <div class="py-1.5 border-b border-gray-100 dark:border-gray-700 last:border-0">
            <div class="flex items-center gap-2">
              <Badge color="purple" class="text-xs">override</Badge>
              <span class="text-xs font-mono">{ov.actor_id}</span>
            </div>
            <p class="text-xs text-gray-500 mt-0.5">{ov.reason}</p>
            <p class="text-xs text-gray-400">{ov.overridden_at}</p>
          </div>
        {:else}
          <p class="text-xs text-gray-400">No overrides recorded.</p>
        {/each}
      </Card>
    </div>

    <!-- Approval hierarchies -->
    {#if (data.snapshot?.hierarchies ?? []).length > 0}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Approval Hierarchies</h3>
        {#each data.snapshot.hierarchies as hier}
          <div class="py-2 border-b border-gray-100 dark:border-gray-700 last:border-0">
            <div class="flex items-center justify-between">
              <span class="text-xs font-mono text-gray-700 dark:text-gray-300">{hier.hierarchy_id}</span>
              <Badge color={hier.all_satisfied ? "green" : "yellow"}>
                {hier.all_satisfied ? "Satisfied" : "Pending"}
              </Badge>
            </div>
            <p class="text-xs text-gray-500 mt-0.5">{hier.rationale}</p>
            <div class="mt-1 flex flex-wrap gap-1">
              {#each hier.steps as step}
                <Badge color={step.satisfied ? "green" : "gray"} class="text-xs">
                  Step {step.step_order}: {step.satisfied ? step.satisfied_by : "pending"}
                </Badge>
              {/each}
            </div>
          </div>
        {/each}
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
  {/if}
</div>
