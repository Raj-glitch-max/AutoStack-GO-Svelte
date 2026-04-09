<script lang="ts">
	import { onMount } from 'svelte';
	import { pb } from '$lib/pocketbase';

	export let deploymentId: string;
	export let errorLogs: string = '';

	let analysis: any = null;
	let loading = false;
	let error = '';

	interface FixStep {
		step: string;
		completed: boolean;
	}

	let fixSteps: FixStep[] = [];

	async function analyzeError() {
		if (!errorLogs) {
			error = 'No error logs provided';
			return;
		}

		loading = true;
		error = '';

		try {
			const response = await fetch(`/api/intelligence/analyze/${deploymentId}`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'Authorization': pb.authStore.token
				},
				body: JSON.stringify({
					error_logs: errorLogs
				})
			});

			if (!response.ok) {
				throw new Error('Failed to analyze error');
			}

			const data = await response.json();
			analysis = data.analysis;

			// Convert fix steps to checkable items
			if (analysis.fix_steps) {
				fixSteps = analysis.fix_steps.map((step: string) => ({
					step,
					completed: false
				}));
			}
		} catch (err: any) {
			error = err.message || 'Failed to analyze error';
		} finally {
			loading = false;
		}
	}

	function getSeverityColor(severity: string): string {
		switch (severity) {
			case 'critical':
				return 'text-red-600 bg-red-50 border-red-200';
			case 'high':
				return 'text-orange-600 bg-orange-50 border-orange-200';
			case 'medium':
				return 'text-yellow-600 bg-yellow-50 border-yellow-200';
			case 'low':
				return 'text-blue-600 bg-blue-50 border-blue-200';
			default:
				return 'text-gray-600 bg-gray-50 border-gray-200';
		}
	}

	function getConfidenceColor(confidence: number): string {
		if (confidence >= 0.8) return 'text-green-600';
		if (confidence >= 0.6) return 'text-yellow-600';
		return 'text-red-600';
	}

	onMount(() => {
		if (errorLogs) {
			analyzeError();
		}
	});
</script>

<div class="error-analysis-container">
	{#if loading}
		<div class="flex items-center justify-center p-8">
			<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
			<span class="ml-3 text-gray-600">Analyzing error with AI...</span>
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
	{:else if analysis}
		<div class="space-y-4">
			<!-- Header -->
			<div class="flex items-center justify-between">
				<h3 class="text-lg font-semibold text-gray-900">🔍 Error Analysis</h3>
				<div class="flex items-center space-x-2">
					<span class="text-sm text-gray-600">Confidence:</span>
					<span class="font-semibold {getConfidenceColor(analysis.confidence)}">
						{(analysis.confidence * 100).toFixed(0)}%
					</span>
				</div>
			</div>

			<!-- Severity Badge -->
			<div class="inline-flex items-center px-3 py-1 rounded-full border {getSeverityColor(analysis.severity)}">
				<span class="text-xs font-semibold uppercase">{analysis.severity} Severity</span>
			</div>

			<!-- Error Details -->
			<div class="bg-white border border-gray-200 rounded-lg p-4 space-y-3">
				<div>
					<span class="text-sm font-medium text-gray-700">Error Type:</span>
					<span class="ml-2 text-sm text-gray-900">{analysis.error_type}</span>
				</div>

				<div>
					<span class="text-sm font-medium text-gray-700">Category:</span>
					<span class="ml-2 text-sm text-gray-900">{analysis.category}</span>
				</div>

				<div>
					<span class="text-sm font-medium text-gray-700">Description:</span>
					<p class="mt-1 text-sm text-gray-900">{analysis.description}</p>
				</div>

				{#if analysis.root_cause}
					<div>
						<span class="text-sm font-medium text-gray-700">Root Cause:</span>
						<p class="mt-1 text-sm text-gray-900 font-mono bg-gray-50 p-2 rounded">
							{analysis.root_cause}
						</p>
					</div>
				{/if}
			</div>

			<!-- Suggested Fix -->
			<div class="bg-blue-50 border border-blue-200 rounded-lg p-4">
				<div class="flex items-start">
					<svg class="w-5 h-5 text-blue-600 mt-0.5 mr-2" fill="currentColor" viewBox="0 0 20 20">
						<path
							fill-rule="evenodd"
							d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z"
							clip-rule="evenodd"
						/>
					</svg>
					<div class="flex-1">
						<h4 class="text-sm font-semibold text-blue-900 mb-1">💡 Suggested Fix</h4>
						<p class="text-sm text-blue-800">{analysis.suggested_fix}</p>
					</div>
				</div>
			</div>

			<!-- Auto-Fixable Badge -->
			{#if analysis.auto_fixable}
				<div class="bg-green-50 border border-green-200 rounded-lg p-3">
					<div class="flex items-center">
						<svg class="w-5 h-5 text-green-600 mr-2" fill="currentColor" viewBox="0 0 20 20">
							<path
								fill-rule="evenodd"
								d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
								clip-rule="evenodd"
							/>
						</svg>
						<span class="text-sm font-medium text-green-800">
							✨ This error can be automatically fixed!
						</span>
					</div>
				</div>
			{:else}
				<div class="bg-yellow-50 border border-yellow-200 rounded-lg p-3">
					<div class="flex items-center">
						<svg class="w-5 h-5 text-yellow-600 mr-2" fill="currentColor" viewBox="0 0 20 20">
							<path
								fill-rule="evenodd"
								d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
								clip-rule="evenodd"
							/>
						</svg>
						<span class="text-sm font-medium text-yellow-800">
							Manual intervention required
						</span>
					</div>
				</div>
			{/if}

			<!-- Fix Steps -->
			{#if fixSteps.length > 0}
				<div class="bg-white border border-gray-200 rounded-lg p-4">
					<h4 class="text-sm font-semibold text-gray-900 mb-3">📋 Fix Steps</h4>
					<div class="space-y-2">
						{#each fixSteps as item, index}
							<label class="flex items-start cursor-pointer hover:bg-gray-50 p-2 rounded">
								<input
									type="checkbox"
									bind:checked={item.completed}
									class="mt-1 h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 rounded"
								/>
								<span class="ml-3 text-sm text-gray-700" class:line-through={item.completed}>
									{index + 1}. {item.step}
								</span>
							</label>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Estimated Fix Time -->
			{#if analysis.estimated_fix_time}
				<div class="flex items-center text-sm text-gray-600">
					<svg class="w-4 h-4 mr-1" fill="currentColor" viewBox="0 0 20 20">
						<path
							fill-rule="evenodd"
							d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.828 2.829a1 1 0 101.415-1.415L11 9.586V6z"
							clip-rule="evenodd"
						/>
					</svg>
					<span>Estimated fix time: <strong>{analysis.estimated_fix_time}</strong></span>
				</div>
			{/if}

			<!-- Related Documentation -->
			{#if analysis.related_docs && analysis.related_docs.length > 0}
				<div class="bg-gray-50 border border-gray-200 rounded-lg p-4">
					<h4 class="text-sm font-semibold text-gray-900 mb-2">📚 Related Documentation</h4>
					<ul class="space-y-1">
						{#each analysis.related_docs as doc}
							<li>
								<a
									href={doc}
									target="_blank"
									rel="noopener noreferrer"
									class="text-sm text-blue-600 hover:text-blue-800 hover:underline"
								>
									{doc}
								</a>
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		</div>
	{:else}
		<div class="text-center p-8 text-gray-500">
			<svg class="w-12 h-12 mx-auto mb-3 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
				<path
					stroke-linecap="round"
					stroke-linejoin="round"
					stroke-width="2"
					d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"
				/>
			</svg>
			<p>No error analysis available</p>
			{#if !errorLogs}
				<p class="text-sm mt-1">Provide error logs to analyze</p>
			{/if}
		</div>
	{/if}
</div>

<style>
	.error-analysis-container {
		@apply w-full;
	}
</style>
