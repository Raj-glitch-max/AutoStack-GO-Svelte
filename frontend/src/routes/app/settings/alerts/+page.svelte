<script lang="ts">
	import { Breadcrumb, BreadcrumbItem } from 'flowbite-svelte';
	import { Home, Settings, Bell } from 'lucide-svelte';
	import AlertSettings from '$lib/components/cost/AlertSettings.svelte';
</script>

<svelte:head>
	<title>Alert Settings - AutoStack</title>
</svelte:head>

<div class="p-6">
	<!-- Breadcrumb -->
	<Breadcrumb class="mb-6">
		<BreadcrumbItem href="/app" home>
			<Home class="w-4 h-4" />
		</BreadcrumbItem>
		<BreadcrumbItem href="/app/settings">
			<Settings class="w-4 h-4 mr-2" />
			Settings
		</BreadcrumbItem>
		<BreadcrumbItem>
			<Bell class="w-4 h-4 mr-2" />
			Alert Preferences
		</BreadcrumbItem>
	</Breadcrumb>

	<!-- Alert Settings Component -->
	<AlertSettings />
</div>
