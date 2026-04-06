<script lang="ts">
  import { onMount } from 'svelte';
  import { Card, Spinner, Badge } from 'flowbite-svelte';
  import { DollarSign, TrendingUp, Info } from 'lucide-svelte';
  
  export let blueprint: string = '';
  export let region: string = 'us-east-1';
  export let configuration: any = {};
  
  let costEstimate: any = null;
  let loading = false;
  let error = '';
  
  // Reactive statement to update cost when inputs change
  $: if (blueprint && region) {
    updateCostEstimate();
  }
  
  async function updateCostEstimate() {
    if (!blueprint) return;
    
    loading = true;
    error = '';
    
    try {
      const response = await fetch('/api/aws/cost-estimate', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          blueprint,
          region,
          configuration
        })
      });
      
      if (response.ok) {
        costEstimate = await response.json();
      } else {
        error = 'Failed to get cost estimate';
      }
    } catch (err) {
      console.error('Cost estimation error:', err);
      error = 'Failed to calculate costs';
    } finally {
      loading = false;
    }
  }
  
  onMount(() => {
    updateCostEstimate();
  });
  
  function formatCurrency(amount: number): string {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2
    }).format(amount);
  }
  
  function getCostColor(amount: number): string {
    if (amount < 10) return 'green';
    if (amount < 50) return 'yellow';
    if (amount < 100) return 'orange';
    return 'red';
  }
</script>

<Card class="cost-estimator">
  <div class="flex items-center justify-between mb-4">
    <h3 class="text-lg font-semibold text-gray-900 dark:text-white flex items-center gap-2">
      <DollarSign class="w-5 h-5 text-green-600" />
      Cost Estimate
    </h3>
    {#if loading}
      <Spinner size="4" />
    {/if}
  </div>
  
  {#if loading}
    <div class="flex justify-center items-center py-8">
      <div class="text-center">
        <Spinner size="6" />
        <p class="text-sm text-gray-500 mt-2">Calculating costs...</p>
      </div>
    </div>
  {:else if error}
    <div class="text-center py-8">
      <div class="text-red-600 dark:text-red-400 mb-2">
        <Info class="w-8 h-8 mx-auto mb-2" />
        {error}
      </div>
      <p class="text-sm text-gray-500">
        Using fallback estimates based on typical usage
      </p>
      
      <!-- Fallback estimates -->
      <div class="mt-4 p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
        <div class="text-2xl font-bold text-gray-700 dark:text-gray-300">
          {#if blueprint === 'ecs-web-app'}
            ~$32/month
          {:else if blueprint === 'full-stack'}
            ~$53/month
          {:else if blueprint === 'static-site'}
            ~$2/month
          {:else}
            ~$25/month
          {/if}
        </div>
        <div class="text-sm text-gray-500 mt-1">
          Estimated monthly cost
        </div>
      </div>
    </div>
  {:else if costEstimate}
    <div class="space-y-4">
      <!-- Total Cost Display -->
      <div class="text-center p-6 bg-gradient-to-r from-green-50 to-blue-50 dark:from-green-900/20 dark:to-blue-900/20 rounded-lg border">
        <div class="text-3xl font-bold text-gray-900 dark:text-white">
          {formatCurrency(costEstimate.totalMonthlyCost)}
        </div>
        <div class="text-sm text-gray-600 dark:text-gray-400 mt-1">
          Estimated monthly cost in {costEstimate.region}
        </div>
        <Badge color={getCostColor(costEstimate.totalMonthlyCost)} class="mt-2">
          {#if costEstimate.totalMonthlyCost < 10}
            Very Low Cost
          {:else if costEstimate.totalMonthlyCost < 50}
            Low Cost
          {:else if costEstimate.totalMonthlyCost < 100}
            Moderate Cost
          {:else}
            High Cost
          {/if}
        </Badge>
      </div>
      
      <!-- Cost Breakdown -->
      {#if costEstimate.breakdown && Object.keys(costEstimate.breakdown).length > 0}
        <div class="space-y-3">
          <h4 class="font-medium text-gray-900 dark:text-white flex items-center gap-2">
            <TrendingUp class="w-4 h-4" />
            Cost Breakdown
          </h4>
          
          {#each Object.entries(costEstimate.breakdown) as [service, cost]}
            <div class="flex justify-between items-center py-2 px-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
              <span class="text-sm text-gray-700 dark:text-gray-300">
                {service}
              </span>
              <span class="font-medium text-gray-900 dark:text-white">
                {formatCurrency(cost)}
              </span>
            </div>
          {/each}
          
          <!-- Total Line -->
          <div class="flex justify-between items-center py-2 px-3 bg-gray-100 dark:bg-gray-700 rounded-lg border-t">
            <span class="font-medium text-gray-900 dark:text-white">
              Total Monthly
            </span>
            <span class="font-bold text-gray-900 dark:text-white">
              {formatCurrency(costEstimate.totalMonthlyCost)}
            </span>
          </div>
        </div>
      {/if}
      
      <!-- Disclaimer -->
      {#if costEstimate.disclaimer}
        <div class="text-xs text-gray-500 dark:text-gray-400 p-3 bg-yellow-50 dark:bg-yellow-900/20 rounded-lg border border-yellow-200 dark:border-yellow-800">
          <Info class="w-3 h-3 inline mr-1" />
          {costEstimate.disclaimer}
        </div>
      {/if}
      
      <!-- Cost Optimization Tips -->
      <div class="text-xs text-gray-500 dark:text-gray-400 space-y-1">
        <div class="font-medium text-gray-700 dark:text-gray-300 mb-2">💡 Cost Optimization Tips:</div>
        {#if blueprint === 'ecs-web-app'}
          <div>• Use smaller instance sizes for development environments</div>
          <div>• Enable auto-scaling to optimize compute costs</div>
          <div>• Consider using Spot instances for non-critical workloads</div>
        {:else if blueprint === 'full-stack'}
          <div>• Use RDS reserved instances for production databases</div>
          <div>• Enable automated backups with appropriate retention</div>
          <div>• Monitor database performance and right-size instances</div>
        {:else if blueprint === 'static-site'}
          <div>• Enable CloudFront compression to reduce data transfer</div>
          <div>• Use S3 Intelligent Tiering for automatic cost optimization</div>
          <div>• Set up lifecycle policies for old content</div>
        {/if}
      </div>
    </div>
  {:else}
    <div class="text-center py-8 text-gray-500 dark:text-gray-400">
      <DollarSign class="w-12 h-12 mx-auto mb-4 opacity-50" />
      <p>Select a blueprint to see cost estimates</p>
    </div>
  {/if}
</Card>

<style>
  .cost-estimator {
    min-height: 300px;
  }
</style>