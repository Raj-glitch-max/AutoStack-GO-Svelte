<script lang="ts">
  import { client } from "$lib/pocketbase";
  import type {
    BlueprintsResponse,
    RolloutsRecord,
    DeploymentsRecord
  } from "$lib/pocketbase/generated-types";
  import { UpdateFilterEnum, blueprints, updateDataStores } from "$lib/stores/data";
  import selectedProjectId from "$lib/stores/project";
  import { recordLogoUrl } from "$lib/utils/blueprint.utils";

  import DeploymentTargetSelector from "../aws/DeploymentTargetSelector.svelte";
  import NewAWSDeployment from "../aws/NewAWSDeployment.svelte";
  import { Button, Input, Label } from "flowbite-svelte";
  import { ArrowRight, BookLock, BookUser } from "lucide-svelte";
  import toast from "svelte-french-toast";
  import { onMount } from "svelte";

  export let modal: boolean;

  let deploymentTarget: 'kubernetes' | 'aws' = 'kubernetes';
  let name: string = "";

  let filteredBlueprints: BlueprintsResponse[] = [];
  let selectedBlueprint: BlueprintsResponse;
  
  // AWS blueprints
  let awsBlueprints: any[] = [];
  let loadingAwsBlueprints = false;

  // Temporary hardcoded blueprints for testing
  const tempKubernetesBlueprints = [
    {
      id: "temp_nginx",
      name: "NGINX Web Server",
      description: "Simple NGINX web server deployment",
      manifest: {
        apiVersion: "v1",
        kind: "List",
        items: [
          {
            apiVersion: "apps/v1",
            kind: "Deployment",
            metadata: {
              name: "nginx-deployment",
              labels: { app: "nginx" }
            },
            spec: {
              replicas: 2,
              selector: { matchLabels: { app: "nginx" } },
              template: {
                metadata: { labels: { app: "nginx" } },
                spec: {
                  containers: [{
                    name: "nginx",
                    image: "nginx:latest",
                    ports: [{ containerPort: 80 }]
                  }]
                }
              }
            }
          },
          {
            apiVersion: "v1",
            kind: "Service",
            metadata: { name: "nginx-service" },
            spec: {
              selector: { app: "nginx" },
              ports: [{ protocol: "TCP", port: 80, targetPort: 80 }],
              type: "ClusterIP"
            }
          }
        ]
      },
      owner: "system",
      public: true,
      expand: { owner: { id: "system", name: "System" } }
    },
    {
      id: "temp_nodejs",
      name: "Node.js Application", 
      description: "Node.js application with deployment and service",
      manifest: {
        apiVersion: "v1",
        kind: "List",
        items: [
          {
            apiVersion: "apps/v1",
            kind: "Deployment",
            metadata: {
              name: "nodejs-app",
              labels: { app: "nodejs" }
            },
            spec: {
              replicas: 2,
              selector: { matchLabels: { app: "nodejs" } },
              template: {
                metadata: { labels: { app: "nodejs" } },
                spec: {
                  containers: [{
                    name: "nodejs",
                    image: "node:18-alpine",
                    ports: [{ containerPort: 3000 }],
                    env: [
                      { name: "NODE_ENV", value: "production" },
                      { name: "PORT", value: "3000" }
                    ]
                  }]
                }
              }
            }
          },
          {
            apiVersion: "v1",
            kind: "Service",
            metadata: { name: "nodejs-service" },
            spec: {
              selector: { app: "nodejs" },
              ports: [{ protocol: "TCP", port: 80, targetPort: 3000 }],
              type: "LoadBalancer"
            }
          }
        ]
      },
      owner: "system",
      public: true,
      expand: { owner: { id: "system", name: "System" } }
    }
  ];

  $: filteredBlueprints = ($blueprints.length > 0 
    ? $blueprints.filter(
        (blueprint: any) => blueprint.expand?.owner?.id === client.authStore.record?.id || (blueprint as any).public === true
      )
    : tempKubernetesBlueprints) as BlueprintsResponse[];
  $: selectedBlueprint = filteredBlueprints[0];

  // Load AWS blueprints when AWS target is selected
  $: if (deploymentTarget === 'aws') {
    loadAwsBlueprints();
  }

  async function loadAwsBlueprints() {
    if (awsBlueprints.length > 0) return; // Already loaded
    
    loadingAwsBlueprints = true;
    try {
      const response = await client.collection('awsBlueprints').getFullList({
        sort: '-created'
      });
      awsBlueprints = response;
    } catch (error) {
      console.error('Failed to load AWS blueprints:', error);
      toast.error('Failed to load AWS blueprints');
    } finally {
      loadingAwsBlueprints = false;
    }
  }

  async function handleCreateKubernetesDeployment(event: Event) {
    event.preventDefault();

    if (!name) {
      toast.error("Please enter a deployment name");
      return;
    }

    if (!$selectedProjectId) {
      toast.error("Please select a project first");
      return;
    }

    if (!selectedBlueprint || !selectedBlueprint.manifest) {
      toast.error("Please select a valid blueprint");
      return;
    }

    // Check if this is a temporary blueprint (not from database)
    const isTemporaryBlueprint = selectedBlueprint.id.startsWith('temp_');
    
    let deploymentData: DeploymentsRecord;
    
    if (isTemporaryBlueprint) {
      // For temporary blueprints, we'll store the blueprint data directly in the deployment
      deploymentData = {
        name: name,
        blueprint: "", // No blueprint reference for temporary ones
        user: client.authStore?.record?.id ?? "",
        project: $selectedProjectId,
        // Store blueprint info in a custom field if available, or handle differently
      };
    } else {
      deploymentData = {
        name: name,
        blueprint: selectedBlueprint.id,
        user: client.authStore?.record?.id ?? "",
        project: $selectedProjectId
      };
    }

    // Clean up manifest - remove any ingresses from the blueprint manifest
    let cleanManifest: any = selectedBlueprint?.manifest ? { ...selectedBlueprint.manifest } : {};
    
    // Safely check for interfaces and remove ingress objects
    try {
      if (cleanManifest && typeof cleanManifest === 'object') {
        // Check if it's an AutoStack-style manifest with spec.interfaces
        if ((cleanManifest as any).spec && (cleanManifest as any).spec.interfaces && Array.isArray((cleanManifest as any).spec.interfaces)) {
          for (let i = 0; i < (cleanManifest as any).spec.interfaces.length; i++) {
            if ((cleanManifest as any).spec.interfaces[i] && (cleanManifest as any).spec.interfaces[i].ingress) {
              delete (cleanManifest as any).spec.interfaces[i].ingress;
            }
          }
        }
        
        // Check if it's a Kubernetes List with items that might have ingress
        if ((cleanManifest as any).kind === 'List' && (cleanManifest as any).items && Array.isArray((cleanManifest as any).items)) {
          (cleanManifest as any).items = (cleanManifest as any).items.filter((item: any) => 
            item && item.kind !== 'Ingress'
          );
        }
      }
    } catch (error) {
      console.warn('Error cleaning manifest:', error);
      // Continue with original manifest if cleaning fails
      cleanManifest = selectedBlueprint?.manifest || {};
    }

    try {
      const deploymentResponse = await client.collection("deployments").create(deploymentData);
      
      // Create initial rollout
      const rollout: RolloutsRecord = {
        manifest: cleanManifest,
        startDate: "",
        endDate: "",
        project: $selectedProjectId,
        deployment: deploymentResponse.id,
        user: client.authStore?.record?.id ?? ""
      };

      await client.collection("rollouts").create(rollout);
      
      toast.success("Kubernetes Deployment & Rollout created successfully!");
      
      // Update data stores
      updateDataStores({
        filter: UpdateFilterEnum.ALL,
        projectId: $selectedProjectId
      });
      
      // Close modal and reset form
      modal = false;
      name = "";
      
    } catch (error: any) {
      console.error("Deployment creation failed:", error);
      toast.error(`Failed to create deployment: ${error.message || 'Unknown error'}`);
    }
  }
</script>

<div class="flex flex-col space-y-6">
  <!-- Deployment Target Selector -->
  <DeploymentTargetSelector bind:selectedTarget={deploymentTarget} />
  
  {#if deploymentTarget === 'kubernetes'}
    <!-- Kubernetes Deployment Form -->
    <div class="border-t pt-6">
      <h3 class="mb-4 text-xl font-medium text-gray-900 dark:text-gray-400">
        Create Kubernetes Deployment
      </h3>
      
      <div class="space-y-6">
        <Label class="space-y-2">
          <span>Deployment name *</span>
          <Input
            type="text"
            name="deployment"
            placeholder="Enter the name of your deployment"
            required
            bind:value={name}
          />
        </Label>
        
        <fieldset class="space-y-2">
          <Label class="space-y-2">
            <span>Select a blueprint *</span>
          </Label>
          <div class="grid grid-cols-2 gap-2">
            {#if filteredBlueprints && filteredBlueprints.length > 0}
              {#each filteredBlueprints as blueprint (blueprint.id)}
                <!-- svelte-ignore a11y-click-events-have-key-events -->
                <!-- svelte-ignore a11y-no-static-element-interactions -->
                <span
                  class="cursor-pointer w-full rounded-lg px-6 py-4 sm:flex sm:justify-between border-2
                {selectedBlueprint?.id === blueprint?.id
                    ? 'border-primary-600 bg-gray-50 dark:bg-transparent'
                    : ' border-gray-200'}
                "
                  on:click={() => {
                    selectedBlueprint = blueprint;
                  }}
                >
                  <input
                    type="radio"
                    name="server-size"
                    value={blueprint?.id}
                    class="sr-only"
                    aria-labelledby="server-size-1-label"
                    aria-describedby="server-size-1-description-0 server-size-1-description-1"
                  />
                  <span class="flex items-center">
                    <span class="flex flex-col text-sm">
                      <span id="server-size-1-label" class="font-medium">
                        {blueprint?.name}
                      </span>

                      <span id="server-size-1-description-0" class=" hover:text-gray-600 mt-1">
                        {#if blueprint?.expand?.owner?.id === client.authStore.record?.id}
                          <BookLock class="w-4 h-4 mr-1 inline-block" />
                        {:else}
                          <BookUser class="w-4 h-4 mr-1 inline-block" />
                        {/if}
                        <p class="block sm:inline">
                          {blueprint?.description}
                        </p>
                      </span>
                    </span>
                  </span>
                  <span
                    id="server-size-1-description-1"
                    class="mt-2 flex text-sm sm:ml-4 sm:mt-0 sm:flex-col sm:text-right"
                  >
                    <img
                      src={recordLogoUrl(blueprint)}
                      alt={blueprint?.name}
                      class="h-12 w-12 flex-none rounded-lg object-cover ring-1 ring-gray-900/10"
                    />
                  </span>
                  <span
                    class="pointer-events-none absolute -inset-px rounded-lg border-2"
                    aria-hidden="true"
                  ></span>
                </span>
              {/each}
            {:else}
              <div class="col-span-2 text-center py-8 text-gray-500 dark:text-gray-400">
                <p class="mb-2">No Kubernetes blueprints available yet.</p>
                <p class="text-sm">Blueprints loaded: {$blueprints.length}</p>
                <p class="text-sm">Filtered blueprints: {filteredBlueprints.length}</p>
                <p class="text-sm mb-4">Create your own blueprint or check back later for system blueprints.</p>
                <Button 
                  size="sm" 
                  color="alternative"
                  on:click={() => {
                    updateDataStores({
                      filter: UpdateFilterEnum.ALL,
                      projectId: $selectedProjectId
                    });
                  }}
                >
                  Refresh Blueprints
                </Button>
              </div>
            {/if}
          </div>
        </fieldset>
        
        {#if !$selectedProjectId}
          <div class="mb-4 p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
            <p class="text-sm text-yellow-800 dark:text-yellow-200">
              ⚠️ Please create and select a project first before creating deployments.
            </p>
          </div>
        {/if}
        
        <Button 
          type="submit" 
          class="w-full" 
          color="primary" 
          disabled={!selectedBlueprint || filteredBlueprints.length === 0 || !name.trim() || !$selectedProjectId}
          on:click={handleCreateKubernetesDeployment}
        >
          {#if !$selectedProjectId}
            Select Project First
          {:else if !name.trim()}
            Enter Deployment Name
          {:else if !selectedBlueprint}
            Select Blueprint
          {:else}
            Create Kubernetes Deployment
          {/if}
          <ArrowRight class="w-4 h-4 ml-2 inline-block" />
        </Button>
      </div>
    </div>
    
  {:else if deploymentTarget === 'aws'}
    <!-- AWS Deployment Form -->
    <div class="border-t pt-6">
      <NewAWSDeployment bind:modal projectId={$selectedProjectId} />
    </div>
  {/if}
</div>