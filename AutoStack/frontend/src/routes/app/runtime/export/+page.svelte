<script lang="ts">
  import { client } from "$lib/pocketbase";
  import { Input, Button, Spinner, Alert, Card, Badge } from "flowbite-svelte";
  import { Download, ShieldCheck, ShieldAlert } from "lucide-svelte";

  let targetId = "";
  let loading = false;
  let error = "";
  let snapshot: any = null;
  let snapshotJson = "";

  // Verification
  let verifyJson = "";
  let verifying = false;
  let verifyResult: any = null;
  let verifyError = "";

  async function exportSnapshot() {
    if (!targetId.trim()) {
      error = "Target ID is required";
      return;
    }
    loading = true;
    error = "";
    snapshot = null;
    try {
      const res = await client.send(`/api/v1/reconciler/forensic-snapshot/${targetId.trim()}`, { method: "GET" });
      snapshot = res;
      snapshotJson = JSON.stringify(res, null, 2);
    } catch (e: any) {
      error = e?.message ?? "Export failed";
    } finally {
      loading = false;
    }
  }

  function downloadSnapshot() {
    if (!snapshotJson) return;
    const blob = new Blob([snapshotJson], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `autostack-forensic-${targetId}-${new Date().toISOString().slice(0, 19)}.json`;
    a.click();
    URL.revokeObjectURL(url);
  }

  async function verifyHash() {
    verifying = true;
    verifyResult = null;
    verifyError = "";
    let parsed: any;
    try {
      parsed = JSON.parse(verifyJson);
    } catch (e) {
      verifyError = "Invalid JSON — paste the full snapshot JSON";
      verifying = false;
      return;
    }
    try {
      const res = await client.send("/api/v1/reconciler/verify-export-hash", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(parsed),
      });
      verifyResult = res;
    } catch (e: any) {
      verifyError = e?.message ?? "Verification failed";
    } finally {
      verifying = false;
    }
  }
</script>

<div class="max-w-3xl mx-auto px-4 pt-16 pb-8 space-y-8">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Forensic Snapshot Export</h1>
    <p class="text-sm text-gray-500 dark:text-gray-400 mt-2">
      Exports a deterministic JSON snapshot of all persisted state for a target.
      The snapshot includes a SHA-256 integrity checksum and schema version.
      Two exports taken at the same second against identical DB state produce byte-identical JSON.
    </p>
  </div>

  <!-- Export -->
  <Card>
    <h3 class="font-semibold text-gray-800 dark:text-white mb-3">Generate Snapshot</h3>
    <div class="flex gap-3">
      <Input size="sm" placeholder="Deployment target ID" bind:value={targetId} class="flex-1" />
      <Button size="sm" color="primary" on:click={exportSnapshot} disabled={loading}>
        {#if loading}<Spinner size="4" class="mr-2" />{/if}
        Export
      </Button>
    </div>

    {#if error}
      <Alert color="red" class="mt-3">{error}</Alert>
    {/if}

    {#if snapshot}
      <div class="mt-4 space-y-3">
        <div class="grid grid-cols-2 gap-3 text-sm">
          <div>
            <span class="text-gray-500">Schema version:</span>
            <Badge color="blue">{snapshot.schema_version}</Badge>
          </div>
          <div>
            <span class="text-gray-500">Snapshot at:</span>
            <span class="font-mono text-xs">{snapshot.snapshot_at}</span>
          </div>
          <div>
            <span class="text-gray-500">Metric truthfulness:</span>
            <Badge color={snapshot.metric_truthfulness === 'hydrated_from_db' ? 'green' : 'yellow'}>
              {snapshot.metric_truthfulness}
            </Badge>
          </div>
          <div>
            <span class="text-gray-500">Export hash:</span>
            <span class="font-mono text-xs break-all">{snapshot.export_hash?.slice(0, 16)}…</span>
          </div>
        </div>

        <div class="text-xs text-gray-500 space-y-1">
          <div>Operations: {snapshot.operations?.length ?? 0}</div>
          <div>Incidents: {snapshot.incidents?.length ?? 0}</div>
          <div>Provider observations: {snapshot.provider_observations?.length ?? 0}</div>
          <div>Provider drifts: {snapshot.provider_drifts?.length ?? 0}</div>
          <div>Invariant violations: {snapshot.invariant_audit?.length ?? 0}</div>
        </div>

        {#if snapshot.truthfulness_note}
          <Alert color="yellow" class="text-xs">{snapshot.truthfulness_note}</Alert>
        {/if}

        <Button size="sm" color="alternative" on:click={downloadSnapshot}>
          <Download class="w-4 h-4 mr-2" /> Download JSON
        </Button>
      </div>
    {/if}
  </Card>

  <!-- Verification -->
  <Card>
    <h3 class="font-semibold text-gray-800 dark:text-white mb-3">Verify Snapshot Integrity</h3>
    <p class="text-xs text-gray-400 mb-3">
      Paste a previously exported snapshot JSON to verify its SHA-256 checksum.
      Any modification to the snapshot (including whitespace changes in the export_hash field) will produce a mismatch.
    </p>
    <textarea
      bind:value={verifyJson}
      rows={8}
      placeholder="Paste full snapshot JSON here…"
      class="w-full font-mono text-xs border border-gray-300 dark:border-gray-600 rounded p-2 bg-gray-50 dark:bg-gray-800 dark:text-gray-200 resize-y"
    ></textarea>
    <Button size="sm" color="primary" class="mt-3" on:click={verifyHash} disabled={verifying || !verifyJson.trim()}>
      {#if verifying}<Spinner size="4" class="mr-2" />{/if}
      Verify Integrity
    </Button>

    {#if verifyError}
      <Alert color="red" class="mt-3">{verifyError}</Alert>
    {/if}

    {#if verifyResult}
      <div class="mt-4 rounded-lg border p-4
        {verifyResult.intact ? 'bg-green-50 border-green-200 dark:bg-green-900/20' : 'bg-red-50 border-red-200 dark:bg-red-900/20'}">
        <div class="flex items-center gap-2 font-semibold">
          {#if verifyResult.intact}
            <ShieldCheck class="w-5 h-5 text-green-600" />
            <span class="text-green-800 dark:text-green-300">Snapshot is intact — hash matches</span>
          {:else}
            <ShieldAlert class="w-5 h-5 text-red-600" />
            <span class="text-red-800 dark:text-red-300">Hash mismatch — snapshot may have been modified</span>
          {/if}
        </div>
        <div class="mt-2 space-y-1 text-xs font-mono text-gray-600 dark:text-gray-400">
          <div>Stored: {verifyResult.stored_hash}</div>
          <div>Recomputed: {verifyResult.recomputed_hash}</div>
          <div>Schema: {verifyResult.schema_version} · Taken at: {verifyResult.snapshot_at}</div>
          <div>Verified at: {verifyResult.verified_at}</div>
        </div>
      </div>
    {/if}
  </Card>
</div>
