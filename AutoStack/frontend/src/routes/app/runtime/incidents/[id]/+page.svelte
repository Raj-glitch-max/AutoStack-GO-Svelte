<script lang="ts">
  import { page } from "$app/stores";
  import { onMount } from "svelte";
  import { goto } from "$app/navigation";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Button, Spinner, Alert } from "flowbite-svelte";
  import { ArrowLeft, RefreshCw } from "lucide-svelte";

  const incidentId = $page.params.id;

  let loading = true;
  let error = "";
  let incident: any = null;
  let narrative: any = null;
  let narrativeLoading = false;

  async function fetchIncident() {
    loading = true;
    error = "";
    try {
      incident = await client.send(`/api/v1/reconciler/incidents/${incidentId}`, { method: "GET" });
      // Also fetch narrative for the incident's target if available.
      if (incident?.target) {
        narrativeLoading = true;
        try {
          narrative = await client.send(
            `/api/v1/reconciler/narrative/${incident.target}?window_hours=168`,
            { method: "GET" }
          );
        } catch (_) {
          // Narrative is best-effort.
        } finally {
          narrativeLoading = false;
        }
      }
    } catch (e: any) {
      error = e?.message ?? "Failed to load incident";
    } finally {
      loading = false;
    }
  }

  onMount(fetchIncident);

  function severityColor(sev: string) {
    switch (sev) {
      case "critical": return "red";
      case "high": return "orange";
      case "medium": return "yellow";
      default: return "gray";
    }
  }

  function statusColor(s: string) {
    switch (s) {
      case "open": return "blue";
      case "escalating": return "red";
      case "acknowledged": return "yellow";
      case "resolved": return "green";
      case "reopened": return "orange";
      case "suppressed": return "dark";
      default: return "gray";
    }
  }

  function entryIcon(kind: string) {
    switch (kind) {
      case "decision": return "📋";
      case "operation": return "🔧";
      case "history": return "📜";
      case "incident": return "🚨";
      default: return "·";
    }
  }
</script>

<div class="max-w-4xl mx-auto px-4 pt-16 pb-8 space-y-6">
  <div class="flex items-center gap-3">
    <button on:click={() => goto("/app/runtime/incidents")} class="text-gray-500 hover:text-gray-800">
      <ArrowLeft class="w-5 h-5" />
    </button>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Incident Detail</h1>
    <span class="font-mono text-sm text-gray-400">{incidentId}</span>
    <button on:click={fetchIncident} class="ml-auto text-gray-500 hover:text-gray-800">
      <RefreshCw class="w-4 h-4" />
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><Spinner /></div>
  {:else if error}
    <Alert color="red">{error}</Alert>
  {:else if incident}

    <!-- Incident metadata -->
    <Card>
      <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-4">
        <div>
          <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Status</div>
          <Badge color={statusColor(incident.status)}>{incident.status}</Badge>
        </div>
        <div>
          <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Severity</div>
          <Badge color={severityColor(incident.severity)}>{incident.severity}</Badge>
        </div>
        <div>
          <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Source</div>
          <span class="text-sm font-mono">{incident.source}</span>
        </div>
        <div>
          <div class="text-xs text-gray-500 uppercase tracking-wide mb-1">Escalations</div>
          <span class="text-xl font-bold {incident.escalation_count > 5 ? 'text-red-600' : 'text-gray-700'}">{incident.escalation_count}</span>
        </div>
      </div>

      <div class="space-y-2 text-sm">
        <div><span class="text-gray-500">Target:</span> <span class="font-mono">{incident.target}</span></div>
        <div><span class="text-gray-500">First seen:</span> {incident.first_seen_at}</div>
        <div><span class="text-gray-500">Last seen:</span> {incident.last_seen_at}</div>
        {#if incident.resolved_at}
          <div><span class="text-gray-500">Resolved at:</span> {incident.resolved_at}</div>
          <div><span class="text-gray-500">Resolution reason:</span> {incident.resolution_reason}</div>
        {/if}
        {#if incident.acknowledged_by}
          <div><span class="text-gray-500">Acknowledged by:</span> {incident.acknowledged_by}</div>
        {/if}
      </div>

      {#if incident.forensic_summary}
        <div class="mt-4 p-3 bg-gray-50 dark:bg-gray-800 rounded text-sm font-mono">
          {incident.forensic_summary}
        </div>
      {/if}
    </Card>

    <!-- Runtime state snapshots (forensic evidence at creation time) -->
    {#if incident.retry_state || incident.ambiguity_state || incident.lease_state}
      <Card>
        <h3 class="font-semibold text-gray-800 dark:text-white mb-3">Runtime State at Incident Creation</h3>
        <p class="text-xs text-gray-400 mb-3">
          These snapshots capture what the runtime believed when this incident was first opened.
          They are forensic evidence — not current state.
        </p>
        <div class="space-y-2 text-sm font-mono">
          {#if incident.retry_state}
            <div><span class="text-gray-500">Retry state:</span> {incident.retry_state}</div>
          {/if}
          {#if incident.ambiguity_state}
            <div><span class="text-gray-500">Ambiguity state:</span> {incident.ambiguity_state}</div>
          {/if}
          {#if incident.lease_state}
            <div><span class="text-gray-500">Lease state:</span> {incident.lease_state}</div>
          {/if}
        </div>
      </Card>
    {/if}

    <!-- Narrative timeline -->
    <div>
      <h3 class="font-semibold text-gray-800 dark:text-white mb-3">Event Narrative (last 7 days)</h3>
      <p class="text-xs text-gray-400 mb-3">
        Deterministically ordered events from decisions, operations, history, and incidents.
        Two loads against unchanged DB state produce the same ordering.
      </p>

      {#if narrativeLoading}
        <div class="flex justify-center py-6"><Spinner size="6" /></div>
      {:else if narrative?.entries?.length > 0}
        <div class="space-y-2 max-h-96 overflow-y-auto pr-2">
          {#each narrative.entries as entry}
            <div class="flex gap-3 text-sm border-l-2 pl-3 py-1
              {entry.kind === 'incident' ? 'border-red-400' :
               entry.kind === 'decision' ? 'border-blue-400' :
               entry.kind === 'operation' ? 'border-green-400' :
               'border-gray-300'}">
              <span class="text-base mt-0.5">{entryIcon(entry.kind)}</span>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-mono text-xs text-gray-400">{entry.timestamp}</span>
                  <Badge color="gray" class="text-xs">{entry.kind}</Badge>
                  {#if entry.label}
                    <span class="text-gray-700 dark:text-gray-300 text-xs">{entry.label}</span>
                  {/if}
                </div>
                {#if entry.summary}
                  <div class="text-gray-600 dark:text-gray-400 text-xs mt-0.5 truncate">{entry.summary}</div>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <div class="text-gray-400 text-sm">No narrative entries available for this target.</div>
      {/if}
    </div>

  {/if}
</div>
