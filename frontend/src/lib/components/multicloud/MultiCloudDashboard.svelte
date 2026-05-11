<script lang="ts">
	import { onMount } from 'svelte';
	import { pb } from '$lib/pocketbase';
	import { Card, Badge, Spinner, Button } from 'flowbite-svelte';
	import { Cloud, RefreshCw, CheckCircle, AlertTriangle, XCircle, Clock } from 'lucide-svelte';

	interface CloudDeployment {
		id: string;
		name: string;
		cloud: 'aws' | 'gcp' | 'azure' | 'k8s';
		status: string;
		region: string;
		created: string;
		monthlyCost?: number;
	}

	let deployments: CloudDeployment[] = [];
	let loading = true;

	const cloudMeta = {
		aws:   { label: 'AWS',          logo: '🟠', color: 'orange' as const },
		gcp:   { label: 'Google Cloud', logo: '🔵', color: 'blue' as const },
		azure: { label: 'Azure',        logo: '🔷', color: 'indigo' as const },
		k8s:   { label: 'Kubernetes',   logo: '⚙️',  color: 'green' as const }
	};

	const statusMeta: Record<string, { color: 'green' | 'yellow' | 'red' | 'dark'; icon: typeof CheckCircle }> = {
		active:     { color: 'green',  icon: CheckCircle },
		running:    { color: 'green',  icon: CheckCircle },
		deploying:  { color: 'yellow', icon: Clock },
		applying:   { color: 'yellow', icon: Clock },
		planning:   { color: 'yellow', icon: Clock },
		failed:     { color: 'red',    icon: XCircle },
		destroyed:  { color: 'dark',   icon: XCircle },
		degraded:   { color: 'red',    icon: AlertTriangle }
	};

	function getStatusMeta(status: string) {
		return statusMeta[status] ?? { color: 'dark' as const, icon: Clock };
	}

	async function loadDeployments() {
		loading = true;
		const all: CloudDeployment[] = [];

		try {
			// AWS deployments
			const awsResp = await fetch(`${pb.baseUrl}/api/aws/deployments`, {
				headers: { Authorization: pb.authStore.token }
			});
			if (awsResp.ok) {
				const awsData = await awsResp.json();
				const list = Array.isArray(awsData) ? awsData : (awsData.items ?? []);
				list.forEach((d: any) => all.push({
					id: d.id, name: d.name, cloud: 'aws',
					status: d.status, region: d.region, created: d.created
				}));
			}
		} catch { /* AWS not configured */ }

		try {
			// Kubernetes deployments
			const k8sResp = await fetch(`${pb.baseUrl}/api/collections/deployments/records`, {
				headers: { Authorization: pb.authStore.token }
			});
			if (k8sResp.ok) {
				const k8sData = await k8sResp.json();
				(k8sData.items ?? []).forEach((d: any) => all.push({
					id: d.id, name: d.name, cloud: 'k8s',
					status: d.status ?? 'running', region: 'cluster', created: d.created
				}));
			}
		} catch { /* K8s not configured */ }

		deployments = all.sort((a, b) => new Date(b.created).getTime() - new Date(a.created).getTime());
		loading = false;
	}

	$: cloudCounts = deployments.reduce((acc, d) => {
		acc[d.cloud] = (acc[d.cloud] ?? 0) + 1;
		return acc;
	}, {} as Record<string, number>);

	$: activeCount = deployments.filter(d => d.status === 'active' || d.status === 'running').length;

	function getCloudColor(cloud: string): 'yellow' | 'blue' | 'indigo' | 'green' | 'dark' {
		const map: Record<string, 'yellow' | 'blue' | 'indigo' | 'green' | 'dark'> = {
			aws: 'yellow', gcp: 'blue', azure: 'indigo', k8s: 'green'
		};
		return map[cloud] ?? 'dark';
	}

	function getCloudMeta(cloud: string) {
		return cloudMeta[cloud as keyof typeof cloudMeta] ?? { label: cloud, logo: '☁️', color: 'dark' as const };
	}

	onMount(() => loadDeployments());
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-3">
			<div class="p-2 bg-blue-100 dark:bg-blue-900 rounded-lg">
				<Cloud class="w-5 h-5 text-blue-600 dark:text-blue-400" />
			</div>
			<div>
				<h2 class="text-xl font-bold text-gray-900 dark:text-white">Multi-Cloud Overview</h2>
				<p class="text-sm text-gray-500 dark:text-gray-400">
					{deployments.length} deployments · {activeCount} active
				</p>
			</div>
		</div>
		<Button size="sm" color="alternative" on:click={loadDeployments} disabled={loading}>
			<RefreshCw class="w-4 h-4 {loading ? 'animate-spin' : ''}" />
		</Button>
	</div>

	<!-- Cloud summary pills -->
	<div class="flex flex-wrap gap-2">
		{#each Object.entries(cloudCounts) as [cloud, count]}
			<div class="flex items-center gap-2 px-3 py-1.5 bg-gray-100 dark:bg-gray-800 rounded-full text-sm">
				<span>{getCloudMeta(cloud).logo}</span>
				<span class="font-medium text-gray-700 dark:text-gray-300">{getCloudMeta(cloud).label}</span>
				<Badge color={getCloudColor(cloud)}>{count}</Badge>
			</div>
		{/each}
		{#if deployments.length === 0 && !loading}
			<p class="text-sm text-gray-500 dark:text-gray-400">No deployments yet</p>
		{/if}
	</div>

	<!-- Deployments list -->
	{#if loading}
		<div class="flex justify-center py-8">
			<Spinner size="8" />
		</div>
	{:else}
		<div class="space-y-2">
			{#each deployments as dep}
				{@const status = getStatusMeta(dep.status)}
				{@const StatusIcon = status.icon}
				<Card class="p-3">
					<div class="flex items-center gap-3">
						<span class="text-xl">{getCloudMeta(dep.cloud).logo}</span>
						<div class="flex-1 min-w-0">
							<div class="flex items-center gap-2">
								<span class="font-medium text-gray-900 dark:text-white truncate">{dep.name}</span>
								<Badge color={getCloudColor(dep.cloud)} class="shrink-0 text-xs">{getCloudMeta(dep.cloud).label}</Badge>
							</div>
							<div class="text-xs text-gray-500 dark:text-gray-400">
								{dep.region} · {new Date(dep.created).toLocaleDateString()}
							</div>
						</div>
						<div class="flex items-center gap-1.5 shrink-0">
							<StatusIcon class="w-4 h-4" />
							<Badge color={status.color}>{dep.status}</Badge>
						</div>
					</div>
				</Card>
			{/each}
		</div>
	{/if}
</div>
