<script lang="ts">
  import { pb } from '$lib/pocketbase';
  import { onMount } from 'svelte';
  import { Button, Modal, Label, Input, Select, Badge, Table, TableBody, TableBodyCell, TableBodyRow, TableHead, TableHeadCell, Checkbox, Toggle } from 'flowbite-svelte';
  import { Plus, Trash2, Globe, CheckCircle, XCircle } from 'lucide-svelte';

  export let projectId: string;
  export let userRole: string = 'viewer';

  let webhooks: any[] = [];
  let loading = true;
  let showAddModal = false;
  let newWebhook = {
    url: '',
    provider: 'generic',
    events: ['deploy.success', 'deploy.failed'],
    active: true
  };
  let saving = false;

  const providers = [
    { value: 'slack', name: 'Slack' },
    { value: 'discord', name: 'Discord' },
    { value: 'generic', name: 'Generic Webhook' }
  ];

  const eventTypes = [
    { id: 'deploy.started', label: 'Deployment Started' },
    { id: 'deploy.success', label: 'Deployment Success' },
    { id: 'deploy.failed', label: 'Deployment Failed' },
    { id: 'cost.alert', label: 'Cost Threshold Alert' },
    { id: 'health.degraded', label: 'Infrastructure Degraded' }
  ];

  onMount(async () => {
    await fetchWebhooks();
  });

  async function fetchWebhooks() {
    loading = true;
    try {
      webhooks = await pb.collection('webhooks').getFullList({
        filter: `project = "${projectId}"`
      });
    } catch (err) {
      console.error('Error fetching webhooks:', err);
    } finally {
      loading = false;
    }
  }

  async function addWebhook() {
    saving = true;
    try {
      await pb.collection('webhooks').create({
        project: projectId,
        ...newWebhook
      });
      showAddModal = false;
      newWebhook = { url: '', provider: 'generic', events: ['deploy.success', 'deploy.failed'], active: true };
      await fetchWebhooks();
    } catch (err) {
      console.error('Error adding webhook:', err);
      alert('Failed to add webhook.');
    } finally {
      saving = false;
    }
  }

  async function toggleWebhook(webhook: any) {
    try {
      await pb.collection('webhooks').update(webhook.id, {
        active: !webhook.active
      });
      await fetchWebhooks();
    } catch (err) {
      console.error('Error toggling webhook:', err);
      alert('Failed to update webhook.');
    }
  }

  async function deleteWebhook(id: string) {
    if (!confirm('Are you sure you want to delete this webhook?')) return;
    try {
      await pb.collection('webhooks').delete(id);
      await fetchWebhooks();
    } catch (err) {
      console.error('Error deleting webhook:', err);
      alert('Failed to delete webhook.');
    }
  }

  function handleEventToggle(eventId: string) {
    if (newWebhook.events.includes(eventId)) {
      newWebhook.events = newWebhook.events.filter(e => e !== eventId);
    } else {
      newWebhook.events = [...newWebhook.events, eventId];
    }
  }
</script>

<div class="space-y-4">
  <div class="flex justify-between items-center">
    <div>
      <h3 class="text-xl font-bold text-white">Webhooks</h3>
      <p class="text-gray-400 text-sm">Notify external services about project events.</p>
    </div>
    {#if userRole === 'owner'}
      <Button color="alternative" on:click={() => showAddModal = true}>
        <Plus class="w-4 h-4 mr-2" />
        Add Webhook
      </Button>
    {/if}
  </div>

  <div class="bg-gray-800 rounded-lg overflow-hidden border border-gray-700">
    <Table hoverable={true}>
      <TableHead class="bg-gray-700 text-gray-300">
        <TableHeadCell>URL / Provider</TableHeadCell>
        <TableHeadCell>Subscribed Events</TableHeadCell>
        <TableHeadCell>Status</TableHeadCell>
        <TableHeadCell>Actions</TableHeadCell>
      </TableHead>
      <TableBody>
        {#each webhooks as webhook}
          <TableBodyRow class="border-gray-700">
            <TableBodyCell>
              <div class="flex items-center space-x-3">
                <Globe class="w-6 h-6 text-gray-500" />
                <div>
                  <div class="text-white font-medium truncate max-w-xs">{webhook.url}</div>
                  <Badge color="dark" class="mt-1">{webhook.provider.toUpperCase()}</Badge>
                </div>
              </div>
            </TableBodyCell>
            <TableBodyCell>
              <div class="flex flex-wrap gap-1">
                {#each webhook.events as event}
                  <Badge color="blue">{event}</Badge>
                {/each}
              </div>
            </TableBodyCell>
            <TableBodyCell>
              <Toggle checked={webhook.active} on:change={() => toggleWebhook(webhook)} disabled={userRole !== 'owner'} />
            </TableBodyCell>
            <TableBodyCell>
              {#if userRole === 'owner'}
                <Button color="red" size="xs" on:click={() => deleteWebhook(webhook.id)}>
                  <Trash2 class="w-4 h-4" />
                </Button>
              {/if}
            </TableBodyCell>
          </TableBodyRow>
        {/each}
        {#if webhooks.length === 0 && !loading}
          <TableBodyRow>
            <TableBodyCell colspan="4" class="text-center py-8 text-gray-500">
              No webhooks configured.
            </TableBodyCell>
          </TableBodyRow>
        {/if}
      </TableBody>
    </Table>
  </div>
</div>

<Modal title="Add New Webhook" bind:open={showAddModal} autoclose={false} size="md" class="bg-gray-800">
  <div class="space-y-4">
    <div>
      <Label for="url" class="text-white mb-2">Endpoint URL</Label>
      <Input id="url" type="url" placeholder="https://hooks.slack.com/services/..." bind:value={newWebhook.url} class="bg-gray-700 border-gray-600 text-white" />
    </div>
    <div>
      <Label for="provider" class="text-white mb-2">Provider Type</Label>
      <Select id="provider" items={providers} bind:value={newWebhook.provider} class="bg-gray-700 border-gray-600 text-white" />
    </div>
    
    <div>
      <Label class="text-white mb-2">Events to Subscribe</Label>
      <div class="grid grid-cols-2 gap-2 mt-2">
        {#each eventTypes as event}
          <div class="flex items-center space-x-2 bg-gray-700/50 p-2 rounded">
            <Checkbox checked={newWebhook.events.includes(event.id)} on:change={() => handleEventToggle(event.id)} />
            <span class="text-gray-300 text-sm">{event.label}</span>
          </div>
        {/each}
      </div>
    </div>

    <div class="flex justify-end space-x-3 mt-6">
      <Button color="alternative" on:click={() => showAddModal = false}>Cancel</Button>
      <Button color="blue" on:click={addWebhook} disabled={saving || !newWebhook.url}>
        {saving ? 'Adding...' : 'Add Webhook'}
      </Button>
    </div>
  </div>
</Modal>
