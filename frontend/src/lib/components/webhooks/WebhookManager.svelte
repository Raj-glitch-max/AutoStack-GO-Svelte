<script lang="ts">
	import { onMount } from 'svelte';
	import { pb } from '$lib/pocketbase';
	import {
		Card,
		Button,
		Modal,
		Label,
		Input,
		Checkbox,
		Badge,
		Spinner,
		Alert
	} from 'flowbite-svelte';
	import {
		Webhook,
		Plus,
		Trash2,
		Edit,
		TestTube,
		Check,
		X,
		AlertTriangle,
		Copy
	} from 'lucide-svelte';
	import toast from 'svelte-french-toast';

	interface Webhook {
		id: string;
		name: string;
		url: string;
		events: string[];
		enabled: boolean;
		secret: string;
		lastTriggered?: string;
		failureCount: number;
		created: string;
	}

	let webhooks: Webhook[] = [];
	let loading = true;
	let showModal = false;
	let editingWebhook: Webhook | null = null;

	// Form state
	let formName = '';
	let formUrl = '';
	let formEvents: string[] = [];
	let formSecret = '';

	const availableEvents = [
		{ value: 'deploy.started', label: 'Deployment Started', description: 'When a deployment begins' },
		{ value: 'deploy.success', label: 'Deployment Success', description: 'When a deployment completes successfully' },
		{ value: 'deploy.failed', label: 'Deployment Failed', description: 'When a deployment fails' },
		{ value: 'cost.alert', label: 'Cost Alert', description: 'When a cost anomaly is detected' },
		{ value: 'health.degraded', label: 'Health Degraded', description: 'When service health degrades' }
	];

	async function loadWebhooks() {
		loading = true;
		try {
			const response = await fetch(`${pb.baseUrl}/api/webhooks`, {
				headers: {
					Authorization: pb.authStore.token
				}
			});

			if (response.ok) {
				webhooks = await response.json();
			}
		} catch (err) {
			console.error('Error loading webhooks:', err);
			toast.error('Failed to load webhooks');
		} finally {
			loading = false;
		}
	}

	function openCreateModal() {
		editingWebhook = null;
		formName = '';
		formUrl = '';
		formEvents = [];
		formSecret = '';
		showModal = true;
	}

	function openEditModal(webhook: Webhook) {
		editingWebhook = webhook;
		formName = webhook.name;
		formUrl = webhook.url;
		formEvents = [...webhook.events];
		formSecret = webhook.secret;
		showModal = true;
	}

	async function saveWebhook() {
		if (!formName || !formUrl || formEvents.length === 0) {
			toast.error('Please fill in all required fields');
			return;
		}

		try {
			const payload = {
				name: formName,
				url: formUrl,
				events: formEvents,
				...(formSecret && { secret: formSecret })
			};

			const url = editingWebhook
				? `${pb.baseUrl}/api/webhooks/${editingWebhook.id}`
				: `${pb.baseUrl}/api/webhooks`;

			const method = editingWebhook ? 'PUT' : 'POST';

			const response = await fetch(url, {
				method,
				headers: {
					Authorization: pb.authStore.token,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify(payload)
			});

			if (response.ok) {
				toast.success(editingWebhook ? 'Webhook updated' : 'Webhook created');
				showModal = false;
				loadWebhooks();
			} else {
				const error = await response.json();
				toast.error(error.message || 'Failed to save webhook');
			}
		} catch (err) {
			console.error('Error saving webhook:', err);
			toast.error('Failed to save webhook');
		}
	}

	async function deleteWebhook(id: string) {
		if (!confirm('Are you sure you want to delete this webhook?')) {
			return;
		}

		try {
			const response = await fetch(`${pb.baseUrl}/api/webhooks/${id}`, {
				method: 'DELETE',
				headers: {
					Authorization: pb.authStore.token
				}
			});

			if (response.ok) {
				toast.success('Webhook deleted');
				loadWebhooks();
			}
		} catch (err) {
			console.error('Error deleting webhook:', err);
			toast.error('Failed to delete webhook');
		}
	}

	async function testWebhook(id: string) {
		try {
			const response = await fetch(`${pb.baseUrl}/api/webhooks/${id}/test`, {
				method: 'POST',
				headers: {
					Authorization: pb.authStore.token
				}
			});

			if (response.ok) {
				toast.success('Test webhook sent successfully');
			} else {
				const error = await response.json();
				toast.error(error.message || 'Failed to send test webhook');
			}
		} catch (err) {
			console.error('Error testing webhook:', err);
			toast.error('Failed to send test webhook');
		}
	}

	async function toggleWebhook(webhook: Webhook) {
		try {
			const response = await fetch(`${pb.baseUrl}/api/webhooks/${webhook.id}`, {
				method: 'PUT',
				headers: {
					Authorization: pb.authStore.token,
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ enabled: !webhook.enabled })
			});

			if (response.ok) {
				toast.success(webhook.enabled ? 'Webhook disabled' : 'Webhook enabled');
				loadWebhooks();
			}
		} catch (err) {
			console.error('Error toggling webhook:', err);
			toast.error('Failed to toggle webhook');
		}
	}

	function copySecret(secret: string) {
		navigator.clipboard.writeText(secret);
		toast.success('Secret copied to clipboard');
	}

	onMount(() => {
		loadWebhooks();
	});
</script>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-3">
			<div class="p-3 bg-primary-100 dark:bg-primary-900 rounded-lg">
				<Webhook class="w-6 h-6 text-primary-600 dark:text-primary-400" />
			</div>
			<div>
				<h2 class="text-2xl font-bold text-gray-900 dark:text-white">Webhooks</h2>
				<p class="text-sm text-gray-600 dark:text-gray-400">
					Receive real-time notifications about deployment events
				</p>
			</div>
		</div>
		<Button on:click={openCreateModal}>
			<Plus class="w-4 h-4 mr-2" />
			Add Webhook
		</Button>
	</div>

	<!-- Info Alert -->
	<Alert color="blue">
		<span class="font-semibold">How webhooks work:</span> When events occur (deployments, cost alerts, etc.),
		AutoStack will send HTTP POST requests to your configured URLs with event details.
	</Alert>

	<!-- Webhooks List -->
	{#if loading}
		<div class="flex justify-center py-12">
			<Spinner size="8" />
		</div>
	{:else if webhooks.length === 0}
		<Card class="text-center py-12">
			<Webhook class="w-16 h-16 mx-auto text-gray-400 mb-4" />
			<h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-2">No webhooks configured</h3>
			<p class="text-gray-600 dark:text-gray-400 mb-4">
				Get started by creating your first webhook
			</p>
			<Button on:click={openCreateModal}>
				<Plus class="w-4 h-4 mr-2" />
				Create Webhook
			</Button>
		</Card>
	{:else}
		<div class="grid gap-4">
			{#each webhooks as webhook}
				<Card>
					<div class="flex items-start justify-between">
						<div class="flex-1">
							<div class="flex items-center gap-3 mb-2">
								<h3 class="text-lg font-semibold text-gray-900 dark:text-white">
									{webhook.name}
								</h3>
								{#if webhook.enabled}
									<Badge color="green">
										<Check class="w-3 h-3 mr-1" />
										Enabled
									</Badge>
								{:else}
									<Badge color="dark">
										<X class="w-3 h-3 mr-1" />
										Disabled
									</Badge>
								{/if}
								{#if webhook.failureCount > 0}
									<Badge color="red">
										<AlertTriangle class="w-3 h-3 mr-1" />
										{webhook.failureCount} failures
									</Badge>
								{/if}
							</div>

							<p class="text-sm text-gray-600 dark:text-gray-400 mb-3 font-mono">
								{webhook.url}
							</p>

							<div class="flex flex-wrap gap-2 mb-3">
								{#each webhook.events as event}
									<Badge color="blue">{event}</Badge>
								{/each}
							</div>

							<div class="flex items-center gap-4 text-xs text-gray-500">
								{#if webhook.lastTriggered}
									<span>Last triggered: {new Date(webhook.lastTriggered).toLocaleString()}</span>
								{:else}
									<span>Never triggered</span>
								{/if}
								<button
									on:click={() => copySecret(webhook.secret)}
									class="flex items-center gap-1 hover:text-primary-600"
								>
									<Copy class="w-3 h-3" />
									Copy Secret
								</button>
							</div>
						</div>

						<div class="flex gap-2">
							<Button size="sm" color="alternative" on:click={() => testWebhook(webhook.id)}>
								<TestTube class="w-4 h-4" />
							</Button>
							<Button size="sm" color="alternative" on:click={() => openEditModal(webhook)}>
								<Edit class="w-4 h-4" />
							</Button>
							<Button
								size="sm"
								color="alternative"
								on:click={() => toggleWebhook(webhook)}
							>
								{webhook.enabled ? 'Disable' : 'Enable'}
							</Button>
							<Button size="sm" color="red" on:click={() => deleteWebhook(webhook.id)}>
								<Trash2 class="w-4 h-4" />
							</Button>
						</div>
					</div>
				</Card>
			{/each}
		</div>
	{/if}
</div>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} size="lg">
	<h3 class="text-xl font-semibold text-gray-900 dark:text-white mb-4">
		{editingWebhook ? 'Edit Webhook' : 'Create Webhook'}
	</h3>

	<div class="space-y-4">
		<div>
			<Label for="name" class="mb-2">Name</Label>
			<Input id="name" bind:value={formName} placeholder="My Webhook" required />
		</div>

		<div>
			<Label for="url" class="mb-2">Webhook URL</Label>
			<Input
				id="url"
				type="url"
				bind:value={formUrl}
				placeholder="https://example.com/webhook"
				required
			/>
			<p class="mt-1 text-xs text-gray-500">
				AutoStack will send POST requests to this URL
			</p>
		</div>

		<div>
			<Label class="mb-2">Events to Subscribe</Label>
			<div class="space-y-2">
				{#each availableEvents as event}
					<div class="flex items-start gap-2 p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
						<Checkbox
							bind:group={formEvents}
							value={event.value}
							class="mt-1"
						/>
						<div class="flex-1">
							<div class="font-medium text-sm">{event.label}</div>
							<div class="text-xs text-gray-600 dark:text-gray-400">{event.description}</div>
						</div>
					</div>
				{/each}
			</div>
		</div>

		<div>
			<Label for="secret" class="mb-2">Secret (Optional)</Label>
			<Input
				id="secret"
				bind:value={formSecret}
				placeholder="Leave empty to auto-generate"
			/>
			<p class="mt-1 text-xs text-gray-500">
				Used to sign webhook payloads for verification
			</p>
		</div>
	</div>

	<div class="flex justify-end gap-2 mt-6">
		<Button color="alternative" on:click={() => (showModal = false)}>Cancel</Button>
		<Button on:click={saveWebhook}>
			{editingWebhook ? 'Update' : 'Create'} Webhook
		</Button>
	</div>
</Modal>
