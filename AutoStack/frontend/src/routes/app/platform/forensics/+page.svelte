<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert, Select } from "flowbite-svelte";

  let loading = true;
  let error = "";
  let data: any = null;
  let targetID = "";
  let executionList: Array<{ value: string; name: string }> = [];
  type ForensicTab = "snapshot" | "replay" | "cert" | "contradictions" | "backup";
  let activeTab: ForensicTab = "snapshot";
  function setTab(t: string) { activeTab = t as ForensicTab; }

  async function fetchExecutionList() {
    try {
      const list: any = await client.send("/api/v1/platform/executions", { method: "GET" });
      executionList = (list ?? []).map((e: any) => ({
        value: e.execution_id,
        name: `${e.execution_id}  (${e.verification_state})`,
      }));
    } catch (e: any) {
      // Empty dropdown is fine.
    }
  }

  async function fetchForensics() {
    if (!targetID.trim()) {
      error = "Select or enter a target ID";
      return;
    }
    loading = true;
    error = "";
    try {
      data = await client.send(`/api/v1/platform/forensics/${targetID.trim()}`, { method: "GET" });
    } catch (e: any) {
      error = e?.message ?? "Failed to load forensic data";
    } finally {
      loading = false;
    }
  }

  onMount(async () => {
    await fetchExecutionList();
    const urlParam = new URL(window.location.href).searchParams.get("id");
    if (urlParam) {
      targetID = urlParam;
      await fetchForensics();
      return;
    }
    // Auto-load the contradicted scenario first — it has the richest demo data.
    const preferred = executionList.find((e) => e.value.includes("contradicted"))?.value;
    if (preferred) {
      targetID = preferred;
      await fetchForensics();
    } else if (executionList.length > 0) {
      targetID = executionList[0].value;
      await fetchForensics();
    } else {
      loading = false;
    }
  });

  function onSelect() {
    if (targetID) fetchForensics();
  }

  function formatTs(ts: string): string {
    if (!ts) return "—";
    try { return new Date(ts).toLocaleString(); } catch { return ts; }
  }

  function truncate(s: string, n = 20): string {
    return s && s.length > n ? s.slice(0, n) + "…" : (s ?? "—");
  }
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Forensic Investigation</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Point-in-time forensic snapshot, replay manifest inspection, certification evidence, contradiction evidence,
      and backup verification. Read-only — no state is mutated from this surface.
    </p>
  </div>

  <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
    {#if executionList.length > 0}
      <Select
        class="flex-1"
        bind:value={targetID}
        items={executionList}
        on:change={onSelect}
        size="sm"
      />
    {/if}
    <input
      bind:value={targetID}
      on:keydown={(e) => e.key === "Enter" && fetchForensics()}
      placeholder="…or enter a target ID manually"
      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
    />
    <button
      on:click={fetchForensics}
      class="rounded-md bg-primary-600 px-4 py-2 text-sm text-white hover:bg-primary-700 whitespace-nowrap"
    >
      Investigate
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-16"><Spinner size="8" /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if data}
    <!-- Tab bar -->
    <div class="border-b border-gray-200 dark:border-gray-700">
      <nav class="-mb-px flex gap-4 flex-wrap">
        {#each [
          { key: "snapshot", label: "Snapshot" },
          { key: "replay", label: "Replay Manifest" },
          { key: "cert", label: "Certification" },
          { key: "contradictions", label: "Contradictions" },
          { key: "backup", label: "Backup" },
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

    <!-- Forensic Snapshot -->
    {#if activeTab === "snapshot"}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Forensic Snapshot</h3>
        {#if data.snapshot}
          <div class="space-y-2">
            <div class="grid grid-cols-2 gap-3 text-xs">
              <div>
                <span class="text-gray-500">Target ID</span>
                <p class="font-mono text-gray-800 dark:text-gray-200">{data.snapshot.target_id}</p>
              </div>
              <div>
                <span class="text-gray-500">Captured At</span>
                <p class="text-gray-800 dark:text-gray-200">{formatTs(data.snapshot.captured_at)}</p>
              </div>
              <div>
                <span class="text-gray-500">Snapshot Hash</span>
                <p class="font-mono text-gray-800 dark:text-gray-200">{truncate(data.snapshot.snapshot_hash, 32)}</p>
              </div>
              <div>
                <span class="text-gray-500">Schema Version</span>
                <p class="font-mono text-gray-800 dark:text-gray-200">{data.snapshot.schema_version ?? "—"}</p>
              </div>
            </div>
            {#if data.snapshot.invariant_violations?.length}
              <div class="rounded border border-red-200 bg-red-50 dark:bg-red-900/20 p-3">
                <p class="text-xs font-semibold text-red-700 dark:text-red-400 mb-1">Invariant Violations</p>
                {#each data.snapshot.invariant_violations as v}
                  <p class="text-xs text-red-600 dark:text-red-300">{v}</p>
                {/each}
              </div>
            {:else}
              <p class="text-xs text-green-600 dark:text-green-400">No invariant violations detected.</p>
            {/if}
          </div>
        {:else}
          <p class="text-xs text-gray-400">No snapshot available for this target.</p>
        {/if}
      </Card>
    {/if}

    <!-- Replay Manifest Inspector -->
    {#if activeTab === "replay"}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Replay Manifest</h3>
        {#if data.replay_manifest}
          <div class="space-y-2 text-xs">
            <div class="grid grid-cols-2 gap-3">
              <div>
                <span class="text-gray-500">Manifest ID</span>
                <p class="font-mono text-gray-800 dark:text-gray-200">{data.replay_manifest.manifest_id}</p>
              </div>
              <div>
                <span class="text-gray-500">Hash Verification</span>
                <Badge color={data.replay_manifest.hash_valid ? "green" : "red"} class="text-xs">
                  {data.replay_manifest.hash_valid ? "PASS" : "FAIL"}
                </Badge>
                {#if data.replay_manifest.hash_reason}
                  <p class="text-red-500 mt-0.5">{data.replay_manifest.hash_reason}</p>
                {/if}
              </div>
              <div>
                <span class="text-gray-500">Event Count</span>
                <p class="text-gray-800 dark:text-gray-200">{data.replay_manifest.event_count}</p>
              </div>
              <div>
                <span class="text-gray-500">Created At</span>
                <p class="text-gray-800 dark:text-gray-200">{formatTs(data.replay_manifest.created_at)}</p>
              </div>
            </div>
            <div>
              <span class="text-gray-500">Manifest Hash</span>
              <p class="font-mono text-gray-800 dark:text-gray-200 break-all">{data.replay_manifest.manifest_hash}</p>
            </div>
            <div>
              <span class="text-gray-500">Content Hash</span>
              <p class="font-mono text-gray-800 dark:text-gray-200 break-all">{data.replay_manifest.content_hash}</p>
            </div>
          </div>
        {:else}
          <p class="text-xs text-gray-400">No replay manifest available for this target.</p>
        {/if}
      </Card>
    {/if}

    <!-- Certification Evidence -->
    {#if activeTab === "cert"}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Operational Certification</h3>
        {#if data.certification}
          <div class="space-y-3">
            <div class="flex items-center gap-3">
              <Badge color={data.certification.certified ? "green" : "red"} class="text-sm">
                {data.certification.certified ? "CERTIFIED" : "NOT CERTIFIED"}
              </Badge>
              <span class="text-xs text-gray-500">
                {data.certification.gates_passed}/{data.certification.gates_total} gates passed
              </span>
            </div>
            <div class="grid grid-cols-1 gap-1">
              {#each data.certification.gates ?? [] as gate}
                <div class="flex items-center gap-2 py-1 border-b border-gray-50 dark:border-gray-700 last:border-0">
                  <Badge color={gate.passed ? "green" : "red"} class="text-xs w-12 text-center">
                    {gate.passed ? "PASS" : "FAIL"}
                  </Badge>
                  <span class="text-xs font-mono text-gray-700 dark:text-gray-300 w-40">{gate.name}</span>
                  <span class="text-xs text-gray-500 truncate">{gate.detail}</span>
                </div>
              {/each}
            </div>
            {#if data.certification.cert_hash}
              <div class="text-xs">
                <span class="text-gray-500">Cert Hash: </span>
                <span class="font-mono text-gray-700 dark:text-gray-300">{truncate(data.certification.cert_hash, 40)}</span>
              </div>
            {/if}
          </div>
        {:else}
          <p class="text-xs text-gray-400">No certification record for this target.</p>
        {/if}
      </Card>
    {/if}

    <!-- Contradiction Evidence -->
    {#if activeTab === "contradictions"}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Contradiction Evidence</h3>
        {#if (data.contradiction_evidence ?? []).length === 0}
          <p class="text-xs text-gray-400">No contradiction evidence recorded for this target.</p>
        {:else}
          <div class="space-y-3">
            {#each data.contradiction_evidence as ev}
              <div class="rounded border border-orange-200 bg-orange-50 dark:bg-orange-900/20 p-3">
                <div class="flex items-center justify-between mb-1">
                  <span class="text-xs font-mono font-semibold text-orange-800 dark:text-orange-300">
                    {ev.contradiction_id}
                  </span>
                  <Badge color={ev.status === "resolved" ? "green" : "red"} class="text-xs">
                    {ev.status}
                  </Badge>
                </div>
                <p class="text-xs text-orange-700 dark:text-orange-400">{ev.description}</p>
                <p class="text-xs text-gray-500 mt-1">
                  Detected: {formatTs(ev.detected_at)}
                  {#if ev.resolved_at} · Resolved: {formatTs(ev.resolved_at)}{/if}
                </p>
                {#if ev.evidence_fields}
                  <div class="mt-2 text-xs font-mono bg-white dark:bg-gray-800 rounded p-2 border border-orange-100 dark:border-orange-800">
                    {JSON.stringify(ev.evidence_fields, null, 2)}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </Card>
    {/if}

    <!-- Backup Verification -->
    {#if activeTab === "backup"}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Backup Verification</h3>
        {#if data.backup_manifest}
          <div class="space-y-3 text-xs">
            <div class="flex items-center gap-3">
              <Badge color={data.backup_manifest_valid ? "green" : "red"}>
                Manifest {data.backup_manifest_valid ? "Valid" : "Invalid"}
              </Badge>
              <Badge color={data.backup_payload_valid ? "green" : "red"}>
                Payload {data.backup_payload_valid ? "Valid" : "Invalid"}
              </Badge>
            </div>
            <div class="grid grid-cols-2 gap-3">
              <div>
                <span class="text-gray-500">Backup ID</span>
                <p class="font-mono text-gray-800 dark:text-gray-200">{data.backup_manifest.backup_id}</p>
              </div>
              <div>
                <span class="text-gray-500">Kind</span>
                <p class="text-gray-800 dark:text-gray-200">{data.backup_manifest.kind}</p>
              </div>
              <div>
                <span class="text-gray-500">Schema Version</span>
                <p class="font-mono text-gray-800 dark:text-gray-200">{data.backup_manifest.schema_version}</p>
              </div>
              <div>
                <span class="text-gray-500">Created At</span>
                <p class="text-gray-800 dark:text-gray-200">{formatTs(data.backup_manifest.created_at)}</p>
              </div>
            </div>
            <div>
              <span class="text-gray-500">Manifest Hash</span>
              <p class="font-mono text-gray-800 dark:text-gray-200 break-all">{data.backup_manifest.manifest_hash}</p>
            </div>
            <div>
              <span class="text-gray-500">Content Hash</span>
              <p class="font-mono text-gray-800 dark:text-gray-200 break-all">{data.backup_manifest.content_hash}</p>
            </div>
          </div>
        {:else}
          <p class="text-xs text-gray-400">No backup manifest available for this target.</p>
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
    <div class="text-center py-16 text-gray-400 text-sm">Enter a target ID to begin forensic investigation.</div>
  {/if}
</div>
