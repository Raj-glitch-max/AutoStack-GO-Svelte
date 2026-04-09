<script lang="ts">
	import { onMount } from 'svelte';
	import RecoveryDashboard from '$lib/components/intelligence/RecoveryDashboard.svelte';
	import { pb } from '$lib/pocketbase';
	import { goto } from '$app/navigation';

	let loading = true;

	onMount(() => {
		// Check if user is authenticated
		if (!pb.authStore.isValid) {
			goto('/login');
			return;
		}
		loading = false;
	});
</script>

<svelte:head>
	<title>AI Intelligence - AutoStack</title>
</svelte:head>

{#if loading}
	<div class="flex items-center justify-center min-h-screen">
		<div class="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
	</div>
{:else}
	<div class="container mx-auto px-4 py-8 max-w-7xl">
		<RecoveryDashboard />
	</div>
{/if}

<style>
	:global(body) {
		background-color: #f9fafb;
	}
</style>
