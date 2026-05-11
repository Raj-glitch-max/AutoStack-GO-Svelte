<script lang="ts">
  import { onMount } from 'svelte';
  import { pb } from '$lib/pocketbase';
  import type { CostAlert } from '$lib/types/cost';
  import { 
    AlertTriangle, 
    TrendingUp, 
    TrendingDown, 
    Check,
    CheckCircle2,
    Clock,
    Search,
    Filter,
    ArrowRight
  } from 'lucide-svelte';
  import { 
    Button, 
    Card, 
    Badge, 
    Input, 
    Select,
    Spinner,
    Breadcrumb,
    BreadcrumbItem
  } from 'flowbite-svelte';
  import { goto } from '$app/navigation';
  import toast from 'svelte-french-toast';

  let alerts: CostAlert[] = [];
  let loading = true;
  let searchTerm = '';
  let typeFilter = 'all';

  async function fetchAlerts() {
    loading = true;
    try {
      const response = await fetch(`${pb.baseUrl}/api/alerts`, {
        headers: {
          'Authorization': pb.authStore.token
        }
      });
      if (response.ok) {
        alerts = await response.json();
      }
    } catch (err) {
      console.error('Error fetching alerts:', err);
      toast.error('Failed to load alerts');
    } finally {
      loading = false;
    }
  }

  async function acknowledgeAlert(alertId: string) {
    try {
      const response = await fetch(`${pb.baseUrl}/api/alerts/${alertId}/acknowledge`, {
        method: 'POST',
        headers: {
          'Authorization': pb.authStore.token
        }
      });
      if (response.ok) {
        alerts = alerts.map(a => a.id === alertId ? { ...a, acknowledged: true } : a);
        toast.success('Alert cleared');
      }
    } catch (err) {
      console.error('Error acknowledging alert:', err);
      toast.error('Failed to clear alert');
    }
  }

  async function acknowledgeAll() {
    const unread = alerts.filter(a => !a.acknowledged);
    if (unread.length === 0) return;

    try {
      const response = await fetch(`${pb.baseUrl}/api/alerts/acknowledge-all`, {
        method: 'POST',
        headers: {
          'Authorization': pb.authStore.token
        }
      });
      if (response.ok) {
        alerts = alerts.map(a => ({ ...a, acknowledged: true }));
        toast.success('All alerts cleared');
      }
    } catch (err) {
      console.error('Error acknowledging all alerts:', err);
      toast.error('Failed to clear all alerts');
    }
  }

  function formatTime(dateStr: string | undefined) {
    if (!dateStr) return 'Unknown';
    return new Date(dateStr).toLocaleString();
  }

  $: filteredAlerts = alerts.filter(alert => {
    const matchesSearch = alert.message.toLowerCase().includes(searchTerm.toLowerCase()) || 
                         alert.deployment.toLowerCase().includes(searchTerm.toLowerCase());
    const matchesType = typeFilter === 'all' || alert.type === typeFilter;
    return matchesSearch && matchesType;
  });

  onMount(fetchAlerts);
</script>

<div class="container mx-auto p-4 lg:p-8">
  <Breadcrumb aria-label="Alerts navigation" class="mb-6">
    <BreadcrumbItem href="/app" home>Dashboard</BreadcrumbItem>
    <BreadcrumbItem>Cost Alerts</BreadcrumbItem>
  </Breadcrumb>

  <div class="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 mb-8">
    <div>
      <h1 class="text-3xl font-bold text-gray-900 dark:text-white mb-2">Cost Alerts</h1>
      <p class="text-gray-500 dark:text-gray-400">Manage and respond to cost anomalies across your deployments.</p>
    </div>
    
    {#if alerts.some(a => !a.acknowledged)}
      <Button color="alternative" on:click={acknowledgeAll}>
        <CheckCircle2 class="w-4 h-4 mr-2" /> Mark all as read
      </Button>
    {/if}
  </div>

  <div class="grid grid-cols-1 md:grid-cols-4 gap-6 mb-8">
    <Card class="md:col-span-3">
      <div class="flex flex-col md:flex-row gap-4 mb-6">
        <div class="flex-1">
          <Input placeholder="Search alerts..." bind:value={searchTerm}>
            <Search slot="left" class="w-4 h-4 text-gray-400" />
          </Input>
        </div>
        <div class="w-full md:w-48">
          <Select bind:value={typeFilter}>
            <option value="all">All Types</option>
            <option value="SPIKE">Cost Spike</option>
            <option value="DROP">Cost Drop</option>
            <option value="budget_exceeded">Budget Alert</option>
          </Select>
        </div>
      </div>

      {#if loading}
        <div class="flex justify-center py-12">
          <Spinner size="8" />
        </div>
      {:else if filteredAlerts.length === 0}
        <div class="text-center py-12 bg-gray-50 dark:bg-gray-800/50 rounded-xl border-2 border-dashed border-gray-200 dark:border-gray-700">
          <AlertTriangle class="w-12 h-12 mx-auto mb-4 text-gray-300" />
          <p class="text-gray-500">No alerts found matching your criteria.</p>
        </div>
      {:else}
        <div class="space-y-4">
          {#each filteredAlerts as alert}
            <div class="p-4 rounded-xl border transition-all {alert.acknowledged ? 'bg-gray-50 dark:bg-gray-800/40 border-gray-200 dark:border-gray-700 opacity-75' : 'bg-white dark:bg-gray-800 border-primary-100 dark:border-primary-900/30 shadow-sm'}">
              <div class="flex flex-col md:flex-row gap-4">
                <div class="hidden md:block">
                  {#if alert.type === 'SPIKE'}
                    <div class="p-3 bg-red-100 dark:bg-red-900/30 text-red-600 rounded-xl">
                      <TrendingUp class="w-6 h-6" />
                    </div>
                  {:else if alert.type === 'DROP'}
                    <div class="p-3 bg-blue-100 dark:bg-blue-900/30 text-blue-600 rounded-xl">
                      <TrendingDown class="w-6 h-6" />
                    </div>
                  {:else}
                    <div class="p-3 bg-amber-100 dark:bg-amber-900/30 text-amber-600 rounded-xl">
                      <AlertTriangle class="w-6 h-6" />
                    </div>
                  {/if}
                </div>

                <div class="flex-1">
                  <div class="flex flex-wrap items-center gap-2 mb-2">
                    <h3 class="font-bold text-gray-900 dark:text-white">
                      {#if alert.type === 'SPIKE'}Cost Spike Detected{:else if alert.type === 'DROP'}Significant Cost Drop{:else}Budget Threshold Exceeded{/if}
                    </h3>
                    {#if !alert.acknowledged}
                      <Badge color="red" rounded>New</Badge>
                    {/if}
                    <div class="ml-auto text-xs text-gray-500 flex items-center gap-1">
                      <Clock class="w-3 h-3" /> {formatTime(alert.created)}
                    </div>
                  </div>

                  <p class="text-sm text-gray-600 dark:text-gray-400 mb-4">
                    {alert.message}
                  </p>

                  <div class="flex flex-wrap items-center justify-between gap-4">
                    <div class="flex gap-6">
                      {#if alert.actualCost !== undefined}
                        <div>
                          <p class="text-[10px] uppercase tracking-wider text-gray-400 font-bold mb-1">Actual</p>
                          <p class="text-sm font-semibold text-gray-900 dark:text-white">${alert.actualCost.toFixed(2)}/mo</p>
                        </div>
                      {/if}
                      {#if alert.estimatedCost !== undefined}
                        <div>
                          <p class="text-[10px] uppercase tracking-wider text-gray-400 font-bold mb-1">Estimate</p>
                          <p class="text-sm font-semibold text-gray-900 dark:text-white">${alert.estimatedCost.toFixed(2)}/mo</p>
                        </div>
                      {/if}
                      {#if alert.variance !== undefined}
                        <div>
                          <p class="text-[10px] uppercase tracking-wider text-gray-400 font-bold mb-1">Variance</p>
                          <Badge color={alert.variance > 50 ? 'red' : alert.variance > 0 ? 'yellow' : 'blue'}>
                            {alert.variance > 0 ? '+' : ''}{alert.variance.toFixed(1)}%
                          </Badge>
                        </div>
                      {/if}
                    </div>

                    <div class="flex gap-2">
                      <Button size="sm" color="light" on:click={() => goto(`/app/deployments/${alert.deploymentId || alert.deployment || ''}`)}>
                        View Deployment <ArrowRight class="w-3 h-3 ml-2" />
                      </Button>
                      {#if !alert.acknowledged}
                        <Button size="sm" color="alternative" on:click={() => acknowledgeAlert(alert.id)}>
                          <Check class="w-3 h-3 mr-2" /> Clear Alert
                        </Button>
                      {/if}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </Card>

    <div class="space-y-6">
      <Card>
        <h3 class="font-bold text-gray-900 dark:text-white mb-4">Statistics</h3>
        <div class="space-y-4">
          <div class="flex justify-between items-center">
            <span class="text-sm text-gray-500">Total Alerts</span>
            <span class="font-semibold text-gray-900 dark:text-white">{alerts.length}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-sm text-gray-500">Unread</span>
            <span class="font-semibold text-red-500">{alerts.filter(a => !a.acknowledged).length}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-sm text-gray-500">Cost Spikes</span>
            <span class="font-semibold text-gray-900 dark:text-white">{alerts.filter(a => a.type === 'SPIKE').length}</span>
          </div>
        </div>
      </Card>

      <Card class="bg-primary-600 text-white">
        <h3 class="font-bold mb-2">Cost Advice</h3>
        <p class="text-xs text-primary-100 mb-4">
          Cost anomalies are often caused by unexpected scaling or resource leaks. Review your service breakdown to identify the source.
        </p>
        <Button color="light" size="xs" class="w-full" on:click={() => goto('/app/optimizer')}>
          View AI Recommendations
        </Button>
      </Card>
    </div>
  </div>
</div>
