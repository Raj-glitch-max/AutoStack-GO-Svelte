<script lang="ts">
	import { Breadcrumb, BreadcrumbItem } from 'flowbite-svelte';
	import { Home, Webhook } from 'lucide-svelte';
	import WebhookManager from '$lib/components/webhooks/WebhookManager.svelte';
</script>

<svelte:head>
	<title>Webhooks - AutoStack</title>
</svelte:head>

<div class="p-6">
	<!-- Breadcrumb -->
	<Breadcrumb class="mb-6">
		<BreadcrumbItem href="/app" home>
			<Home class="w-4 h-4" />
		</BreadcrumbItem>
		<BreadcrumbItem>
			<Webhook class="w-4 h-4 mr-2" />
			Webhooks
		</BreadcrumbItem>
	</Breadcrumb>

	<!-- Webhook Manager Component -->
	<WebhookManager />
</div>
