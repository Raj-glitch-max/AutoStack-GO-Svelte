<script lang="ts">
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";
  import { client } from "$lib/pocketbase";

  let loading = false;
  let error = "";
  let result: any = null;
  let rawInput = "";

  async function verifyExports() {
    let body: any;
    try {
      body = JSON.parse(rawInput);
    } catch {
      error = "Invalid JSON — paste a valid export payload";
      return;
    }

    loading = true;
    error = "";
    result = null;
    try {
      result = await client.send("/api/v1/platform/exports/verify", {
        method: "POST",
        body: JSON.stringify(body),
        headers: { "Content-Type": "application/json" },
      });
    } catch (e: any) {
      error = e?.message ?? "Verification request failed";
    } finally {
      loading = false;
    }
  }
</script>

<div class="max-w-4xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Export Verification</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Verify tamper-evidence hashes across verification, governance, topology, and scheduler snapshots.
    </p>
  </div>

  <Card>
    <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-2">Export Payload (JSON)</h3>
    <textarea
      bind:value={rawInput}
      rows="12"
      placeholder={`{
  "verification_snapshot": { ... },
  "governance_snapshot": { ... },
  "topology_snapshot": { ... },
  "scheduler_snapshot": { ... }
}`}
      class="w-full rounded-md border border-gray-300 px-3 py-2 text-xs font-mono dark:border-gray-600 dark:bg-gray-800 dark:text-white"
    />
    <button
      on:click={verifyExports}
      disabled={loading || !rawInput.trim()}
      class="mt-3 rounded-md bg-primary-600 px-4 py-2 text-sm text-white hover:bg-primary-700 disabled:opacity-50"
    >
      {loading ? "Verifying…" : "Verify"}
    </button>
  </Card>

  {#if loading}
    <div class="flex justify-center py-8"><Spinner size="6" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if result}
    <div class="grid gap-4 sm:grid-cols-2">
      {#each [
        { label: "Verification Snapshot", key: "verification_valid" },
        { label: "Governance Snapshot", key: "governance_valid" },
        { label: "Topology Snapshot", key: "topology_valid" },
        { label: "Scheduler Snapshot", key: "scheduler_valid" },
      ] as check}
        <Card class="flex items-center justify-between">
          <span class="text-sm text-gray-700 dark:text-gray-300">{check.label}</span>
          <Badge color={result[check.key] ? "green" : "red"}>
            {result[check.key] ? "Valid" : "Invalid"}
          </Badge>
        </Card>
      {/each}
    </div>

    {#if result.reason}
      <Alert color="red">
        <strong>Verification failure:</strong> {result.reason}
      </Alert>
    {/if}
  {/if}
</div>
