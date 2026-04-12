<script lang="ts">
	import { onMount } from 'svelte';
	import { client as pb } from '$lib/pocketbase';

	let stats: any = null;
	let loading = true;
	let error = '';

	async function loadStats() {
		loading = true;
		error = '';

		try {
			const response = await fetch('/api/intelligence/stats', {
				headers: {
					'Authorization': pb.authStore.token
				}
			});

			if (!response.ok) {
				throw new Error('Failed to load statistics');
			}

			stats = await response.json();
		} catch (err: any) {
			error = err.message || 'Failed to load statistics';
		} finally {
			loading = false;
		}
	}

	function formatTime(seconds: number): string {
		if (seconds < 60) {
			return `${seconds.toFixed(0)}s`;
		}
		const minutes = Math.floor(seconds / 60);
		const remainingSeconds = seconds % 60;
		return `${minutes}m ${remainingSeconds.toFixed(0)}s`;
	}

	onMount(() => {
		loadStats();
	});

</script>

<div class="recovery-dashboard">
	<div class="mb-6">
		<h2 class="text-2xl font-bold text-gray-900 mb-2">🤖 AI Recovery Intelligence</h2>
		<p class="text-gray-600">
			Automatic error detection and recovery statistics for your deployments
		</p>
	</div>

	{#if loading}
		<div class="flex items-center justify-center p-12">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
			<span class="ml-3 text-gray-600">Loading statistics...</span>
		</div>
	{:else if error}
		<div class="bg-red-50 border border-red-200 rounded-lg p-4">
			<div class="flex items-center">
				<svg class="w-5 h-5 text-red-600 mr-2" fill="currentColor" viewBox="0 0 20 20">
					<path
						fill-rule="evenodd"
						d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
						clip-rule="evenodd"
					/>
				</svg>
				<span class="text-red-800">{error}</span>
			</div>
		</div>
	{:else if stats}
		<!-- Stats Grid -->
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
			<!-- Total Attempts -->
			<div class="bg-white border border-gray-200 rounded-lg p-6 shadow-sm">
				<div class="flex items-center justify-between mb-2">
					<span class="text-sm font-medium text-gray-600">Total Attempts</span>
					<svg class="w-5 h-5 text-blue-500" fill="currentColor" viewBox="0 0 20 20">
						<path d="M9 2a1 1 0 000 2h2a1 1 0 100-2H9z" />
						<path
							fill-rule="evenodd"
							d="M4 5a2 2 0 012-2 3 3 0 003 3h2a3 3 0 003-3 2 2 0 012 2v11a2 2 0 01-2 2H6a2 2 0 01-2-2V5zm3 4a1 1 0 000 2h.01a1 1 0 100-2H7zm3 0a1 1 0 000 2h3a1 1 0 100-2h-3zm-3 4a1 1 0 100 2h.01a1 1 0 100-2H7zm3 0a1 1 0 100 2h3a1 1 0 100-2h-3z"
							clip-rule="evenodd"
						/>
					</svg>
				</div>
				<div class="text-3xl font-bold text-gray-900">{stats.totalAttempts}</div>
			</div>

			<!-- Success Rate -->
			<div class="bg-white border border-gray-200 rounded-lg p-6 shadow-sm">
				<div class="flex items-center justify-between mb-2">
					<span class="text-sm font-medium text-gray-600">Success Rate</span>
					<svg class="w-5 h-5 text-green-500" fill="currentColor" viewBox="0 0 20 20">
						<path
							fill-rule="evenodd"
							d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
							clip-rule="evenodd"
						/>
					</svg>
				</div>
				<div class="text-3xl font-bold text-gray-900">{stats.successRate.toFixed(1)}%</div>
				<div class="mt-1 text-xs text-gray-500">
					{stats.successfulFixes} successful / {stats.totalAttempts} total
				</div>
			</div>

			<!-- Avg Recovery Time -->
			<div class="bg-white border border-gray-200 rounded-lg p-6 shadow-sm">
				<div class="flex items-center justify-between mb-2">
					<span class="text-sm font-medium text-gray-600">Avg Recovery Time</span>
					<svg class="w-5 h-5 text-purple-500" fill="currentColor" viewBox="0 0 20 20">
						<path
							fill-rule="evenodd"
							d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.828 2.829a1 1 0 101.415-1.415L11 9.586V6z"
							clip-rule="evenodd"
						/>
					</svg>
				</div>
				<div class="text-3xl font-bold text-gray-900">{formatTime(stats.avgRecoveryTime)}</div>
			</div>

			<!-- Failed Fixes -->
			<div class="bg-white border border-gray-200 rounded-lg p-6 shadow-sm">
				<div class="flex items-center justify-between mb-2">
					<span class="text-sm font-medium text-gray-600">Failed Fixes</span>
					<svg class="w-5 h-5 text-red-500" fill="currentColor" viewBox="0 0 20 20">
						<path
							fill-rule="evenodd"
							d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"
							clip-rule="evenodd"
						/>
					</svg>
				</div>
				<div class="text-3xl font-bold text-gray-900">{stats.failedFixes}</div>
			</div>
		</div>

		<!-- Most Common Errors -->
		{#if stats.mostCommonErrors && stats.mostCommonErrors.length > 0}
			<div class="bg-white border border-gray-200 rounded-lg p-6 shadow-sm">
				<h3 class="text-lg font-semibold text-gray-900 mb-4">📊 Most Common Errors</h3>
				<div class="space-y-3">
					{#each stats.mostCommonErrors as errorStat, index}
						<div class="flex items-center justify-between">
							<div class="flex items-center flex-1">
								<span class="text-sm font-medium text-gray-500 w-6">{index + 1}.</span>
								<span class="text-sm text-gray-900 ml-2">{errorStat.error_type}</span>
							</div>
							<div class="flex items-center">
								<div class="w-32 bg-gray-200 rounded-full h-2 mr-3">
									<div
										class="bg-blue-600 h-2 rounded-full"
										style="width: {(errorStat.count / stats.totalAttempts) * 100}%"
									></div>
								</div>
								<span class="text-sm font-semibold text-gray-700 w-12 text-right">
									{errorStat.count}
								</span>
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Info Box -->
		<div class="mt-8 bg-blue-50 border border-blue-200 rounded-lg p-4">
			<div class="flex items-start">
				<svg class="w-5 h-5 text-blue-600 mt-0.5 mr-3" fill="currentColor" viewBox="0 0 20 20">
					<path
						fill-rule="evenodd"
						d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z"
						clip-rule="evenodd"
					/>
				</svg>
				<div class="flex-1">
					<h4 class="text-sm font-semibold text-blue-900 mb-1">How AI Recovery Works</h4>
					<p class="text-sm text-blue-800">
						When a deployment fails, our AI system automatically analyzes the error logs, identifies
						the root cause, and attempts to fix common issues like resource naming conflicts,
						timeout errors, and configuration problems. This reduces manual debugging time by up to
						90%.
					</p>
				</div>
			</div>
		</div>
	{:else}
		<div class="text-center p-12 text-gray-500">
			<svg class="w-16 h-16 mx-auto mb-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path
					stroke-linecap="round"
					stroke-linejoin="round"
					stroke-width="2"
					d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
				/>
			</svg>
			<p class="text-lg font-medium">No recovery data yet</p>
			<p class="text-sm mt-1">Statistics will appear after your first deployment</p>
		</div>
	{/if}
</div>

<style>
	.recovery-dashboard {
		@apply w-full;
	}
</style>
