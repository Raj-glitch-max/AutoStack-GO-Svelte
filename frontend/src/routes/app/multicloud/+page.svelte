<script lang="ts">
	import { Breadcrumb, BreadcrumbItem, Tabs, TabItem } from 'flowbite-svelte';
	import { Home, Cloud } from 'lucide-svelte';
	import MultiCloudDashboard from '$lib/components/multicloud/MultiCloudDashboard.svelte';
	import CloudComparisonCard from '$lib/components/multicloud/CloudComparisonCard.svelte';
	import GCPCredentialsForm from '$lib/components/multicloud/GCPCredentialsForm.svelte';
	import AzureCredentialsForm from '$lib/components/multicloud/AzureCredentialsForm.svelte';
</script>

<svelte:head>
	<title>Multi-Cloud - AutoStack</title>
</svelte:head>

<div class="p-6 space-y-6">
	<Breadcrumb>
		<BreadcrumbItem href="/app" home><Home class="w-4 h-4" /></BreadcrumbItem>
		<BreadcrumbItem><Cloud class="w-4 h-4 mr-2" />Multi-Cloud</BreadcrumbItem>
	</Breadcrumb>

	<Tabs>
		<TabItem open title="Overview">
			<MultiCloudDashboard />
		</TabItem>

		<TabItem title="Compare Clouds">
			<div class="space-y-4 pt-4">
				<p class="text-sm text-gray-600 dark:text-gray-400">
					Compare estimated costs across AWS, GCP, and Azure before deploying.
				</p>
				<CloudComparisonCard workloadType="web-app" />
				<CloudComparisonCard workloadType="full-stack" />
				<CloudComparisonCard workloadType="static-site" />
			</div>
		</TabItem>

		<TabItem title="GCP Setup">
			<div class="pt-4 max-w-xl">
				<GCPCredentialsForm />
			</div>
		</TabItem>

		<TabItem title="Azure Setup">
			<div class="pt-4 max-w-xl">
				<AzureCredentialsForm />
			</div>
		</TabItem>
	</Tabs>
</div>
