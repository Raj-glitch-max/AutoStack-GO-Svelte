<script lang="ts">
  import { healthSummary, updateHealthSummary } from "$lib/stores/data";
  import { onMount, onDestroy } from "svelte";
  import { Card, Badge, Spinner, Button, Tooltip } from "flowbite-svelte";
  import { 
    Activity, 
    CheckCircle2, 
    AlertCircle, 
    HelpCircle, 
    RefreshCw, 
    Server, 
    Database, 
    Network,
    Box,
    Cpu,
    Zap,
    ArrowRight
  } from "lucide-svelte";
  import type { DeploymentHealth, HealthStatus } from "$lib/types/health";
  import { fade, slide } from "svelte/transition";

  let interval: any;
  let refreshing = false;

  onMount(() => {
    updateHealthSummary();
    interval = setInterval(async () => {
      refreshing = true;
      await updateHealthSummary();
      refreshing = false;
    }, 30000); // Poll every 30 seconds
  });

  onDestroy(() => {
    if (interval) clearInterval(interval);
  });

  async function handleRefresh() {
    refreshing = true;
    await updateHealthSummary();
    refreshing = false;
  }

  function getStatusColor(status: HealthStatus): "blue" | "green" | "yellow" | "red" | "indigo" | "purple" | "pink" | "none" | "dark" | "primary" {
    switch (status) {
      case 'healthy': return 'green';
      case 'unhealthy': return 'red';
      case 'pending': return 'yellow';
      case 'degraded': return 'yellow';
      default: return 'dark';
    }
  }

  function getStatusIcon(status: HealthStatus) {
    switch (status) {
      case 'healthy': return CheckCircle2;
      case 'unhealthy': return AlertCircle;
      case 'pending': return RefreshCw;
      case 'degraded': return AlertCircle;
      default: return HelpCircle;
    }
  }

  function getResourceTypeIcon(type: string) {
    switch (type) {
      case 'ECS_SERVICE': return Server;
      case 'ALB_TARGET_GROUP': return Network;
      case 'RDS_INSTANCE': return Database;
      case 'K8S_ROLLOUT': return Zap;
      case 'K8S_POD': return Box;
      case 'K8S_METRICS': return Cpu;
      default: return Box;
    }
  }
</script>

<div class="space-y-6">
  <div class="flex justify-between items-center">
    <div class="flex items-center space-x-3">
      <div class="p-2 bg-primary-100 dark:bg-primary-900/30 rounded-lg text-primary-600 dark:text-primary-400">
        <Activity size={24} />
      </div>
      <div>
        <h2 class="text-2xl font-bold dark:text-white">Infrastructure Health</h2>
        <p class="text-sm text-gray-500 dark:text-gray-400">Real-time status of your AWS and Kubernetes resources</p>
      </div>
    </div>
    
    <Button 
      outline 
      color="alternative" 
      size="sm" 
      class="flex items-center space-x-2" 
      on:click={handleRefresh}
      disabled={refreshing}
    >
      <RefreshCw size={16} class={refreshing ? 'animate-spin' : ''} />
      <span>{refreshing ? 'Refreshing...' : 'Refresh Now'}</span>
    </Button>
  </div>

  {#if $healthSummary.length === 0}
    <div class="flex flex-col items-center justify-center p-12 bg-gray-50 dark:bg-gray-800/50 rounded-2xl border-2 border-dashed border-gray-200 dark:border-gray-700">
      <Activity size={48} class="text-gray-300 dark:text-gray-600 mb-4" />
      <h3 class="text-lg font-medium dark:text-white">No Active Deployments</h3>
      <p class="text-gray-500 dark:text-gray-400 text-center max-w-xs mt-2">
        Health monitoring will automatically start once you have active AWS or Kubernetes deployments.
      </p>
    </div>
  {:else}
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      {#each $healthSummary as health (health.deploymentId)}
        <div in:fade={{ duration: 300 }}>
          <Card class="w-full max-w-none border-none shadow-xl bg-white/80 dark:bg-gray-800/80 backdrop-blur-md overflow-hidden relative group">
            <!-- Status Glow Effect -->
            <div 
              class="absolute top-0 right-0 w-32 h-32 -mr-16 -mt-16 rounded-full blur-3xl opacity-20 transition-colors duration-500"
              class:bg-green-500={health.overallStatus === 'healthy'}
              class:bg-red-500={health.overallStatus === 'unhealthy'}
              class:bg-yellow-500={health.overallStatus === 'pending'}
              class:bg-orange-500={health.overallStatus === 'degraded'}
              class:bg-gray-500={health.overallStatus === 'unknown'}
            ></div>

            <div class="p-6">
              <div class="flex justify-between items-start mb-6">
                <div>
                  <h3 class="font-bold text-lg dark:text-white truncate max-w-[180px]">
                    {health.deploymentId}
                  </h3>
                  <p class="text-xs text-gray-500 dark:text-gray-400 mt-1 uppercase tracking-wider">
                    {health.deploymentType === 'aws' ? 'AWS Deployment' : 'Kubernetes Deployment'}
                  </p>
                </div>
                <Badge color={getStatusColor(health.overallStatus)} class="px-2.5 py-1 rounded-full uppercase text-[10px] font-bold tracking-widest">
                  <svelte:component this={getStatusIcon(health.overallStatus)} size={12} class="mr-1 inline" />
                  {health.overallStatus}
                </Badge>
              </div>

              <div class="space-y-4">
                {#each health.resources as resource}
                  <div class="flex items-center justify-between p-3 rounded-xl bg-gray-50/50 dark:bg-gray-900/30 border border-gray-100 dark:border-gray-700/50 group-hover:border-primary-500/20 transition-all">
                    <div class="flex items-center space-x-3">
                      <div class="p-2 bg-white dark:bg-gray-800 rounded-lg shadow-sm">
                        <svelte:component 
                          this={getResourceTypeIcon(resource.resourceType)} 
                          size={18} 
                          class="text-gray-600 dark:text-gray-400" 
                        />
                      </div>
                      <div class="min-w-0">
                        <p class="text-xs font-bold dark:text-white truncate max-w-[120px]">{resource.resourceId}</p>
                        <p class="text-[10px] text-gray-500 dark:text-gray-500 uppercase">{resource.resourceType.replace('_', ' ')}</p>
                      </div>
                    </div>
                    
                    <div class="flex flex-col items-end">
                      <div 
                        class="w-2 h-2 rounded-full animate-pulse"
                        class:bg-green-500={resource.status === 'healthy'}
                        class:bg-red-500={resource.status === 'unhealthy'}
                        class:bg-yellow-500={resource.status === 'pending'}
                        class:bg-orange-500={resource.status === 'degraded'}
                        class:bg-gray-500={resource.status === 'unknown'}
                      ></div>
                      <p class="text-[10px] mt-1 text-gray-400 dark:text-gray-500">{resource.status}</p>
                    </div>
                  </div>
                  
                  {#if resource.status === 'unhealthy'}
                    <div class="px-3 py-2 bg-red-50 dark:bg-red-900/20 rounded-lg border border-red-100 dark:border-red-900/30" transition:slide>
                      <p class="text-[10px] text-red-600 dark:text-red-400 font-medium leading-relaxed">
                        <AlertCircle size={10} class="inline mr-1 mb-0.5" />
                        {resource.message}
                      </p>
                    </div>
                  {/if}
                {/each}
              </div>
              
              <div class="mt-6 flex justify-end">
                <Button size="xs" color="primary" outline class="rounded-lg group-hover:bg-primary-600 group-hover:text-white transition-colors">
                  View Logs
                  <ArrowRight size={14} class="ml-1" />
                </Button>
              </div>
            </div>
          </Card>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  :global(.dark) .backdrop-blur-md {
    background-color: rgba(31, 41, 55, 0.8);
  }
</style>
