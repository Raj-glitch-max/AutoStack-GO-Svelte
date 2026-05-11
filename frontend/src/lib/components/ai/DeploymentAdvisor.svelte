<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { pb } from '$lib/pocketbase';
	import { Button, Spinner, Badge, Card, Textarea } from 'flowbite-svelte';
	import {
		Sparkles,
		ChevronRight,
		Check,
		RefreshCw,
		ThumbsUp,
		ThumbsDown,
		Server,
		Globe,
		Database,
		Zap
	} from 'lucide-svelte';
	import toast from 'svelte-french-toast';

	const dispatch = createEventDispatcher<{
		accept: { blueprintId: string; region: string; instanceSize: string };
	}>();

	interface Recommendation {
		blueprint_id: string;
		region: string;
		instance_size: string;
		reasoning: string;
		estimated_cost: string;
		tradeoffs: string[];
	}

	let description = '';
	let loading = false;
	let recommendation: Recommendation | null = null;
	let feedbackSent = false;

	const blueprintIcons: Record<string, typeof Server> = {
		'ecs-web-app': Server,
		'full-stack': Database,
		'static-site': Globe,
		serverless: Zap
	};

	const blueprintLabels: Record<string, string> = {
		'ecs-web-app': 'ECS Web App',
		'full-stack': 'Full Stack (ECS + RDS)',
		'static-site': 'Static Site (S3 + CDN)',
		serverless: 'Serverless (Lambda)'
	};

	const regionLabels: Record<string, string> = {
		'us-east-1': 'US East (N. Virginia)',
		'us-east-2': 'US East (Ohio)',
		'us-west-2': 'US West (Oregon)',
		'eu-west-1': 'EU West (Ireland)',
		'ap-southeast-1': 'Asia Pacific (Singapore)'
	};

	async function getRecommendation() {
		if (!description.trim()) {
			toast.error('Please describe your application first');
			return;
		}

		loading = true;
		recommendation = null;
		feedbackSent = false;

		try {
			const response = await fetch(`${pb.baseUrl}/api/intelligence/recommend`, {
				method: 'POST',
				headers: {
					Authorization: pb.authStore.token,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ description })
			});

			if (response.ok) {
				recommendation = await response.json();
			} else {
				const err = await response.json();
				toast.error(err.error || 'Failed to get recommendation');
			}
		} catch (err) {
			console.error(err);
			toast.error('Failed to connect to AI advisor');
		} finally {
			loading = false;
		}
	}

	function acceptRecommendation() {
		if (!recommendation) return;
		dispatch('accept', {
			blueprintId: recommendation.blueprint_id,
			region: recommendation.region,
			instanceSize: recommendation.instance_size
		});
		toast.success('Recommendation applied!');
	}

	async function sendFeedback(helpful: boolean) {
		feedbackSent = true;
		// Log feedback for future prompt tuning
		try {
			await fetch(`${pb.baseUrl}/api/intelligence/feedback`, {
				method: 'POST',
				headers: {
					Authorization: pb.authStore.token,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({
					description,
					recommendation,
					helpful,
					type: 'advisor'
				})
			});
		} catch {
			// Feedback is best-effort
		}
		toast.success(helpful ? 'Thanks for the feedback!' : 'Got it — we\'ll improve this.');
	}
</script>

<div class="space-y-5">
	<!-- Header -->
	<div class="flex items-center gap-3">
		<div class="p-2 bg-purple-100 dark:bg-purple-900 rounded-lg">
			<Sparkles class="w-5 h-5 text-purple-600 dark:text-purple-400" />
		</div>
		<div>
			<h3 class="font-semibold text-gray-900 dark:text-white">AI Deployment Advisor</h3>
			<p class="text-xs text-gray-500 dark:text-gray-400">
				Describe your app and get an instant infrastructure recommendation
			</p>
		</div>
	</div>

	<!-- Input -->
	<div>
		<Textarea
			bind:value={description}
			placeholder="e.g. A Node.js REST API with a PostgreSQL database, expecting ~500 requests/day, needs to store user uploads..."
			rows={3}
			class="text-sm"
		/>
	</div>

	<Button
		on:click={getRecommendation}
		disabled={loading || !description.trim()}
		class="w-full"
		color="purple"
	>
		{#if loading}
			<Spinner size="4" class="mr-2" />
			Analyzing...
		{:else}
			<Sparkles class="w-4 h-4 mr-2" />
			Get Recommendation
		{/if}
	</Button>

	<!-- Recommendation Card -->
	{#if recommendation}
		{@const Icon = blueprintIcons[recommendation.blueprint_id] ?? Server}
		<Card class="border-purple-200 dark:border-purple-800 bg-purple-50 dark:bg-purple-950">
			<div class="space-y-4">
				<!-- Blueprint + Region -->
				<div class="flex items-start justify-between gap-4">
					<div class="flex items-center gap-3">
						<div class="p-2 bg-purple-100 dark:bg-purple-900 rounded-lg">
							<Icon class="w-5 h-5 text-purple-600 dark:text-purple-400" />
						</div>
						<div>
							<div class="font-semibold text-gray-900 dark:text-white">
								{blueprintLabels[recommendation.blueprint_id] ?? recommendation.blueprint_id}
							</div>
							<div class="text-xs text-gray-500 dark:text-gray-400">
								{regionLabels[recommendation.region] ?? recommendation.region}
								{#if recommendation.instance_size}
									· {recommendation.instance_size}
								{/if}
							</div>
						</div>
					</div>
					<Badge color="purple" class="shrink-0">{recommendation.estimated_cost}/mo</Badge>
				</div>

				<!-- Reasoning -->
				<p class="text-sm text-gray-700 dark:text-gray-300 leading-relaxed">
					{recommendation.reasoning}
				</p>

				<!-- Tradeoffs -->
				{#if recommendation.tradeoffs?.length}
					<div>
						<p class="text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wide mb-2">
							Tradeoffs
						</p>
						<ul class="space-y-1">
							{#each recommendation.tradeoffs as tradeoff}
								<li class="flex items-start gap-2 text-sm text-gray-600 dark:text-gray-400">
									<ChevronRight class="w-3 h-3 mt-0.5 shrink-0 text-purple-500" />
									{tradeoff}
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				<!-- Actions -->
				<div class="flex items-center gap-2 pt-2 border-t dark:border-purple-800">
					<Button size="sm" color="purple" on:click={acceptRecommendation} class="flex-1">
						<Check class="w-4 h-4 mr-1" />
						Use This Setup
					</Button>
					<Button size="sm" color="alternative" on:click={getRecommendation}>
						<RefreshCw class="w-4 h-4" />
					</Button>
				</div>

				<!-- Feedback -->
				{#if !feedbackSent}
					<div class="flex items-center gap-2 text-xs text-gray-500">
						<span>Was this helpful?</span>
						<button
							on:click={() => sendFeedback(true)}
							class="hover:text-green-600 transition-colors"
							aria-label="Yes"
						>
							<ThumbsUp class="w-4 h-4" />
						</button>
						<button
							on:click={() => sendFeedback(false)}
							class="hover:text-red-500 transition-colors"
							aria-label="No"
						>
							<ThumbsDown class="w-4 h-4" />
						</button>
					</div>
				{:else}
					<p class="text-xs text-gray-400">Thanks for the feedback ✓</p>
				{/if}
			</div>
		</Card>
	{/if}
</div>
