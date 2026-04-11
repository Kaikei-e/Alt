<script lang="ts">
import AdminMetricCard from "./AdminMetricCard.svelte";

let {
	projector,
}: {
	projector: {
		eventsProcessed: number;
		lagSeconds: number;
		batchDurationMsP50: number;
		batchDurationMsP95: number;
		batchDurationMsP99: number;
		errors: number;
	} | null;
} = $props();

const lagStatus = $derived<"ok" | "warning" | "error" | "neutral">(
	!projector
		? "neutral"
		: projector.lagSeconds < 60
			? "ok"
			: projector.lagSeconds < 300
				? "warning"
				: "error",
);

const errorsStatus = $derived<"ok" | "warning" | "error" | "neutral">(
	!projector ? "neutral" : projector.errors === 0 ? "ok" : "error",
);
</script>

<div class="panel" data-role="projector-pipeline">
	<h3 class="section-heading">Projector Pipeline</h3>
	<div class="heading-rule"></div>

	{#if !projector}
		<div class="loading-state">
			<span class="loading-pulse"></span>
			<span class="loading-text">Loading projector data</span>
		</div>
	{:else}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
			<AdminMetricCard
				label="Events Processed"
				value={projector.eventsProcessed.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Lag"
				value="{projector.lagSeconds}s"
				status={lagStatus}
			/>
			<AdminMetricCard
				label="Errors"
				value={projector.errors}
				status={errorsStatus}
			/>
		</div>

		<div class="percentile-row">
			<span class="percentile-label">Batch Duration</span>
			<div class="percentile-values">
				<span class="percentile-item">
					<span class="percentile-key">P50</span>
					<span class="percentile-val">{projector.batchDurationMsP50}ms</span>
				</span>
				<span class="percentile-item">
					<span class="percentile-key">P95</span>
					<span class="percentile-val">{projector.batchDurationMsP95}ms</span>
				</span>
				<span class="percentile-item">
					<span class="percentile-key">P99</span>
					<span class="percentile-val">{projector.batchDurationMsP99}ms</span>
				</span>
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

	.percentile-row {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.percentile-label {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.percentile-values {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.percentile-item {
		display: flex;
		align-items: baseline;
		gap: 0.25rem;
	}

	.percentile-key {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		color: var(--alt-ash);
	}

	.percentile-val {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		font-weight: 700;
		color: var(--alt-charcoal);
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
