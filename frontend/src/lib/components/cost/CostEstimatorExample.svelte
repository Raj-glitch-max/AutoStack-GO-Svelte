<script lang="ts">
  import CostEstimator from './CostEstimator.svelte';
  import { Card } from 'flowbite-svelte';

  // Example: Static values
  let blueprint = 'static-website';
  let region = 'us-east-1';
  let variables = {};

  // Example: Available blueprints and regions
  const blueprints = [
    { id: 'static-website', name: 'Static Website' },
    { id: 'web-application', name: 'Web Application' },
    { id: 'full-stack-app', name: 'Full Stack App' },
    { id: 'microservices', name: 'Microservices' }
  ];

  const regions = [
    { id: 'us-east-1', name: 'US East (N. Virginia)' },
    { id: 'us-west-2', name: 'US West (Oregon)' },
    { id: 'eu-west-1', name: 'EU (Ireland)' },
    { id: 'ap-southeast-1', name: 'Asia Pacific (Singapore)' }
  ];
</script>

<div class="container mx-auto p-6 max-w-4xl">
  <h1 class="text-3xl font-bold mb-6 text-gray-900 dark:text-white">
    AWS Cost Estimation Example
  </h1>

  <Card class="mb-6">
    <h2 class="text-xl font-semibold mb-4 text-gray-900 dark:text-white">
      Configuration
    </h2>
    
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div>
        <label for="blueprint" class="block mb-2 text-sm font-medium text-gray-900 dark:text-white">
          Blueprint
        </label>
        <select
          id="blueprint"
          bind:value={blueprint}
          class="bg-gray-50 border border-gray-300 text-gray-900 text-sm rounded-lg focus:ring-blue-500 focus:border-blue-500 block w-full p-2.5 dark:bg-gray-700 dark:border-gray-600 dark:placeholder-gray-400 dark:text-white"
        >
          {#each blueprints as bp}
            <option value={bp.id}>{bp.name}</option>
          {/each}
        </select>
      </div>

      <div>
        <label for="region" class="block mb-2 text-sm font-medium text-gray-900 dark:text-white">
          AWS Region
        </label>
        <select
          id="region"
          bind:value={region}
          class="bg-gray-50 border border-gray-300 text-gray-900 text-sm rounded-lg focus:ring-blue-500 focus:border-blue-500 block w-full p-2.5 dark:bg-gray-700 dark:border-gray-600 dark:placeholder-gray-400 dark:text-white"
        >
          {#each regions as r}
            <option value={r.id}>{r.name}</option>
          {/each}
        </select>
      </div>
    </div>
  </Card>

  <CostEstimator {blueprint} {region} {variables} />

  <Card class="mt-6">
    <h2 class="text-xl font-semibold mb-4 text-gray-900 dark:text-white">
      About Cost Estimates
    </h2>
    <div class="prose dark:prose-invert max-w-none">
      <p class="text-sm text-gray-700 dark:text-gray-300 mb-3">
        Cost estimates are calculated based on current AWS pricing data and typical usage patterns.
        The range shown represents the expected minimum and maximum costs based on variable usage.
      </p>
      <p class="text-sm text-gray-700 dark:text-gray-300 mb-3">
        <strong>What's included:</strong> Compute resources, networking costs, storage, and data transfer
        based on the selected blueprint and region.
      </p>
      <p class="text-sm text-gray-700 dark:text-gray-300">
        <strong>Note:</strong> Actual costs may vary based on your specific usage patterns, data transfer volumes,
        and any additional AWS services you enable.
      </p>
    </div>
  </Card>
</div>
