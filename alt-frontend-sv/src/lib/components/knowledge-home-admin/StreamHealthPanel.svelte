<script lang="ts">
import AdminMetricCard from "./AdminMetricCard.svelte";

let {
	stream,
}: {
	stream: {
		connectionsTotal: number;
		disconnectsTotal: number;
		reconnectsTotal: number;
		deliveriesTotal: number;
		disconnectRatePct: number;
	} | null;
} = $props();

const clampedPct = $derived(
	stream ? Math.max(0, Math.min(100, stream.disconnectRatePct)) : 0,
);

const barColor = $derived(
	clampedPct >= 20
		? "var(--alt-terracotta)"
		: clampedPct >= 5
			? "var(--alt-sand)"
			: "var(--alt-sage)",
);
</script>

<div class="panel" data-role="stream-health">
	<h3 class="section-heading">Stream Health</h3>
	<div class="heading-rule"></div>

	{#if !stream}
		<div class="loading-state">
			<span class="loading-pulse"></span>
			<span class="loading-text">Loading stream data</span>
		</div>
	{:else}
		<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
			<AdminMetricCard
				label="Connections"
				value={stream.connectionsTotal.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Disconnects"
				value={stream.disconnectsTotal.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Reconnects"
				value={stream.reconnectsTotal.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Deliveries"
				value={stream.deliveriesTotal.toLocaleString()}
				status="neutral"
			/>
		</div>

		<div class="rate-container">
			<div class="rate-header">
				<span class="rate-label">Disconnect Rate</span>
				<span class="rate-value" style="color: {barColor};">
					{clampedPct.toFixed(1)}%
				</span>
			</div>
			<div class="rate-track">
				<div
					class="rate-fill"
					style="width: {clampedPct}%; background: {barColor};"
				></div>
			</div>
		</div>
	{/if}
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.section-heading {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.heading-rule {
		height: 1px;
		background: var(--surface-border);
		margin-bottom: 0.25rem;
	}

	.rate-container {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.rate-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.rate-label {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.rate-value {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 700;
	}

	.rate-track {
		height: 2px;
		width: 100%;
		overflow: hidden;
		background: var(--surface-border);
	}

	.rate-fill {
		height: 100%;
		transition: width 0.3s ease;
	}

	.loading-state {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}
</style>
