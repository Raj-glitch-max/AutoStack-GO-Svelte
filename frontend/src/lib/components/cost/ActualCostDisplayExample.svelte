<script lang="ts">
  import ActualCostDisplay from './ActualCostDisplay.svelte';

  // Example deployment ID - replace with actual deployment ID
  let deploymentId = 'your-deployment-id-here';
  
  // Optional: disable auto-refresh for testing
  let autoRefresh = true;
  
  // Optional: custom refresh interval (in milliseconds)
  let refreshInterval = 300000; // 5 minutes
</script>

<div class="container mx-auto p-6 max-w-4xl">
  <h1 class="text-2xl font-bold mb-6 text-gray-900 dark:text-white">
    Actual Cost Display Example
  </h1>

  <div class="mb-6">
    <label for="deploymentId" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
      Deployment ID
    </label>
    <input
      id="deploymentId"
      type="text"
      bind:value={deploymentId}
      placeholder="Enter deployment ID"
      class="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent"
    />
  </div>

  <div class="mb-6 flex items-center gap-4">
    <label class="flex items-center gap-2">
      <input
        type="checkbox"
        bind:checked={autoRefresh}
        class="w-4 h-4 text-blue-600 bg-gray-100 border-gray-300 rounded focus:ring-blue-500"
      />
      <span class="text-sm text-gray-700 dark:text-gray-300">Auto-refresh enabled</span>
    </label>
  </div>

  {#if deploymentId && deploymentId !== 'your-deployment-id-here'}
    <ActualCostDisplay 
      {deploymentId} 
      {autoRefresh}
      {refreshInterval}
    />
  {:else}
    <div class="border border-gray-200 dark:border-gray-700 rounded-lg p-6 bg-white dark:bg-gray-800">
      <p class="text-center text-gray-500 dark:text-gray-400">
        Enter a deployment ID above to view actual costs
      </p>
    </div>
  {/if}

  <!-- Usage Documentation -->
  <div class="mt-8 p-6 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-700">
    <h2 class="text-lg font-semibold mb-4 text-gray-900 dark:text-white">
      Component Usage
    </h2>
    
    <div class="space-y-4 text-sm text-gray-700 dark:text-gray-300">
      <div>
        <h3 class="font-semibold mb-2">Basic Usage:</h3>
        <pre class="bg-gray-800 text-gray-100 p-4 rounded-lg overflow-x-auto"><code>&lt;script&gt;
  import ActualCostDisplay from '$lib/components/cost/ActualCostDisplay.svelte';
&lt;/script&gt;

&lt;ActualCostDisplay deploymentId="deploy_123" /&gt;</code></pre>
      </div>

      <div>
        <h3 class="font-semibold mb-2">With Custom Options:</h3>
        <pre class="bg-gray-800 text-gray-100 p-4 rounded-lg overflow-x-auto"><code>&lt;ActualCostDisplay 
  deploymentId="deploy_123"
  autoRefresh={true}
  refreshInterval={300000}
/&gt;</code></pre>
      </div>

      <div>
        <h3 class="font-semibold mb-2">Props:</h3>
        <ul class="list-disc list-inside space-y-1 ml-4">
          <li><code class="bg-gray-200 dark:bg-gray-700 px-1 rounded">deploymentId</code> (required): The deployment ID to fetch costs for</li>
          <li><code class="bg-gray-200 dark:bg-gray-700 px-1 rounded">autoRefresh</code> (optional, default: true): Enable automatic cost data refresh</li>
          <li><code class="bg-gray-200 dark:bg-gray-700 px-1 rounded">refreshInterval</code> (optional, default: 300000): Refresh interval in milliseconds</li>
        </ul>
      </div>

      <div>
        <h3 class="font-semibold mb-2">Features:</h3>
        <ul class="list-disc list-inside space-y-1 ml-4">
          <li>Displays current spend and projected monthly cost</li>
          <li>Shows variance from original estimate with color coding (green/orange/red)</li>
          <li>Service-level cost breakdown sorted by highest cost</li>
          <li>Handles 48-hour AWS Cost Explorer delay with informative messaging</li>
          <li>Automatic refresh with configurable interval</li>
          <li>Manual refresh button</li>
          <li>Loading states and error handling</li>
        </ul>
      </div>
    </div>
  </div>
</div>
