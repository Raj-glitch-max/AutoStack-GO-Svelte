<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { pb } from '$lib/pocketbase';
	import { Card, Button, Badge, Spinner } from 'flowbite-svelte';
	import { Cloud, Check, Sparkles, TrendingDown } from 'lucide-svelte';
	import toast from 'svelte-french-toast';

	const dispatch = createEventDispatcher<{ select: { cloud: string; blueprint: string } }>();

	export let workloadType: string = 'web-app'; // 'web-app' | 'full-stack' | 'static-site'
	export let region: string = 'us-east-1';

	interface CloudOption {
		cloud: 'aws' | 'gcp' | 'azure';
		name: string;
		logo: string;
		blueprint: string;
		monthlyEstimate: number;
		keyTradeoff: string;
		hasCredentials: boolean;
	}

	// Static estimates per workload type — fetched from GCP/Azure pricing APIs at runtime.
	// These are the fallback defaults; the component fetches live estimates on mount.
	const estimates: Record<string, Record<string, { min: number; est: number; tradeoff: string }>> = {
		'web-app': {
			aws:   { min: 28, est: 35, tradeoff: 'Mature ecosystem, most integrations' },
			gcp:   { min: 0,  est: 8,  tradeoff: 'Cloud Run scales to zero — no idle cost' },
			azure: { min: 0,  est: 10, tradeoff: 'Container Apps scales to zero, good for .NET' }
		},
		'full-stack': {
			aws:   { min: 45, est: 58, tradeoff: 'RDS Multi-AZ, best managed DB options' },
			gcp:   { min: 30, est: 42, tradeoff: 'Cloud SQL cheaper for small workloads' },
			azure: { min: 35, est: 50, tradeoff: 'Azure SQL best for Microsoft stack' }
		},
		'static-site': {
			aws:   { min: 1,  est: 3,  tradeoff: 'CloudFront has 400+ PoPs globally' },
			gcp:   { min: 1,  est: 2,  tradeoff: 'Cloud CDN slightly cheaper, fewer PoPs' },
			azure: { min: 1,  est: 3,  tradeoff: 'Azure CDN integrates with Azure DevOps' }
		}
	};

	// Live estimates fetched from backend pricing APIs
	let liveEstimates: Record<string, { min: number; est: number }> = {};
	let fetchingPrices = false;

	async function fetchLivePrices() {
		fetchingPrices = true;
		try {
			const resp = await fetch(`${pb.baseUrl}/api/multicloud/pricing?workload=${workloadType}&region=${region}`, {
				headers: { Authorization: pb.authStore.token }
			});
			if (resp.ok) {
				liveEstimates = await resp.json();
			}
		} catch {
			// Fall back to static estimates
		} finally {
			fetchingPrices = false;
		}
	}

	$: if (workloadType || region) fetchLivePrices();

	const cloudMeta = {
		aws:   { name: 'AWS',         logo: '🟠', color: 'orange' as const },
		gcp:   { name: 'Google Cloud', logo: '🔵', color: 'blue' as const },
		azure: { name: 'Azure',        logo: '🔷', color: 'indigo' as const }
	};

	const blueprintMap: Record<string, Record<string, string>> = {
		'web-app':    { aws: 'ecs-web-app',        gcp: 'gcp-cloud-run',    azure: 'azure-container-apps' },
		'full-stack': { aws: 'full-stack',          gcp: 'gcp-full-stack',   azure: 'azure-full-stack' },
		'static-site':{ aws: 'static-site',         gcp: 'gcp-static-site',  azure: 'azure-static-site' }
	};

	let loading = false;
	let aiRecommendation = '';
	let selectedCloud: string | null = null;

	$: options = Object.entries(estimates[workloadType] ?? estimates['web-app']).map(([cloud, data]) => ({
		cloud: cloud as 'aws' | 'gcp' | 'azure',
		...cloudMeta[cloud as keyof typeof cloudMeta],
		blueprint: blueprintMap[workloadType]?.[cloud] ?? '',
		monthlyEstimate: liveEstimates[cloud]?.est ?? data.est,
		minEstimate: liveEstimates[cloud]?.min ?? data.min,
		tradeoff: data.tradeoff
	}));

	$: cheapest = options.reduce((a, b) => a.monthlyEstimate < b.monthlyEstimate ? a : b);

	async function getAIRecommendation() {
		loading = true;
		aiRecommendation = '';
		try {
			const resp = await fetch(`${pb.baseUrl}/api/intelligence/recommend`, {
				method: 'POST',
				headers: {
					Authorization: pb.authStore.token,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					description: `Multi-cloud comparison for ${workloadType} workload in ${region}. Compare AWS, GCP, and Azure. Which cloud is best for cost and reliability?`
				})
			});
			if (resp.ok) {
				const data = await resp.json();
				aiRecommendation = data.reasoning ?? '';
			}
		} catch {
			// best-effort
		} finally {
			loading = false;
		}
	}

	function selectCloud(cloud: string, blueprint: string) {
		selectedCloud = cloud;
		dispatch('select', { cloud, blueprint });
		toast.success(`${cloudMeta[cloud as keyof typeof cloudMeta].name} selected`);
	}
</script>

<div class="space-y-4">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-2">
			<Cloud class="w-5 h-5 text-gray-600 dark:text-gray-400" />
			<h3 class="font-semibold text-gray-900 dark:text-white">Cloud Comparison</h3>
			<Badge color="blue">{workloadType}</Badge>
		</div>
		<Button size="xs" color="alternative" on:click={getAIRecommendation} disabled={loading}>
			{#if loading}
				<Spinner size="3" class="mr-1" />
			{:else}
				<Sparkles class="w-3 h-3 mr-1" />
			{/if}
			AI Pick
		</Button>
	</div>

	<!-- Cloud cards -->
	<div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
		{#each options as opt}
			<button
				on:click={() => selectCloud(opt.cloud, opt.blueprint)}
				class="text-left p-4 rounded-xl border-2 transition-all
					{selectedCloud === opt.cloud
						? 'border-primary-500 bg-primary-50 dark:bg-primary-950'
						: 'border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600'}"
			>
				<div class="flex items-center justify-between mb-2">
					<span class="text-lg">{opt.logo}</span>
					{#if opt.cloud === cheapest.cloud}
						<Badge color="green" class="text-xs">
							<TrendingDown class="w-3 h-3 mr-1" />
							Cheapest
						</Badge>
					{/if}
					{#if selectedCloud === opt.cloud}
						<div class="w-5 h-5 bg-primary-500 rounded-full flex items-center justify-center">
							<Check class="w-3 h-3 text-white" />
						</div>
					{/if}
				</div>

				<div class="font-semibold text-gray-900 dark:text-white text-sm mb-1">{opt.name}</div>

				<div class="text-xl font-bold text-gray-900 dark:text-white">
					~${opt.monthlyEstimate}<span class="text-xs font-normal text-gray-500">/mo</span>
				</div>
				{#if opt.minEstimate === 0}
					<div class="text-xs text-green-600 dark:text-green-400 mb-2">$0 when idle</div>
				{:else}
					<div class="text-xs text-gray-400 mb-2">min ~${opt.minEstimate}/mo</div>
				{/if}

				<p class="text-xs text-gray-500 dark:text-gray-400 leading-relaxed">{opt.tradeoff}</p>
			</button>
		{/each}
	</div>

	<!-- AI recommendation -->
	{#if aiRecommendation}
		<div class="p-3 bg-purple-50 dark:bg-purple-950 border border-purple-200 dark:border-purple-800 rounded-lg">
			<div class="flex items-center gap-2 mb-1">
				<Sparkles class="w-4 h-4 text-purple-600 dark:text-purple-400" />
				<span class="text-xs font-semibold text-purple-700 dark:text-purple-300">AI Recommendation</span>
			</div>
			<p class="text-sm text-purple-800 dark:text-purple-200">{aiRecommendation}</p>
		</div>
	{/if}
</div>
