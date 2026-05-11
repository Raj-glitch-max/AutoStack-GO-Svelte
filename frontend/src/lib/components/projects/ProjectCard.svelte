<script lang="ts">
  import { goto } from "$app/navigation";
  import type { ProjectsResponse, RolloutsResponse } from "$lib/pocketbase/generated-types";
  import { type Rexpand, deployments } from "$lib/stores/data";
  import { rollouts } from "$lib/stores/data";
  import selectedProjectId from "$lib/stores/project";
  import { formatDateTime, timeAgo } from "$lib/utils/date.utils";
  import { recordLogoUrl } from "$lib/utils/blueprint.utils";
  import { Badge, Button, Indicator, Tooltip } from "flowbite-svelte";
  import { ArrowRight, FileQuestion, Tag } from "lucide-svelte";
  import { getTagColor } from "$lib/utils/tags";
  import { healthSummary } from "$lib/stores/data";
  import type { HealthStatus } from "$lib/types/health";
  import { Activity } from "lucide-svelte";
  export let project: ProjectsResponse;

  let tags: Set<string> = new Set();
  if (project.tags) {
    tags = new Set(project.tags.split(","));
  }

  // filter $rollouts by $rollouts.expand.project
  let these_rollouts: RolloutsResponse<Rexpand>[] = [];
  $: these_rollouts = $rollouts.filter((r) => r.expand?.project.id === project.id);

  // Aggregated health for the project
  let projectHealth: HealthStatus = 'unknown';
  $: {
    const projectDeployments = $deployments.filter(d => d.project === project.id);
    const deploymentIds = new Set(projectDeployments.map(d => d.id));
    const projectHealths = $healthSummary.filter(h => deploymentIds.has(h.deploymentId));
    
    if (projectHealths.length > 0) {
      if (projectHealths.some(h => h.overallStatus === 'unhealthy')) {
        projectHealth = 'unhealthy';
      } else if (projectHealths.some(h => h.overallStatus === 'degraded')) {
        projectHealth = 'degraded';
      } else if (projectHealths.some(h => h.overallStatus === 'pending')) {
        projectHealth = 'pending';
      } else if (projectHealths.every(h => h.overallStatus === 'healthy')) {
        projectHealth = 'healthy';
      }
    }
  }

  function getHealthColor(status: HealthStatus) {
    switch (status) {
      case 'healthy': return 'bg-green-500';
      case 'unhealthy': return 'bg-red-500';
      case 'pending': return 'bg-yellow-500';
      case 'degraded': return 'bg-orange-500';
      default: return 'bg-gray-400';
    }
  }
</script>

<div class="rounded-xl border border-gray-200 ov">
  <div class="flex items-center gap-x-4 border-b border-gray-900/5 p-6">
    <div class="relative">
      {#if project.avatar}
        <img
          src={recordLogoUrl(project)}
          alt="Tuple"
          class="h-12 w-12 flex-none rounded-lg object-cover ring-1 ring-gray-900/10 p-1"
        />
      {:else}
        <FileQuestion class="h-12 w-12 flex-none rounded-lg object-cover p-1" />
      {/if}
      <Indicator
        color="dark"
        size="xl"
        placement="top-right"
        class="text-xs font-bold text-white cursor-default"
        >{$deployments.filter((d) => d.project === project.id).length}
      </Indicator>
      <Tooltip>Deployments</Tooltip>
    </div>
    <div class="flex flex-col min-w-0 flex-1">
      <div class="text-sm font-medium leading-6 truncate flex items-center">
        {project.name}
        <div class="ml-2 flex items-center">
          <div class="w-2 h-2 rounded-full {getHealthColor(projectHealth)} shadow-[0_0_8px_rgba(0,0,0,0.1)] transition-colors duration-500"></div>
          <Tooltip>Project Health: {projectHealth}</Tooltip>
        </div>
      </div>
    </div>
    <div class="relative ml-auto">
      <div class="flex justify-end">
        <Button
          color="alternative"
          on:click={() => {
            $selectedProjectId = project.id;
            goto(`/app/projects/${project.id}`);
          }}
        >
          <ArrowRight class="w-5 h-5" />
        </Button>
      </div>
    </div>
  </div>
  <dl class="-my-3 divide-y divide-gray-100 px-6 py-4 text-sm leading-6">
    <div class="flex justify-between gap-x-4 py-3">
      <dt class="">ID</dt>
      <dd class="flex items-start gap-x-2">
        <span class="cursor-default">{project.id}</span>
      </dd>
    </div>
    <div class="flex justify-between gap-x-4 py-3">
      <dt class="">Last rollout</dt>
      {#if these_rollouts.length > 0}
        <dd class=" cursor-default">
          <time datetime={formatDateTime(these_rollouts[0].startDate)}>
            {timeAgo(these_rollouts[0].startDate)}
          </time>
          <Tooltip>{formatDateTime(these_rollouts[0].startDate)}</Tooltip>
        </dd>
      {:else}
        <dd class=" cursor-default">-</dd>
      {/if}
    </div>
    {#if tags}
      <div class="flex flex-col py-3 space-y-2">
        <dd class="items-start gap-2 flex flex-wrap">
          {#each [...tags] as tag (tag)}
            <Badge color={getTagColor(tag)} class="cursor-default">
              <Tag class="w-3 h-3 inline-block" strokeWidth={2} />&nbsp;{tag.charAt(0) +
                tag.slice(1)}
            </Badge>
          {/each}
        </dd>
      </div>
    {/if}
  </dl>
</div>
