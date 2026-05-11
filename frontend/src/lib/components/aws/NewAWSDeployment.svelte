<script lang="ts">
  import { Button, Input, Label, Select } from "flowbite-svelte";
  import { ArrowRight, Cloud, DollarSign, Sparkles } from "lucide-svelte";
  import toast from "svelte-french-toast";
  import DeploymentAdvisor from "$lib/components/intelligence/DeploymentAdvisor.svelte";
  import { fade, slide } from "svelte/transition";
  
  export let modal: boolean;
  export let projectId: string;
  
  let deploymentName = '';
  let selectedRegion = 'us-east-1';
  let selectedBlueprint = 'ecs-web-app';
  let containerImage = 'nginx:latest';
  let instanceType = '256';
  let estimatedCost = 0;
  let customDomain = '';
  let route53ZoneId = '';
  let showAdvanced = false;
  let useAI = false;
  let aiRecommendation: any = null;
  
  const regions = [
    { value: 'us-east-1', name: 'US East (N. Virginia)' },
    { value: 'us-east-2', name: 'US East (Ohio)' },
    { value: 'us-west-1', name: 'US West (N. California)' },
    { value: 'us-west-2', name: 'US West (Oregon)' },
    { value: 'eu-west-1', name: 'Europe (Ireland)' },
    { value: 'eu-central-1', name: 'Europe (Frankfurt)' },
    { value: 'ap-southeast-1', name: 'Asia Pacific (Singapore)' },
    { value: 'ap-northeast-1', name: 'Asia Pacific (Tokyo)' }
  ];
  
  const blueprints = [
    { 
      value: 'ecs-web-app', 
      name: 'ECS Web Application',
      description: 'Simple web app with load balancer',
      baseCost: 25
    },
    { 
      value: 'full-stack', 
      name: 'Full Stack App',
      description: 'Web app with RDS database',
      baseCost: 45
    },
    { 
      value: 'static-site', 
      name: 'Static Website',
      description: 'S3 + CloudFront distribution',
      baseCost: 5
    }
  ];
  
  const instanceTypes = [
    { value: '256', name: '0.25 vCPU, 0.5 GB RAM', cost: 15 },
    { value: '512', name: '0.5 vCPU, 1 GB RAM', cost: 25 },
    { value: '1024', name: '1 vCPU, 2 GB RAM', cost: 45 }
  ];
  
  // Calculate estimated cost
  $: {
    const blueprint = blueprints.find(b => b.value === selectedBlueprint);
    const instance = instanceTypes.find(i => i.value === instanceType);
    estimatedCost = (blueprint?.baseCost || 0) + (instance?.cost || 0);
  }

  function handleAIApply(event: CustomEvent) {
    const rec = event.detail;
    selectedBlueprint = rec.blueprint_id;
    selectedRegion = rec.region;
    
    // Map instance size strings to values if needed
    if (rec.instance_size.includes('0.25')) instanceType = '256';
    else if (rec.instance_size.includes('0.5')) instanceType = '512';
    else if (rec.instance_size.includes('1')) instanceType = '1024';
    
    aiRecommendation = rec;
    useAI = false;
    toast.success("AI recommendations applied!");
  }
  
  async function handleCreateDeployment() {
    if (!deploymentName.trim()) {
      toast.error("Please enter a deployment name");
      return;
    }
    
    if (!containerImage.trim()) {
      toast.error("Please enter a container image");
      return;
    }
    
    try {
      const response = await fetch('/api/aws/deployments', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          name: deploymentName,
          project: projectId,
          blueprint: selectedBlueprint,
          region: selectedRegion,
          configuration: {
            container_image: containerImage,
            instance_type: instanceType,
            app_name: deploymentName.toLowerCase().replace(/[^a-z0-9-]/g, '-'),
            custom_domain: customDomain,
            route53_zone_id: route53ZoneId
          }
        })
      });
      
      if (!response.ok) {
        throw new Error('Failed to create deployment');
      }
      
      const result = await response.json();
      toast.success("AWS deployment created successfully!");
      modal = false;
      
      // Reset form
      deploymentName = '';
      containerImage = 'nginx:latest';
      selectedRegion = 'us-east-1';
      selectedBlueprint = 'ecs-web-app';
      instanceType = '256';
      
    } catch (error) {
      console.error('Error creating deployment:', error);
      toast.error("Failed to create deployment. Please try again.");
    }
  }
</script>

<div class="space-y-6">
  <div class="flex items-center justify-between">
    <div class="flex items-center space-x-3">
      <Cloud size={32} class="text-primary-600 dark:text-primary-400" />
      <div>
        <h3 class="text-xl font-bold dark:text-white">New AWS Deployment</h3>
        <p class="text-sm text-gray-500 dark:text-gray-400">Launch production-grade infrastructure in minutes</p>
      </div>
    </div>
    <Button 
      outline={!useAI} 
      color={useAI ? 'primary' : 'alternative'} 
      size="xs" 
      on:click={() => useAI = !useAI}
    >
      <Sparkles size={14} class="mr-1" />
      {useAI ? 'Back to Manual' : 'Get AI Help'}
    </Button>
  </div>

  {#if useAI}
    <div transition:fade>
      <DeploymentAdvisor on:apply={handleAIApply} />
    </div>
  {:else}
    <div class="grid grid-cols-1 md:grid-cols-2 gap-6" transition:fade>
      <div class="space-y-4">
  <!-- Deployment Name -->
  <Label class="space-y-2">
    <span>Deployment Name *</span>
    <Input
      type="text"
      placeholder="my-awesome-app"
      bind:value={deploymentName}
      required
    />
  </Label>
  
  <!-- Container Image -->
  <Label class="space-y-2">
    <span>Container Image *</span>
    <Input
      type="text"
      placeholder="nginx:latest"
      bind:value={containerImage}
      required
    />
    <p class="text-sm text-gray-500">
      Docker image from Docker Hub or ECR
    </p>
  </Label>
  
  <!-- Region Selection -->
  <Label class="space-y-2">
    <span>AWS Region *</span>
    <Select bind:value={selectedRegion}>
      {#each regions as region}
        <option value={region.value}>{region.name}</option>
      {/each}
    </Select>
  </Label>
  
  <!-- Blueprint Selection -->
  <fieldset class="space-y-3">
    <Label>
      <span>Infrastructure Blueprint *</span>
    </Label>
    <div class="space-y-2">
      {#each blueprints as blueprint}
        <label class="flex items-start space-x-3 p-3 border rounded-lg cursor-pointer
                     {selectedBlueprint === blueprint.value 
                       ? 'border-primary-600 bg-primary-50 dark:bg-primary-900/20' 
                       : 'border-gray-200 dark:border-gray-700'}">
          <input
            type="radio"
            bind:group={selectedBlueprint}
            value={blueprint.value}
            class="mt-1"
          />
          <div class="flex-1">
            <div class="font-medium text-gray-900 dark:text-white">
              {blueprint.name}
            </div>
            <div class="text-sm text-gray-600 dark:text-gray-400">
              {blueprint.description}
            </div>
            <div class="text-xs text-green-600 dark:text-green-400 mt-1">
              Base cost: ~${blueprint.baseCost}/month
            </div>
          </div>
        </label>
      {/each}
    </div>
  </fieldset>
  
  <!-- Instance Type -->
  <Label class="space-y-2">
    <span>Instance Size</span>
    <Select bind:value={instanceType}>
      {#each instanceTypes as instance}
        <option value={instance.value}>
          {instance.name} (~${instance.cost}/month)
        </option>
      {/each}
    </Select>
  </Label>

  <!-- Advanced Settings / Custom Domain -->
  <div class="border-t dark:border-gray-700 pt-4 mt-4">
    <button 
      class="text-sm font-medium text-primary-600 hover:text-primary-700 flex items-center"
      on:click={() => showAdvanced = !showAdvanced}
    >
      {showAdvanced ? 'Hide' : 'Show'} Advanced Settings (Custom Domain)
    </button>
    
    {#if showAdvanced}
      <div class="mt-4 space-y-4">
        <Label class="space-y-2">
          <span>Custom Domain (Optional)</span>
          <Input
            type="text"
            placeholder="app.example.com"
            bind:value={customDomain}
          />
          <p class="text-xs text-gray-500">
            Automated SSL/TLS certificate will be provisioned.
          </p>
        </Label>
        
        <Label class="space-y-2">
          <span>Route53 Hosted Zone ID</span>
          <Input
            type="text"
            placeholder="Z0123456789ABCDEF"
            bind:value={route53ZoneId}
          />
          <p class="text-xs text-gray-500">
            Required for DNS validation and alias record.
          </p>
        </Label>
      </div>
    {/if}
  </div>
  
  <!-- Cost Estimate -->
  <div class="bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg p-4">
    <div class="flex items-center space-x-2">
      <DollarSign class="w-5 h-5 text-green-600" />
      <span class="font-medium text-green-800 dark:text-green-200">
        Estimated Monthly Cost
      </span>
    </div>
    <div class="text-2xl font-bold text-green-600 dark:text-green-400 mt-1">
      ~${estimatedCost}/month
    </div>
    <p class="text-sm text-green-700 dark:text-green-300 mt-1">
      Estimate includes compute, storage, and networking. Actual costs may vary.
    </p>
  </div>
  
  <!-- Deploy Button -->
  <Button 
    type="submit" 
    class="w-full" 
    color="primary" 
    on:click={handleCreateDeployment}
  >
    Deploy to AWS
    <ArrowRight class="w-4 h-4 ml-2" />
  </Button>
  </div>
  </div>
{/if}
</div>