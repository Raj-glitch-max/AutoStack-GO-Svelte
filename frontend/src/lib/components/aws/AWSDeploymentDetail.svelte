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
    Globe
  } from 'lucide-svelte';
  import toast from 'svelte-french-toast';
  
  export let deploymentId: string;
  
  let deployment: any = null;
  let loading = true;
  let logs: string[] = [];
  let wsConnection: WebSocket | null = null;
  let activeTab = 'overview';
  
  // Status color mapping
  const statusColors = {
    pending: 'yellow',
    planning: 'blue',
    applying: 'blue',
    active: 'green',
    failed: 'red',
    destroying: 'orange',
    destroyed: 'gray'
  };
  
  onMount(async () => {
    await loadDeployment();
    connectWebSocket();
  });
  
  onDestroy(() => {
    if (wsConnection) {
      wsConnection.close();
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
  
  async function handlePlan() {
    try {
      const response = await fetch(`/api/aws/deployments/${deploymentId}/plan`, {
        method: 'POST'
      });
      
      if (response.ok) {
        toast.success('Terraform plan started');
        await loadDeployment();
      } else {
        toast.error('Failed to start plan');
      }
    } catch (error) {
      console.error('Error starting plan:', error);
      toast.error('Failed to start plan');
    }
  }
  
  async function handleApply() {
    try {
      const response = await fetch(`/api/aws/deployments/${deploymentId}/apply`, {
        method: 'POST'
      });
      
      if (response.ok) {
        toast.success('Terraform apply started');
        await loadDeployment();
      } else {
        toast.error('Failed to start apply');
      }
    } catch (error) {
      console.error('Error starting apply:', error);
      toast.error('Failed to start apply');
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
          <Badge color={statusColors[deployment.status] || 'gray'}>
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
        {:else if deployment.status === 'pending' || deployment.status === 'failed'}
          <Button color="blue" size="sm" on:click={handlePlan}>
            <FileText class="w-4 h-4 mr-2" />
            Plan
          </Button>
          <Button color="green" size="sm" on:click={handleApply}>
            <Play class="w-4 h-4 mr-2" />
            Apply
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
                <Badge color={statusColors[deployment.status] || 'gray'}>
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
                      href={output.value} 
                      target="_blank" 
                      class="text-blue-600 hover:text-blue-800 font-medium flex items-center gap-1"
                    >
                      {output.value}
                      <ExternalLink class="w-3 h-3" />
                    </a>
                  {:else}
                    <div class="font-medium text-gray-900 dark:text-white">
                      {output.sensitive ? '••••••••' : output.value}
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
            <Server class="w-5 h-5 text-gray-500" />
          </div>
          
          {#if deployment.status === 'active'}
            <div class="space-y-4">
              <!-- This would be populated with actual resource information from Terraform state -->
              <div class="flex items-center gap-3 p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                <Server class="w-5 h-5 text-blue-600" />
                <div>
                  <div class="font-medium">ECS Cluster</div>
                  <div class="text-sm text-gray-500">{deployment.name}-cluster</div>
                </div>
                <Badge color="green">Running</Badge>
              </div>
              
              <div class="flex items-center gap-3 p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                <Globe class="w-5 h-5 text-green-600" />
                <div>
                  <div class="font-medium">Application Load Balancer</div>
                  <div class="text-sm text-gray-500">{deployment.name}-alb</div>
                </div>
                <Badge color="green">Active</Badge>
              </div>
              
              {#if deployment.configuration?.db_password}
                <div class="flex items-center gap-3 p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                  <Database class="w-5 h-5 text-purple-600" />
                  <div>
                    <div class="font-medium">RDS Database</div>
                    <div class="text-sm text-gray-500">{deployment.name}-db</div>
                  </div>
                  <Badge color="green">Available</Badge>
                </div>
              {/if}
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
            <DollarSign class="w-5 h-5 text-gray-500" />
          </div>
          
          <div class="space-y-4">
            <div class="text-center p-6 bg-green-50 dark:bg-green-900/20 rounded-lg">
              <div class="text-3xl font-bold text-green-600 dark:text-green-400">
                ~$32/month
              </div>
              <div class="text-sm text-green-700 dark:text-green-300 mt-1">
                Estimated monthly cost
              </div>
            </div>
            
            <div class="space-y-3">
              <div class="flex justify-between items-center">
                <span class="text-gray-600">ECS Fargate Compute</span>
                <span class="font-medium">$15.00</span>
              </div>
              <div class="flex justify-between items-center">
                <span class="text-gray-600">Application Load Balancer</span>
                <span class="font-medium">$16.20</span>
              </div>
              <div class="flex justify-between items-center">
                <span class="text-gray-600">Data Transfer</span>
                <span class="font-medium">$0.80</span>
              </div>
              <hr class="my-2">
              <div class="flex justify-between items-center font-semibold">
                <span>Total Estimated</span>
                <span>$32.00/month</span>
              </div>
            </div>
            
            <div class="text-xs text-gray-500 mt-4">
              * Estimates are based on typical usage patterns. Actual costs may vary.
            </div>
          </div>
        </Card>
      </TabItem>
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