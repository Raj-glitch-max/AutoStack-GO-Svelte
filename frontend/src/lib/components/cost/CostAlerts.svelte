<script lang="ts">
	import { onMount } from 'svelte';
	import type { CostAlert } from '$lib/types/cost';

	export let deploymentId: string | null = null;

	let alerts: CostAlert[] = [];
	let loading = false;
	let error: string | null = null;

	async function fetchAlerts() {
		loading = true;
		error = null;

		try {
			const url = deploymentId
				? `/api/cost/alerts/deployment/${deploymentId}`
				: '/api/cost/alerts';

			const response = await fetch(url, {
				headers: {
					'Content-Type': 'application/json'
				}
			});

			if (!response.ok) {
				throw new Error('Failed to fetch alerts');
			}

			const data = await response.json();
			alerts = data.alerts || [];
		} catch (err) {
			error = err instanceof Error ? err.message : 'Unknown error';
		} finally {
			loading = false;
		}
	}

	async function acknowledgeAlert(alertId: string) {
		try {
			const response = await fetch(`/api/cost/alerts/${alertId}/acknowledge`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				}
			});

			if (!response.ok) {
				throw new Error('Failed to acknowledge alert');
			}

			// Refresh alerts
			await fetchAlerts();
		} catch (err) {
			console.error('Error acknowledging alert:', err);
		}
	}

	function getVarianceColor(variance: number | undefined): string {
		if (!variance) return 'text-gray-600';
		if (variance < 10) return 'text-yellow-600';
		if (variance < 30) return 'text-orange-600';
		return 'text-red-600';
	}

	onMount(() => {
		fetchAlerts();
	});
</script>

<div class="cost-alerts">
	<div class="header">
		<h3>Cost Alerts</h3>
		{#if !loading && alerts.length > 0}
			<span class="alert-count">{alerts.filter((a) => !a.acknowledged).length} unacknowledged</span>
		{/if}
	</div>

	{#if loading}
		<div class="loading">Loading alerts...</div>
	{:else if error}
		<div class="error">Error: {error}</div>
	{:else if alerts.length === 0}
		<div class="empty">No cost alerts</div>
	{:else}
		<div class="alerts-list">
			{#each alerts as alert}
				<div class="alert-card" class:acknowledged={alert.acknowledged}>
					<div class="alert-header">
						<div class="alert-type">
							<span class="icon">⚠️</span>
							<span class="type-text">{alert.type}</span>
						</div>
						{#if alert.variance !== undefined}
							<div class="alert-variance {getVarianceColor(alert.variance)}">
								+{alert.variance.toFixed(1)}%
							</div>
						{/if}
					</div>

					<div class="alert-message">{alert.message}</div>

					{#if alert.aiExplanation}
						<div class="ai-explanation">
							<span class="ai-badge">✦ AI</span>
							{alert.aiExplanation}
						</div>
					{/if}

					<div class="alert-costs">
						<div class="cost-item">
							<span class="label">Estimated:</span>
							<span class="value">${(alert.estimatedCost ?? 0).toFixed(2)}</span>
						</div>
						<div class="cost-item">
							<span class="label">Actual:</span>
							<span class="value warning">${(alert.actualCost ?? 0).toFixed(2)}</span>
						</div>
					</div>

					{#if alert.serviceBreakdown && Object.keys(alert.serviceBreakdown).length > 0}
						<div class="service-breakdown">
							<h4>Service Breakdown</h4>
							<div class="services">
								{#each Object.entries(alert.serviceBreakdown) as [service, cost]}
									{@const costValue = typeof cost === 'number' ? cost : 0}
									<div class="service-item">
										<span class="service-name">{service}</span>
										<span class="service-cost">${costValue.toFixed(2)}</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}

					<div class="alert-footer">
						<span class="sent-at">
							{alert.sentAt ? new Date(alert.sentAt).toLocaleDateString() : 'Unknown'}
						</span>
						{#if !alert.acknowledged}
							<button class="acknowledge-btn" on:click={() => acknowledgeAlert(alert.id)}>
								Acknowledge
							</button>
						{:else}
							<span class="acknowledged-badge">✓ Acknowledged</span>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.cost-alerts {
		background: white;
		border-radius: 8px;
		padding: 20px;
		box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
	}

	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 20px;
	}

	.header h3 {
		margin: 0;
		font-size: 18px;
		font-weight: 600;
	}

	.alert-count {
		background: #fef3c7;
		color: #92400e;
		padding: 4px 12px;
		border-radius: 12px;
		font-size: 14px;
		font-weight: 500;
	}

	.loading,
	.error,
	.empty {
		text-align: center;
		padding: 40px 20px;
		color: #6b7280;
	}

	.error {
		color: #dc2626;
	}

	.alerts-list {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}

	.alert-card {
		border: 2px solid #fbbf24;
		border-radius: 8px;
		padding: 16px;
		background: #fffbeb;
	}

	.alert-card.acknowledged {
		border-color: #d1d5db;
		background: #f9fafb;
		opacity: 0.7;
	}

	.alert-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 12px;
	}

	.alert-type {
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.icon {
		font-size: 20px;
	}

	.type-text {
		font-weight: 600;
		text-transform: capitalize;
	}

	.alert-variance {
		font-size: 18px;
		font-weight: 700;
	}

	.alert-message {
		margin-bottom: 16px;
		color: #374151;
	}

	.ai-explanation {
		background: linear-gradient(135deg, #f5f3ff, #ede9fe);
		border: 1px solid #c4b5fd;
		border-radius: 8px;
		padding: 10px 14px;
		margin-bottom: 14px;
		font-size: 0.82rem;
		color: #4c1d95;
		line-height: 1.5;
	}

	.ai-badge {
		display: inline-block;
		background: #7c3aed;
		color: white;
		font-size: 0.65rem;
		font-weight: 700;
		padding: 1px 6px;
		border-radius: 4px;
		margin-right: 6px;
		letter-spacing: 0.05em;
	}

	:global(.dark) .ai-explanation {
		background: linear-gradient(135deg, #2e1065, #1e1b4b);
		border-color: #6d28d9;
		color: #ddd6fe;
	}

	.alert-costs {
		display: flex;
		gap: 24px;
		margin-bottom: 16px;
	}

	.cost-item {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.cost-item .label {
		font-size: 12px;
		color: #6b7280;
		font-weight: 500;
	}

	.cost-item .value {
		font-size: 16px;
		font-weight: 600;
	}

	.cost-item .value.warning {
		color: #dc2626;
	}

	.service-breakdown {
		margin-top: 16px;
		padding-top: 16px;
		border-top: 1px solid #e5e7eb;
	}

	.service-breakdown h4 {
		font-size: 14px;
		font-weight: 600;
		margin: 0 0 12px 0;
	}

	.services {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.service-item {
		display: flex;
		justify-content: space-between;
		padding: 8px;
		background: white;
		border-radius: 4px;
	}

	.service-name {
		color: #4b5563;
	}

	.service-cost {
		font-weight: 600;
	}

	.alert-footer {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-top: 16px;
		padding-top: 16px;
		border-top: 1px solid #e5e7eb;
	}

	.sent-at {
		font-size: 13px;
		color: #6b7280;
	}

	.acknowledge-btn {
		background: #2563eb;
		color: white;
		border: none;
		padding: 8px 16px;
		border-radius: 6px;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.2s;
	}

	.acknowledge-btn:hover {
		background: #1d4ed8;
	}

	.acknowledged-badge {
		color: #059669;
		font-weight: 500;
		font-size: 14px;
	}
</style>
