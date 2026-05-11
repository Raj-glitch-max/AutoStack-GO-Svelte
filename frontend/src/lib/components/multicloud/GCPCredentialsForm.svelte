<script lang="ts">
	import { pb } from '$lib/pocketbase';
	import { Card, Label, Input, Textarea, Button, Spinner, Alert } from 'flowbite-svelte';
	import { CheckCircle, AlertTriangle } from 'lucide-svelte';
	import toast from 'svelte-french-toast';

	let projectId = '';
	let serviceAccountKey = '';
	let region = 'us-central1';
	let stateBucketName = '';
	let saving = false;
	let validating = false;
	let validated = false;

	const gcpRegions = [
		'us-central1', 'us-east1', 'us-west1', 'us-west2',
		'europe-west1', 'europe-west2', 'asia-east1', 'asia-southeast1'
	];

	async function saveCredentials() {
		if (!projectId || !serviceAccountKey) {
			toast.error('Project ID and Service Account Key are required');
			return;
		}

		// Validate JSON
		try {
			JSON.parse(serviceAccountKey);
		} catch {
			toast.error('Service Account Key must be valid JSON');
			return;
		}

		saving = true;
		try {
			const resp = await fetch(`${pb.baseUrl}/api/gcp/credentials`, {
				method: 'POST',
				headers: { Authorization: pb.authStore.token, 'Content-Type': 'application/json' },
				body: JSON.stringify({ projectId, serviceAccountKey, region, stateBucketName })
			});

			if (resp.ok) {
				toast.success('GCP credentials saved');
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
			const resp = await fetch(`${pb.baseUrl}/api/gcp/validate-credentials`, {
				method: 'POST',
				headers: { Authorization: pb.authStore.token }
			});

			const data = await resp.json();
			if (data.valid) {
				validated = true;
				toast.success(`Validated! Project: ${data.projectId}`);
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
		<span class="text-2xl">🔵</span>
		<div>
			<h3 class="text-lg font-bold text-gray-900 dark:text-white">Google Cloud Setup</h3>
			<p class="text-sm text-gray-500 dark:text-gray-400">Connect your GCP project via a Service Account</p>
		</div>
	</div>

	<div class="space-y-4">
		<div>
			<Label for="gcp-project" class="mb-2">GCP Project ID</Label>
			<Input id="gcp-project" bind:value={projectId} placeholder="my-project-123456" />
		</div>

		<div>
			<Label for="gcp-region" class="mb-2">Default Region</Label>
			<select
				id="gcp-region"
				bind:value={region}
				class="w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-sm p-2.5"
			>
				{#each gcpRegions as r}
					<option value={r}>{r}</option>
				{/each}
			</select>
		</div>

		<div>
			<Label for="gcp-bucket" class="mb-2">Terraform State Bucket (optional)</Label>
			<Input id="gcp-bucket" bind:value={stateBucketName} placeholder="my-terraform-state-bucket" />
			<p class="mt-1 text-xs text-gray-500">GCS bucket for storing Terraform state</p>
		</div>

		<div>
			<Label for="gcp-key" class="mb-2">Service Account Key (JSON)</Label>
			<Textarea
				id="gcp-key"
				bind:value={serviceAccountKey}
				placeholder="Paste your service account JSON key here..."
				rows={6}
				class="font-mono text-xs"
			/>
			<p class="mt-1 text-xs text-gray-500">
				Paste the full JSON key file. Requires roles: Cloud Run Admin, Cloud SQL Admin, Storage Admin.
			</p>
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
