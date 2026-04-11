<script lang="ts">
import AdminMetricCard from "./AdminMetricCard.svelte";

let {
	sovereign,
}: {
	sovereign: {
		mutationsApplied: number;
		mutationsErrors: number;
		mutationDurationMsP50: number;
		mutationDurationMsP95: number;
		errorRatePct: number;
	} | null;
} = $props();

const errorsStatus = $derived<"ok" | "warning" | "error" | "neutral">(
	!sovereign ? "neutral" : sovereign.mutationsErrors === 0 ? "ok" : "error",
);

const errorRateStatus = $derived<"ok" | "warning" | "error" | "neutral">(
	!sovereign
		? "neutral"
		: sovereign.errorRatePct === 0
			? "ok"
			: sovereign.errorRatePct < 5
				? "warning"
				: "error",
);
</script>

<div class="panel" data-role="sovereign-mutations">
	<h3 class="section-heading">Sovereign Mutations</h3>
	<div class="heading-rule"></div>

	{#if !sovereign}
		<div class="loading-state">
			<span class="loading-pulse"></span>
			<span class="loading-text">Loading sovereign data</span>
		</div>
	{:else}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
			<AdminMetricCard
				label="Applied"
				value={sovereign.mutationsApplied.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Errors"
				value={sovereign.mutationsErrors}
				status={errorsStatus}
			/>
			<AdminMetricCard
				label="Error Rate"
				value="{sovereign.errorRatePct.toFixed(1)}%"
				status={errorRateStatus}
			/>
		</div>

		<div class="percentile-row">
			<span class="percentile-label">Duration</span>
			<div class="percentile-values">
				<span class="percentile-item">
					<span class="percentile-key">P50</span>
					<span class="percentile-val">{sovereign.mutationDurationMsP50}ms</span>
				</span>
				<span class="percentile-item">
					<span class="percentile-key">P95</span>
					<span class="percentile-val">{sovereign.mutationDurationMsP95}ms</span>
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
