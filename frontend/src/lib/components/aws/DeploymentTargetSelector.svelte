<script lang="ts">
  export let selectedTarget: 'kubernetes' | 'aws' = 'kubernetes';
  
  const targets = [
    { 
      id: 'kubernetes', 
      name: 'Kubernetes', 
      description: 'Deploy to your Kubernetes cluster',
      icon: '⚙️'
    },
    { 
      id: 'aws', 
      name: 'AWS', 
      description: 'Deploy to AWS infrastructure with Terraform',
      icon: '☁️'
    }
  ];
</script>

<div class="deployment-target-selector">
  <h3 class="text-lg font-medium text-gray-900 dark:text-white mb-4">
    Choose Deployment Target
  </h3>
  
  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    {#each targets as target}
      <button
        class="target-option p-4 border-2 rounded-lg transition-all duration-200 text-left
               {selectedTarget === target.id 
                 ? 'border-primary-600 bg-primary-50 dark:bg-primary-900/20' 
                 : 'border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600'}"
        on:click={() => {
          if (target.id === 'kubernetes' || target.id === 'aws') {
            selectedTarget = target.id;
          }
        }}
      >
        <div class="flex items-start space-x-3">
          <div class="text-2xl">{target.icon}</div>
          <div class="flex-1">
            <div class="flex items-center space-x-2">
              <h4 class="font-medium text-gray-900 dark:text-white">
                {target.name}
              </h4>
              {#if selectedTarget === target.id}
                <div class="w-2 h-2 bg-primary-600 rounded-full"></div>
              {/if}
            </div>
            <p class="text-sm text-gray-600 dark:text-gray-400 mt-1">
              {target.description}
            </p>
          </div>
        </div>
      </button>
    {/each}
  </div>
</div>

<style>
  .target-option:hover {
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  }
  
  .target-option.selected {
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(59, 130, 246, 0.15);
  }
</style>