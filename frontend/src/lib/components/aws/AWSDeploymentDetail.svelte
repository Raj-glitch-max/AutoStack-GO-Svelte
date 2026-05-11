<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { Badge, Button, Card, Tabs, TabItem, Spinner } from 'flowbite-svelte';
  import { 
    Cloud, 
    Activity, 
    FileText, 
    Settings, 
    ExternalLink, 
    RefreshCw,
    Trash2,
    Play,
    Square,
    DollarSign,
    Server,
    Database,
    Globe,
    Clock,
    AlertCircle
  } from 'lucide-svelte';
  import toast from 'svelte-french-toast';
  import ErrorAnalysis from '$lib/components/intelligence/ErrorAnalysis.svelte';
  
  export let deploymentId: string;
  
  let deployment: any = null;
  let loading = true;
  let logs: string[] = [];
  let wsConnection: WebSocket | null = null;
  let activeTab = 'overview';
  let rollouts: any[] = [];
  let costData: any = null;
  let loadingCost = false;
  let costError: string | null = null;
  let healthData: any = null;
  let loadingHealth = false;
  let healthInterval: any = null;
  
  // Status color mapping - using Flowbite Badge colors
  const statusColors: Record<string, "blue" | "green" | "yellow" | "red" | "indigo" | "purple" | "pink" | "none" | "dark" | "primary"> = {
    pending: 'yellow',
    planning: 'blue',
    applying: 'blue',
    deploying: 'blue',
    active: 'green',
    failed: 'red',
    destroying: 'yellow',
    destroyed: 'dark'
  };
  
  function getOutputValue(output: any): string {
    return output && typeof output === 'object' && output.value ? String(output.value) : '';
  }

  function isOutputSensitive(output: any): boolean {
    return output && typeof output === 'object' && output.sensitive === true;
  }

  onMount(async () => {
    await loadDeployment();
    await fetchRollouts();
    await fetchCostData();
    await fetchHealthData();
    connectWebSocket();
    
    // Poll health every 30 seconds
    healthInterval = setInterval(fetchHealthData, 30000);
  });
  
  onDestroy(() => {
    if (wsConnection) {
      wsConnection.close();
    }
    if (healthInterval) {
      clearInterval(healthInterval);
    }
  });
  
  async function loadDeployment() {
    try {
      const response = await fetch(`/api/aws/deployments/${deploymentId}`);
      if (response.ok) {
        deployment = await response.json();
      } else {
        toast.error('Failed to load deployment details');
      }
    } catch (error) {
      console.error('Error loading deployment:', error);
      toast.error('Failed to load deployment details');
    } finally {
      loading = false;
    }
  }

  async function fetchRollouts() {
    try {
      const response = await fetch(`/api/aws/deployments/${deploymentId}/rollouts`);
      if (response.ok) {
        rollouts = await response.json();
      }
    } catch (err) {
      console.error('Error fetching rollouts:', err);
    }
  }

  async function fetchCostData() {
    if (!deploymentId) return;
    loadingCost = true;
    costError = null;
    try {
      const response = await fetch(`/api/cost/actual/${deploymentId}`);
      if (response.ok) {
        costData = await response.json();
      } else if (response.status === 202) {
        // Data not yet available (48h delay)
        const data = await response.json();
        costData = { pending: true, message: data.message, availableIn: data.availableIn };
      } else {
        costError = "Failed to load cost data";
      }
    } catch (err) {
      console.error("Error fetching cost data:", err);
      costError = "Network error while fetching costs";
    } finally {
      loadingCost = false;
    }
  }

  async function fetchHealthData() {
    if (!deploymentId || deployment?.status !== 'active') return;
    loadingHealth = true;
    try {
      const response = await fetch(`/api/health/detail/${deploymentId}`);
      if (response.ok) {
        healthData = await response.json();
      }
    } catch (err) {
      console.error("Error fetching health data:", err);
    } finally {
      loadingHealth = false;
    }
  }
  
  function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws/aws/terraform-logs?deploymentId=${deploymentId}`;
    
    wsConnection = new WebSocket(wsUrl);
    
    wsConnection.onopen = () => {
      console.log('WebSocket connected for AWS deployment logs');
    };
    
    wsConnection.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'log') {
          logs = [...logs, `[${data.level.toUpperCase()}] ${data.message}`];
        } else if (data.type === 'status') {
          console.log('Status update received:', data);
          if (deployment) {
            deployment.status = data.state;
            deployment.errorMessage = data.message;
            // If status changed to a final state, reload full details to get outputs
            if (['active', 'failed', 'destroyed'].includes(data.state)) {
              loadDeployment();
            }
          }
        }
      } catch (error) {
        console.error('Error parsing WebSocket message:', error);
      }
    };
    
    wsConnection.onclose = () => {
      console.log('WebSocket connection closed');
      // Attempt to reconnect after 5 seconds
      setTimeout(connectWebSocket, 5000);
    };
    
    wsConnection.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  }
  
  async function handleDeploy() {
    try {
      const response = await fetch(`/api/aws/deployments/${deploymentId}/deploy`, {
        method: 'POST'
      });
      
      if (response.ok) {
        toast.success('Deployment workflow started');
        await loadDeployment();
      } else {
        toast.error('Failed to start deployment');
      }
    } catch (error) {
      console.error('Error starting deployment:', error);
      toast.error('Failed to start deployment');
    }
  }
  
  async function handleDestroy() {
    if (!confirm('Are you sure you want to destroy this infrastructure? This action cannot be undone.')) {
      return;
    }
    
    try {
      const response = await fetch(`/api/aws/deployments/${deploymentId}/destroy`, {
        method: 'POST'
      });
      
      if (response.ok) {
        toast.success('Infrastructure destruction started');
        await loadDeployment();
      } else {
        toast.error('Failed to start destruction');
      }
    } catch (error) {
      console.error('Error starting destruction:', error);
      toast.error('Failed to start destruction');
    }
  }
  
  function getResourceIcon(resourceType: string) {
    if (resourceType.includes('database') || resourceType.includes('rds')) {
      return Database;
    } else if (resourceType.includes('load_balancer') || resourceType.includes('alb')) {
      return Globe;
    } else if (resourceType.includes('cluster') || resourceType.includes('ecs')) {
      return Server;
    }
    return Cloud;
  }

  function getStatusBadgeColor(status: string): "blue" | "green" | "yellow" | "red" | "indigo" | "purple" | "pink" | "none" | "dark" | "primary" {
    switch (status.toLowerCase()) {
      case 'healthy': return 'green';
      case 'unhealthy': return 'red';
      case 'pending': return 'yellow';
      case 'degraded': return 'yellow';
      default: return 'dark';
    }
  }
</script>

{#if loading}
  <div class="flex justify-center items-center h-64">
    <Spinner size="8" />
  </div>
{:else if deployment}
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex justify-between items-start">
      <div>
        <h1 class="text-3xl font-bold text-gray-900 dark:text-white flex items-center gap-3">
          <Cloud class="w-8 h-8 text-blue-600" />
          {deployment.name}
        </h1>
        <div class="flex items-center gap-4 mt-2">
          <Badge color={statusColors[deployment.status] || 'dark'}>
            {deployment.status.toUpperCase()}
          </Badge>
          <span class="text-sm text-gray-500">Region: {deployment.region}</span>
          <span class="text-sm text-gray-500">
            Created: {new Date(deployment.created).toLocaleDateString()}
          </span>
        </div>
      </div>
      
      <div class="flex gap-2">
        <Button color="alternative" size="sm" on:click={loadDeployment}>
          <RefreshCw class="w-4 h-4 mr-2" />
          Refresh
        </Button>
        
        {#if deployment.status === 'active'}
          <Button color="red" size="sm" on:click={handleDestroy}>
            <Trash2 class="w-4 h-4 mr-2" />
            Destroy
          </Button>
        {:else if ['pending', 'failed', 'destroyed'].includes(deployment.status)}
          <Button color="blue" size="sm" on:click={handleDeploy}>
            <Play class="w-4 h-4 mr-2" />
            Deploy Now
          </Button>
        {/if}
      </div>
    </div>
    
    <!-- Tabs -->
    <Tabs bind:activeTabValue={activeTab}>
      <TabItem value="overview" title="Overview">
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <!-- Status Card -->
          <Card>
            <div class="flex items-center justify-between mb-4">
              <h3 class="text-lg font-semibold">Deployment Status</h3>
              <Activity class="w-5 h-5 text-gray-500" />
            </div>
            
            <div class="space-y-3">
              <div class="flex justify-between">
                <span class="text-gray-600">Status:</span>
                <Badge color={statusColors[deployment.status] || 'dark'}>
                  {deployment.status.toUpperCase()}
                </Badge>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-600">Region:</span>
                <span class="font-medium">{deployment.region}</span>
              </div>
              <div class="flex justify-between">
                <span class="text-gray-600">Last Updated:</span>
                <span class="font-medium">
                  {new Date(deployment.updated).toLocaleString()}
                </span>
              </div>
            </div>
          </Card>
          
          {#if deployment.status === 'failed' && deployment.errorAnalysis}
            <Card class="border-red-200 bg-red-50 dark:bg-red-900/10 dark:border-red-800">
              <div class="flex items-center justify-between mb-4">
                <h3 class="text-lg font-semibold text-red-800 dark:text-red-200 flex items-center gap-2">
                  <AlertCircle class="w-5 h-5" />
                  Analysis Results
                </h3>
              </div>
              
              {@const analysis = JSON.parse(deployment.errorAnalysis)}
              <div class="space-y-3">
                <div class="text-sm font-bold text-red-700 dark:text-red-300">
                  {analysis.description}
                </div>
                <p class="text-xs text-red-600 dark:text-red-400">
                  {analysis.root_cause}
                </p>
                <div class="pt-2">
                  <div class="text-xs font-semibold text-gray-700 dark:text-gray-300 mb-1">Suggested Fix:</div>
                  <p class="text-xs text-gray-600 dark:text-gray-400 italic">
                    {analysis.suggested_fix}
                  </p>
                </div>
              </div>
            </Card>
          {/if}
          
          <!-- Configuration Card -->
          <Card>
            <div class="flex items-center justify-between mb-4">
              <h3 class="text-lg font-semibold">Configuration</h3>
              <Settings class="w-5 h-5 text-gray-500" />
            </div>
            
            <div class="space-y-3">
              {#if deployment.configuration}
                {#each Object.entries(deployment.configuration) as [key, value]}
                  <div class="flex justify-between">
                    <span class="text-gray-600 capitalize">
                      {key.replace(/_/g, ' ')}:
                    </span>
                    <span class="font-medium">{value}</span>
                  </div>
                {/each}
              {/if}
            </div>
          </Card>
        </div>
        
        <!-- Outputs Section -->
        {#if deployment.outputs && Object.keys(deployment.outputs).length > 0}
          <Card class="mt-6">
            <div class="flex items-center justify-between mb-4">
              <h3 class="text-lg font-semibold">Infrastructure Outputs</h3>
              <ExternalLink class="w-5 h-5 text-gray-500" />
            </div>
            
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              {#each Object.entries(deployment.outputs) as [key, output]}
                <div class="p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                  <div class="text-sm text-gray-600 dark:text-gray-400 mb-1">
                    {key.replace(/_/g, ' ').toUpperCase()}
                  </div>
                  {#if key.includes('url') || key.includes('endpoint')}
                    <a 
                      href={getOutputValue(output) || '#'}
                      target="_blank" 
                      class="text-blue-600 hover:text-blue-800 font-medium flex items-center gap-1"
                    >
                      {getOutputValue(output)}
                      <ExternalLink class="w-3 h-3" />
                    </a>
                  {:else}
                    <div class="font-medium text-gray-900 dark:text-white">
                      {isOutputSensitive(output) ? '••••••••' : getOutputValue(output)}
                    </div>
                  {/if}
                </div>
              {/each}
            </div>
          </Card>
        {/if}
      </TabItem>
      
      <TabItem value="logs" title="Terraform Logs">
        <Card>
          <div class="flex items-center justify-between mb-4">
            <h3 class="text-lg font-semibold">Execution Logs</h3>
            <div class="flex items-center gap-2">
              <div class="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
              <span class="text-sm text-gray-500">Live</span>
            </div>
          </div>
          
          <div class="bg-black text-green-400 p-4 rounded-lg font-mono text-sm h-96 overflow-y-auto">
            {#if logs.length === 0}
              <div class="text-gray-500">No logs available. Logs will appear here during Terraform operations.</div>
            {:else}
              {#each logs as log}
                <div class="mb-1">{log}</div>
              {/each}
            {/if}
          </div>
        </Card>
      </TabItem>
      
      <TabItem value="infrastructure" title="Infrastructure">
        <Card>
          <div class="flex items-center justify-between mb-4">
            <h3 class="text-lg font-semibold">Infrastructure Resources</h3>
            <div class="flex items-center gap-2">
              {#if loadingHealth}
                <RefreshCw class="w-4 h-4 text-blue-500 animate-spin" />
              {/if}
              <Server class="w-5 h-5 text-gray-500" />
            </div>
          </div>
          
          {#if deployment.status === 'active' && healthData?.resources}
            <div class="space-y-4">
              {#each healthData.resources as resource}
                <div class="flex items-center gap-3 p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                  <svelte:component this={getResourceIcon(resource.resourceType.toLowerCase())} class="w-5 h-5 text-blue-600" />
                  <div class="flex-grow">
                    <div class="font-medium">{resource.resourceType.replace(/_/g, ' ')}</div>
                    <div class="text-sm text-gray-500 truncate max-w-xs" title={resource.resourceId}>
                      {resource.resourceId}
                    </div>
                    {#if resource.message}
                      <div class="text-xs text-gray-400 mt-1">{resource.message}</div>
                    {/if}
                  </div>
                  <Badge color={getStatusBadgeColor(resource.status)}>
                    {resource.status.toUpperCase()}
                  </Badge>
                </div>
              {/each}
            </div>
          {:else if deployment.status === 'active'}
            <div class="flex flex-col items-center justify-center py-12 text-gray-500">
              <Spinner size="8" class="mb-4" />
              <p>Fetching real-time resource health...</p>
            </div>
          {:else}
            <div class="text-center py-8 text-gray-500">
              Infrastructure resources will appear here once the deployment is active.
            </div>
          {/if}
        </Card>
      </TabItem>
      
      <TabItem value="costs" title="Costs">
        <Card>
          <div class="flex items-center justify-between mb-4">
            <h3 class="text-lg font-semibold">Cost Breakdown</h3>
            <div class="flex items-center gap-2">
              {#if loadingCost}
                <RefreshCw class="w-4 h-4 text-blue-500 animate-spin" />
              {/if}
              <DollarSign class="w-5 h-5 text-gray-500" />
            </div>
          </div>
          
          {#if costData?.pending}
            <div class="text-center py-8 px-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
              <Clock class="w-12 h-12 text-blue-500 mx-auto mb-3" />
              <div class="font-medium text-blue-900 dark:text-blue-100 mb-1">
                Cost Data Pending
              </div>
              <div class="text-sm text-blue-700 dark:text-blue-300">
                {costData.message}. Available in approx. {costData.availableIn}.
              </div>
            </div>
          {:else if costData?.actual}
            <div class="space-y-6">
              <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div class="text-center p-6 bg-green-50 dark:bg-green-900/20 rounded-lg border border-green-100 dark:border-green-800">
                  <div class="text-3xl font-bold text-green-600 dark:text-green-400">
                    ${costData.actual.costToDate.toFixed(2)}
                  </div>
                  <div class="text-sm text-green-700 dark:text-green-300 mt-1">
                    Cost to date ({costData.actual.period.start} to {costData.actual.period.end})
                  </div>
                </div>
                
                <div class="text-center p-6 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-100 dark:border-blue-800">
                  <div class="text-3xl font-bold text-blue-600 dark:text-blue-400">
                    ${costData.actual.projectedMonthly.toFixed(2)}
                  </div>
                  <div class="text-sm text-blue-700 dark:text-blue-300 mt-1">
                    Projected monthly spend
                  </div>
                </div>
              </div>

              <div class="space-y-3">
                <h4 class="text-sm font-semibold text-gray-500 uppercase tracking-wider">Service Breakdown</h4>
                {#each Object.entries(costData.actual.breakdown) as [service, cost]}
                  {@const costValue = typeof cost === 'number' ? cost : 0}
                  {#if costValue > 0}
                    <div class="flex justify-between items-center py-1">
                      <span class="text-gray-600 dark:text-gray-400">{service}</span>
                      <span class="font-medium text-gray-900 dark:text-white">${costValue.toFixed(2)}</span>
                    </div>
                  {/if}
                {/each}
                
                <hr class="my-2 border-gray-200 dark:border-gray-700">
                
                <div class="flex justify-between items-center font-semibold text-gray-900 dark:text-white">
                  <span>Total Actual (Current Period)</span>
                  <span>${costData.actual.costToDate.toFixed(2)}</span>
                </div>

                <div class="mt-4 p-4 bg-gray-50 dark:bg-gray-800 rounded-lg flex items-center justify-between">
                  <div>
                    <div class="text-sm text-gray-500">Variance from Estimate</div>
                    <div class="text-lg font-bold {costData.actual.variance > 0 ? 'text-red-500' : 'text-green-500'}">
                      {costData.actual.variance > 0 ? '+' : ''}{costData.actual.variance.toFixed(1)}%
                    </div>
                  </div>
                  <div class="text-right text-sm text-gray-500">
                    Original Estimate: <span class="font-medium text-gray-700 dark:text-gray-300">${costData.estimate.total.toFixed(2)}</span>
                  </div>
                </div>
              </div>
              
              <div class="text-xs text-gray-500 mt-4 italic">
                * Cost data is synchronized from AWS Cost Explorer daily. Last fetched: {new Date(costData.actual.fetchedAt).toLocaleString()}
              </div>
            </div>
          {:else if costError}
            <div class="text-center py-8 text-red-500">
              <AlertCircle class="w-12 h-12 mx-auto mb-2 opacity-50" />
              <p>{costError}</p>
              <Button size="sm" color="alternative" class="mt-4" on:click={fetchCostData}>
                Try Again
              </Button>
            </div>
          {:else}
             <!-- Fallback to original estimate if no actual data yet -->
             <div class="space-y-4">
                <div class="text-center p-6 bg-gray-50 dark:bg-gray-800 rounded-lg">
                  <div class="text-3xl font-bold text-gray-900 dark:text-white">
                    Processing...
                  </div>
                  <div class="text-sm text-gray-500 mt-1">
                    Initial cost estimate is being calculated.
                  </div>
                </div>
             </div>
          {/if}
        </Card>
      </TabItem>
      
      <!-- Error Analysis Tab - Only show if deployment failed -->
      {#if deployment.status === 'failed' && logs.length > 0}
        <TabItem value="analysis" title="Error Analysis">
          <ErrorAnalysis 
            deploymentId={deployment.id} 
            errorLogs={logs.join('\n')} 
          />
        </TabItem>
      {/if}
    </Tabs>
  </div>
{:else}
  <div class="text-center py-12">
    <h2 class="text-xl font-semibold text-gray-900 dark:text-white mb-2">
      Deployment Not Found
    </h2>
    <p class="text-gray-600 dark:text-gray-400">
      The requested AWS deployment could not be found.
    </p>
  </div>
{/if}