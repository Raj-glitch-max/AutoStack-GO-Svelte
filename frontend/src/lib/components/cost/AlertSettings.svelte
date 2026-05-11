<script lang="ts">
	import { onMount } from 'svelte';
	import { pb } from '$lib/pocketbase';
	import { Card, Label, Input, Toggle, Select, Button, Spinner } from 'flowbite-svelte';
	import { Bell, Save, AlertTriangle } from 'lucide-svelte';
	import toast from 'svelte-french-toast';

	interface AlertPreferences {
		threshold: number;
		emailEnabled: boolean;
		frequency: 'immediate' | 'daily' | 'weekly';
		slackEnabled: boolean;
		slackWebhook: string;
	}

	let preferences: AlertPreferences = {
		threshold: 20,
		emailEnabled: true,
		frequency: 'immediate',
		slackEnabled: false,
		slackWebhook: ''
	};

	let loading = true;
	let saving = false;

	const frequencyOptions = [
		{ value: 'immediate', name: 'Immediate (as they occur)' },
		{ value: 'daily', name: 'Daily digest' },
		{ value: 'weekly', name: 'Weekly summary' }
	];

	async function loadPreferences() {
		loading = true;
		try {
			const response = await fetch(`${pb.baseUrl}/api/alerts/preferences`, {
				headers: {
					'Authorization': pb.authStore.token
				}
			});

			if (response.ok) {
				preferences = await response.json();
			}
		} catch (err) {
			console.error('Error loading preferences:', err);
			toast.error('Failed to load alert preferences');
		} finally {
			loading = false;
		}
	}

	async function savePreferences() {
		saving = true;
		try {
			const response = await fetch(`${pb.baseUrl}/api/alerts/preferences`, {
				method: 'PUT',
				headers: {
					'Authorization': pb.authStore.token,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify(preferences)
			});

			if (response.ok) {
				toast.success('Alert preferences saved');
			} else {
				const error = await response.json();
				toast.error(error.message || 'Failed to save preferences');
			}
		} catch (err) {
			console.error('Error saving preferences:', err);
			toast.error('Failed to save preferences');
		} finally {
			saving = false;
		}
	}

	onMount(() => {
		loadPreferences();
	});
</script>

<Card class="max-w-2xl">
	<div class="flex items-center gap-3 mb-6">
		<div class="p-3 bg-primary-100 dark:bg-primary-900 rounded-lg">
			<Bell class="w-6 h-6 text-primary-600 dark:text-primary-400" />
		</div>
		<div>
			<h3 class="text-xl font-bold text-gray-900 dark:text-white">Alert Preferences</h3>
			<p class="text-sm text-gray-600 dark:text-gray-400">
				Configure how you receive cost alerts
			</p>
		</div>
	</div>

	{#if loading}
		<div class="flex justify-center py-8">
			<Spinner size="8" />
		</div>
	{:else}
		<div class="space-y-6">
			<!-- Alert Threshold -->
			<div>
				<Label for="threshold" class="mb-2 flex items-center gap-2">
					<AlertTriangle class="w-4 h-4" />
					Alert Threshold
				</Label>
				<div class="flex items-center gap-4">
					<Input
						id="threshold"
						type="number"
						bind:value={preferences.threshold}
						min="0"
						max="100"
						step="5"
						class="flex-1"
					/>
					<span class="text-sm text-gray-600 dark:text-gray-400 whitespace-nowrap">
						% variance
					</span>
				</div>
				<p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
					You'll be alerted when actual costs exceed estimates by this percentage
				</p>
			</div>

			<!-- Email Notifications -->
			<div class="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
				<div>
					<Label class="text-base font-semibold">Email Notifications</Label>
					<p class="text-sm text-gray-600 dark:text-gray-400">
						Receive cost alerts via email
					</p>
				</div>
				<Toggle bind:checked={preferences.emailEnabled} />
			</div>

			<!-- Notification Frequency -->
			{#if preferences.emailEnabled}
				<div>
					<Label for="frequency" class="mb-2">Notification Frequency</Label>
					<Select
						id="frequency"
						bind:value={preferences.frequency}
						items={frequencyOptions}
						placeholder="Select frequency"
					/>
					<p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
						{#if preferences.frequency === 'immediate'}
							Alerts sent as soon as anomalies are detected
						{:else if preferences.frequency === 'daily'}
							All alerts from the day sent in a single email
						{:else}
							Weekly summary of all cost alerts
						{/if}
					</p>
				</div>
			{/if}

			<!-- Slack Integration (Future) -->
			<div class="flex items-center justify-between p-4 bg-gray-50 dark:bg-gray-800 rounded-lg opacity-50">
				<div>
					<Label class="text-base font-semibold">Slack Notifications</Label>
					<p class="text-sm text-gray-600 dark:text-gray-400">
						Coming soon: Receive alerts in Slack
					</p>
				</div>
				<Toggle disabled />
			</div>

			<!-- Save Button -->
			<div class="flex justify-end pt-4 border-t dark:border-gray-700">
				<Button on:click={savePreferences} disabled={saving}>
					{#if saving}
						<Spinner size="4" class="mr-2" />
						Saving...
					{:else}
						<Save class="w-4 h-4 mr-2" />
						Save Preferences
					{/if}
				</Button>
			</div>
		</div>
	{/if}
</Card>

<style>
	:global(.dark) input[type='number'] {
		background-color: rgb(31 41 55);
		border-color: rgb(55 65 81);
		color: white;
	}
</style>
