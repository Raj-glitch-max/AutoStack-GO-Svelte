<script lang="ts">
  import { onMount } from "svelte";
  import { client } from "$lib/pocketbase";
  import { Badge, Card, Spinner, Alert } from "flowbite-svelte";

  import { page } from "$app/stores";

  let loading = true;
  let error = "";
  let data: any = null;
  let executionID = $page.url.searchParams.get("id") ?? "";

  async function fetchReplay() {
    if (!executionID.trim()) {
      error = "Enter an execution ID";
      return;
    }
    loading = true;
    error = "";
    try {
      data = await client.send(`/api/v1/platform/replay/${executionID.trim()}`, { method: "GET" });
    } catch (e: any) {
      error = e?.message ?? "Failed to load replay data";
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (executionID) {
      fetchReplay();
    } else {
      loading = false;
    }
  });

  const eventColor: Record<string, string> = {
    enqueued: "blue",
    dequeued: "green",
    blocked: "red",
    paused: "yellow",
    resumed: "teal",
  };
</script>

<div class="max-w-5xl mx-auto px-6 pt-10 pb-12 space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Replay Explorer</h1>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
      Read-only forensic reconstruction of scheduler event sequence.
      Replay does not re-execute any stage.
    </p>
  </div>

  <div class="flex gap-3">
    <input
      bind:value={executionID}
      placeholder="Execution ID"
      class="flex-1 rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-800 dark:text-white"
    />
    <button
      on:click={fetchReplay}
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
    {#if data.replay_order?.length}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">
          Replay Order ({data.replay_order.length} stages)
        </h3>
        <ol class="space-y-1">
          {#each data.replay_order as stageID, i}
            <li class="flex items-center gap-2 text-xs">
              <span class="w-6 text-right text-gray-400 font-mono">{i + 1}.</span>
              <span class="font-mono text-gray-800 dark:text-gray-200">{stageID}</span>
            </li>
          {/each}
        </ol>
      </Card>
    {/if}

    {#if data.events?.length}
      <Card>
        <h3 class="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">
          Event Log ({data.events.length} events)
        </h3>
        <div class="space-y-2">
          {#each data.events as event}
            <div class="flex items-start gap-3 py-1.5 border-b border-gray-100 dark:border-gray-700 last:border-0">
              <span class="w-8 text-right text-xs text-gray-400 font-mono mt-0.5">#{event.sequence}</span>
              <div class="flex-1">
                <div class="flex items-center gap-2">
                  <Badge color={eventColor[event.event_type] ?? "gray"} class="text-xs">
                    {event.event_type}
                  </Badge>
                  <span class="text-xs font-mono text-gray-700 dark:text-gray-300">{event.stage_id}</span>
                </div>
                <p class="text-xs text-gray-400 mt-0.5">{event.timestamp}</p>
              </div>
            </div>
          {/each}
        </div>
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
