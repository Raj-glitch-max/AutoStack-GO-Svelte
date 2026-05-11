<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { Card, Label, Input, Button, Spinner, Alert } from 'flowbite-svelte';
	import { CheckCircle } from 'lucide-svelte';
	import toast from 'svelte-french-toast';

	let clientId = '';
	let clientSecret = '';
	let tenantId = '';
	let subscriptionId = '';
	let region = 'eastus';
	let stateContainerName = '';
	let stateStorageAccount = '';
	let saving = false;
	let validating = false;
	let validated = false;

	const azureRegions = [
		'eastus', 'eastus2', 'westus', 'westus2', 'westus3',
		'northeurope', 'westeurope', 'uksouth', 'eastasia', 'southeastasia'
	];

	async function saveCredentials() {
		if (!clientId || !clientSecret || !tenantId || !subscriptionId) {
			toast.error('All four credential fields are required');
			return;
		}

		saving = true;
		try {
			const resp = await fetch(`${pb.baseUrl}/api/azure/credentials`, {
				method: 'POST',
				headers: { Authorization: pb.authStore.token, 'Content-Type': 'application/json' },
				body: JSON.stringify({
					clientId, clientSecret, tenantId, subscriptionId,
					region, stateContainerName, stateStorageAccount
				})
			});

			if (resp.ok) {
				toast.success('Azure credentials saved');
			} else {
				const err = await resp.json();
				toast.error(err.message || 'Failed to save credentials');
			}
		} catch {
			toast.error('Failed to save credentials');
		} finally {
			saving = false;
		}
	}

	async function validateCredentials() {
		validating = true;
		validated = false;
		try {
			const resp = await fetch(`${pb.baseUrl}/api/azure/validate-credentials`, {
				method: 'POST',
				headers: { Authorization: pb.authStore.token }
			});

			const data = await resp.json();
			if (data.valid) {
				validated = true;
				toast.success(`Validated! Subscription: ${data.subscriptionId}`);
			} else {
				toast.error(data.error || 'Validation failed');
			}
		} catch {
			toast.error('Validation request failed');
		} finally {
			validating = false;
		}
	}
</script>

<Card>
	<div class="flex items-center gap-3 mb-6">
		<span class="text-2xl">🔷</span>
		<div>
			<h3 class="text-lg font-bold text-gray-900 dark:text-white">Azure Setup</h3>
			<p class="text-sm text-gray-500 dark:text-gray-400">Connect via a Service Principal</p>
		</div>
	</div>

	<div class="space-y-4">
		<div>
			<Label for="az-tenant" class="mb-2">Tenant ID</Label>
			<Input id="az-tenant" bind:value={tenantId} placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
		</div>

		<div>
			<Label for="az-sub" class="mb-2">Subscription ID</Label>
			<Input id="az-sub" bind:value={subscriptionId} placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
		</div>

		<div>
			<Label for="az-client" class="mb-2">Client ID (App ID)</Label>
			<Input id="az-client" bind:value={clientId} placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
		</div>

		<div>
			<Label for="az-secret" class="mb-2">Client Secret</Label>
			<Input id="az-secret" type="password" bind:value={clientSecret} placeholder="Your service principal secret" />
		</div>

		<div>
			<Label for="az-region" class="mb-2">Default Region</Label>
			<select
				id="az-region"
				bind:value={region}
				class="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm p-2.5"
			>
				{#each azureRegions as r}
					<option value={r}>{r}</option>
				{/each}
			</select>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<Label for="az-storage" class="mb-2">State Storage Account</Label>
				<Input id="az-storage" bind:value={stateStorageAccount} placeholder="mystorageaccount" />
			</div>
			<div>
				<Label for="az-container" class="mb-2">State Container</Label>
				<Input id="az-container" bind:value={stateContainerName} placeholder="tfstate" />
			</div>
		</div>

		{#if validated}
			<Alert color="green">
				<CheckCircle class="w-4 h-4 mr-2" />
				Credentials validated successfully
			</Alert>
		{/if}

		<div class="flex gap-2 pt-2">
			<Button on:click={saveCredentials} disabled={saving} class="flex-1">
				{#if saving}<Spinner size="4" class="mr-2" />{/if}
				Save Credentials
			</Button>
			<Button color="alternative" on:click={validateCredentials} disabled={validating}>
				{#if validating}<Spinner size="4" class="mr-2" />{/if}
				Validate
			</Button>
		</div>
	</div>
</Card>
