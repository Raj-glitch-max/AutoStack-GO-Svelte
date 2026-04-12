<script lang="ts">
  import { onMount } from 'svelte';
  import { getCostEstimate } from '$lib/api/cost';
  import type { CostEstimate } from '$lib/types/cost';
  import { Alert, Spinner, Tooltip } from 'flowbite-svelte';
  import { 
    DollarSign, 
    TrendingUp, 
    Server, 
    Network, 
    HardDrive, 
    ArrowLeftRight,
    Info,
    AlertTriangle
  } from 'lucide-svelte';

  export let blueprint: string;
  export let region: string;
  export let variables: Record<string, any> = {};

  let estimate: CostEstimate | null = null;
  let loading = false;
  let error: string | null = null;

  async function fetchEstimate() {
    if (!blueprint || !region) {
      return;
    }

    loading = true;
    error = null;

    try {
      const response = await getCostEstimate(blueprint, region, variables);
      if (response) {
        estimate = response.estimate;
      } else {
        throw new Error('No estimate data received');
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to fetch cost estimate';
      estimate = null;
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    fetchEstimate();
  });

  // Reactive updates when props change
  $: if (blueprint || region) {
    fetchEstimate();
  }

  function formatCurrency(amount: number): string {
    return amount.toFixed(2);
  }

  function formatDate(dateString: string): string {
    try {
      return new Date(dateString).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric'
      });
    } catch {
      return dateString;
    }
  }
</script>

<div class="cost-estimator border border-gray-200 dark:border-gray-700 rounded-lg p-6 bg-white dark:bg-gray-800">
  <div class="flex items-center gap-2 mb-4">
    <DollarSign class="w-5 h-5 text-blue-500" />
    <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
      Estimated Monthly Cost
    </h3>
  </div>

  {#if loading}
    <div class="flex flex-col items-center justify-center py-12 space-y-3">
      <Spinner size="8" />
      <p class="text-sm text-gray-500 dark:text-gray-400">Calculating costs...</p>
    </div>
  {:else if error}
    <Alert color="red" class="mb-4">
      <AlertTriangle class="w-4 h-4 inline-block mr-2" />
      <span class="font-medium">Error:</span> {error}
    </Alert>
  {:else if estimate}
    <!-- Cost Range Display -->
    <div class="mb-6 p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800">
      <div class="flex items-baseline justify-between mb-2">
        <div class="flex items-center gap-2">
          <TrendingUp class="w-4 h-4 text-blue-600 dark:text-blue-400" />
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">Cost Range</span>
        </div>
        <Tooltip class="text-xs">
          Estimated range based on typical usage patterns
        </Tooltip>
      </div>
      <div class="text-3xl font-bold text-blue-600 dark:text-blue-400 mb-1">
        ${formatCurrency(estimate.range.min)} - ${formatCurrency(estimate.range.max)}
      </div>
      <div class="text-sm text-gray-600 dark:text-gray-400">
        Best estimate: <span class="font-semibold">${formatCurrency(estimate.total)}</span> per month
      </div>
    </div>

    <!-- Cost Breakdown -->
    <div class="mb-6">
      <div class="flex items-center gap-2 mb-3">
        <Info class="w-4 h-4 text-gray-500" />
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">Cost Breakdown</h4>
      </div>
      <div class="space-y-2">
        <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <div class="flex items-center gap-2">
            <Server class="w-4 h-4 text-gray-500 dark:text-gray-400" />
            <span class="text-sm text-gray-700 dark:text-gray-300">Compute</span>
          </div>
          <span class="text-sm font-semibold text-gray-900 dark:text-white">
            ${formatCurrency(estimate.breakdown.compute)}
          </span>
        </div>
        <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <div class="flex items-center gap-2">
            <Network class="w-4 h-4 text-gray-500 dark:text-gray-400" />
            <span class="text-sm text-gray-700 dark:text-gray-300">Networking</span>
          </div>
          <span class="text-sm font-semibold text-gray-900 dark:text-white">
            ${formatCurrency(estimate.breakdown.networking)}
          </span>
        </div>
        <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <div class="flex items-center gap-2">
            <HardDrive class="w-4 h-4 text-gray-500 dark:text-gray-400" />
            <span class="text-sm text-gray-700 dark:text-gray-300">Storage</span>
          </div>
          <span class="text-sm font-semibold text-gray-900 dark:text-white">
            ${formatCurrency(estimate.breakdown.storage)}
          </span>
        </div>
        <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
          <div class="flex items-center gap-2">
            <ArrowLeftRight class="w-4 h-4 text-gray-500 dark:text-gray-400" />
            <span class="text-sm text-gray-700 dark:text-gray-300">Data Transfer</span>
          </div>
          <span class="text-sm font-semibold text-gray-900 dark:text-white">
            ${formatCurrency(estimate.breakdown.transfer)}
          </span>
        </div>
      </div>
    </div>

    <!-- Assumptions -->
    {#if estimate.assumptions && Object.keys(estimate.assumptions).length > 0}
      <div class="mb-6">
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white mb-3">Usage Assumptions</h4>
        <div class="space-y-1">
          {#each Object.entries(estimate.assumptions) as [key, value]}
            <div class="flex items-start gap-2 text-sm">
              <span class="text-gray-500 dark:text-gray-400">•</span>
              <span class="text-gray-700 dark:text-gray-300">
                <span class="font-medium capitalize">{key.replace(/_/g, ' ')}:</span> {value}
              </span>
            </div>
          {/each}
        </div>
      </div>
    {/if}

    <!-- Disclaimer -->
    <Alert color="yellow" class="mb-4">
      <div class="flex items-start gap-2">
        <AlertTriangle class="w-4 h-4 mt-0.5 flex-shrink-0" />
        <div class="text-sm">
          <p class="font-medium mb-1">Important Note</p>
          <p class="text-gray-700 dark:text-gray-300">{estimate.disclaimer}</p>
        </div>
      </div>
    </Alert>

    <!-- Pricing Data Freshness -->
    <div class="text-xs text-gray-500 dark:text-gray-400 italic text-center">
      Pricing data from {formatDate(estimate.pricingFetchedAt)}
    </div>
  {:else}
    <div class="text-center py-8 text-gray-500 dark:text-gray-400">
      <p class="text-sm">Select a blueprint and region to see cost estimates</p>
    </div>
  {/if}
</div>
