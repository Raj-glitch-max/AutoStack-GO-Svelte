<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { getActualCost } from '$lib/api/cost';
  import type { ActualCostResponse, ActualCostDelayResponse } from '$lib/types/cost';
  import { Alert, Spinner, Tooltip, Badge } from 'flowbite-svelte';
  import { 
    DollarSign, 
    TrendingUp,
    TrendingDown,
    Minus,
    Clock,
    RefreshCw,
    AlertTriangle,
    CheckCircle,
    Info
  } from 'lucide-svelte';

  export let deploymentId: string;
  export let autoRefresh = true;
  export let refreshInterval = 300000; // 5 minutes default

  let actualCost: ActualCostResponse | null = null;
  let delayInfo: ActualCostDelayResponse | null = null;
  let loading = false;
  let error: string | null = null;
  let refreshTimer: ReturnType<typeof setInterval> | null = null;

  async function fetchActualCost() {
    if (!deploymentId) {
      return;
    }

    loading = true;
    error = null;
    delayInfo = null;

    try {
      const response = await getActualCost(deploymentId);
      
      if (!response) {
        throw new Error('No cost data received');
      }

      // Check if response is a delay message (202 Accepted)
      if ('message' in response && 'availableIn' in response) {
        delayInfo = response as ActualCostDelayResponse;
        actualCost = null;
      } else {
        actualCost = response as ActualCostResponse;
        delayInfo = null;
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to fetch actual costs';
      actualCost = null;
      delayInfo = null;
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    fetchActualCost();
    
    // Set up auto-refresh if enabled
    if (autoRefresh) {
      refreshTimer = setInterval(fetchActualCost, refreshInterval);
    }
  });

  onDestroy(() => {
    if (refreshTimer) {
      clearInterval(refreshTimer);
    }
  });

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

  function getVarianceColor(variance: number): string {
    if (variance < -10) return 'text-green-600 dark:text-green-400';
    if (variance < 10) return 'text-orange-600 dark:text-orange-400';
    return 'text-red-600 dark:text-red-400';
  }

  function getVarianceBgColor(variance: number): string {
    if (variance < -10) return 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800';
    if (variance < 10) return 'bg-orange-50 dark:bg-orange-900/20 border-orange-200 dark:border-orange-800';
    return 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800';
  }

  function getVarianceIcon(variance: number) {
    if (variance < -10) return TrendingDown;
    if (variance < 10) return Minus;
    return TrendingUp;
  }

  function getVarianceStatus(variance: number): string {
    if (variance < -10) return 'Under budget 👍';
    if (variance < 10) return 'On track ✅';
    return 'Over budget ⚠️';
  }

  function getVarianceBadgeColor(variance: number): 'green' | 'yellow' | 'red' {
    if (variance < -10) return 'green';
    if (variance < 10) return 'yellow';
    return 'red';
  }
</script>

<div class="actual-cost-display border border-gray-200 dark:border-gray-700 rounded-lg p-6 bg-white dark:bg-gray-800">
  <div class="flex items-center justify-between mb-4">
    <div class="flex items-center gap-2">
      <DollarSign class="w-5 h-5 text-blue-500" />
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
        Actual Costs
      </h3>
    </div>
    {#if !loading && actualCost}
      <button
        on:click={fetchActualCost}
        class="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
        title="Refresh cost data"
      >
        <RefreshCw class="w-4 h-4" />
      </button>
    {/if}
  </div>

  {#if loading}
    <div class="flex flex-col items-center justify-center py-12 space-y-3">
      <Spinner size="8" />
      <p class="text-sm text-gray-500 dark:text-gray-400">Loading actual costs...</p>
    </div>
  {:else if error}
    <Alert color="red" class="mb-4">
      <AlertTriangle class="w-4 h-4 inline-block mr-2" />
      <span class="font-medium">Error:</span> {error}
    </Alert>
  {:else if delayInfo}
    <!-- 48-hour delay message -->
    <Alert color="blue" class="mb-4">
      <div class="flex items-start gap-3">
        <Clock class="w-5 h-5 mt-0.5 flex-shrink-0" />
        <div>
          <p class="font-medium mb-2">{delayInfo.message}</p>
          <div class="text-sm space-y-1">
            <p>Deployment age: <span class="font-semibold">{delayInfo.deploymentAge}</span></p>
            <p>Available in: <span class="font-semibold">{delayInfo.availableIn}</span></p>
          </div>
          <p class="text-xs mt-3 text-gray-600 dark:text-gray-400">
            AWS Cost Explorer requires a 48-hour delay before cost data becomes available. 
            Check back later to see your actual costs.
          </p>
        </div>
      </div>
    </Alert>
  {:else if actualCost}
    <!-- Cost Comparison Grid -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
      <!-- Current Spend -->
      <div class="p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800">
        <div class="flex items-center gap-2 mb-2">
          <DollarSign class="w-4 h-4 text-blue-600 dark:text-blue-400" />
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">Current Spend</h4>
        </div>
        <div class="text-2xl font-bold text-blue-600 dark:text-blue-400 mb-1">
          ${formatCurrency(actualCost.actual.costToDate)}
        </div>
        <div class="text-xs text-gray-600 dark:text-gray-400">
          Cost to date
        </div>
        <div class="mt-2 pt-2 border-t border-blue-200 dark:border-blue-700">
          <div class="text-sm text-gray-700 dark:text-gray-300">
            Projected: <span class="font-semibold">${formatCurrency(actualCost.actual.projectedMonthly)}</span>/month
          </div>
        </div>
      </div>

      <!-- Original Estimate -->
      <div class="p-4 bg-gray-50 dark:bg-gray-700/50 rounded-lg border border-gray-200 dark:border-gray-600">
        <div class="flex items-center gap-2 mb-2">
          <Info class="w-4 h-4 text-gray-600 dark:text-gray-400" />
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">Original Estimate</h4>
        </div>
        <div class="text-2xl font-bold text-gray-900 dark:text-white mb-1">
          ${formatCurrency(actualCost.estimate.total)}
        </div>
        <div class="text-xs text-gray-600 dark:text-gray-400">
          Per month
        </div>
        <div class="mt-2 pt-2 border-t border-gray-200 dark:border-gray-600">
          <div class="text-xs text-gray-600 dark:text-gray-400">
            From {formatDate(actualCost.estimate.createdAt)}
          </div>
        </div>
      </div>

      <!-- Variance -->
      <div class="p-4 rounded-lg border {getVarianceBgColor(actualCost.actual.variance)}">
        <div class="flex items-center gap-2 mb-2">
          <svelte:component this={getVarianceIcon(actualCost.actual.variance)} class="w-4 h-4 {getVarianceColor(actualCost.actual.variance)}" />
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">Variance</h4>
        </div>
        <div class="text-2xl font-bold {getVarianceColor(actualCost.actual.variance)} mb-1">
          {actualCost.actual.variance > 0 ? '+' : ''}{actualCost.actual.variance.toFixed(1)}%
        </div>
        <div class="text-xs text-gray-600 dark:text-gray-400 mb-2">
          From estimate
        </div>
        <Badge color={getVarianceBadgeColor(actualCost.actual.variance)} class="mt-2">
          {getVarianceStatus(actualCost.actual.variance)}
        </Badge>
      </div>
    </div>

    <!-- Service-Level Cost Breakdown -->
    <div class="mb-6">
      <div class="flex items-center gap-2 mb-3">
        <Info class="w-4 h-4 text-gray-500" />
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">Cost by Service</h4>
      </div>
      
      {#if Object.keys(actualCost.actual.breakdown).length > 0}
        <div class="space-y-2">
          {#each Object.entries(actualCost.actual.breakdown).sort((a, b) => b[1] - a[1]) as [service, cost]}
            <div class="flex items-center justify-between p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
              <div class="flex items-center gap-2">
                <div class="w-2 h-2 rounded-full bg-blue-500"></div>
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{service}</span>
              </div>
              <span class="text-sm font-bold text-gray-900 dark:text-white">
                ${formatCurrency(cost)}
              </span>
            </div>
          {/each}
        </div>
      {:else}
        <div class="text-center py-4 text-gray-500 dark:text-gray-400">
          <p class="text-sm">No service breakdown available</p>
        </div>
      {/if}
    </div>

    <!-- Cost Period -->
    <div class="pt-4 border-t border-gray-200 dark:border-gray-700">
      <div class="flex items-center justify-between text-xs text-gray-600 dark:text-gray-400">
        <div class="flex items-center gap-2">
          <Clock class="w-3 h-3" />
          <span>
            Period: {actualCost.actual.period.start} to {actualCost.actual.period.end}
          </span>
        </div>
        <div class="flex items-center gap-2">
          <CheckCircle class="w-3 h-3" />
          <span>
            Updated {formatDate(actualCost.actual.fetchedAt)}
          </span>
        </div>
      </div>
    </div>

    <!-- Auto-refresh indicator -->
    {#if autoRefresh}
      <div class="mt-3 text-center">
        <p class="text-xs text-gray-500 dark:text-gray-400 italic">
          Auto-refreshing every {Math.floor(refreshInterval / 60000)} minutes
        </p>
      </div>
    {/if}
  {:else}
    <div class="text-center py-8 text-gray-500 dark:text-gray-400">
      <p class="text-sm">No cost data available</p>
    </div>
  {/if}
</div>
