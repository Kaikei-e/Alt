<script lang="ts">
let {
	slis,
}: {
	slis: {
		name: string;
		currentValue: number;
		targetValue: number;
		errorBudgetConsumedPct: number;
		status: string;
	}[];
} = $props();

const formatSliName = (name: string) =>
	name
		.split("_")
		.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
		.join(" ");

const barColor = (pct: number) => {
	if (pct >= 80) return "var(--alt-terracotta)";
	if (pct >= 50) return "var(--alt-sand)";
	return "var(--alt-sage)";
};
</script>

<div class="panel" data-role="error-budget-burn-rate">
	<h3 class="section-heading">Error Budget Burn Rate</h3>
	<div class="heading-rule"></div>

	{#if slis.length === 0}
		<p class="empty-text">No SLI data available.</p>
	{:else}
		<div class="burn-list">
			{#each slis as sli (sli.name)}
				{@const clamped = Math.max(0, Math.min(100, sli.errorBudgetConsumedPct))}
				{@const color = barColor(sli.errorBudgetConsumedPct)}
				<div class="burn-item">
					<div class="burn-header">
						<span class="burn-name">{formatSliName(sli.name)}</span>
						<span class="burn-value" style="color: {color};">
							{clamped.toFixed(1)}%
						</span>
					</div>
					<div class="burn-track">
						<div
							class="burn-fill"
							style="width: {clamped}%; background: {color};"
						></div>
					</div>
				</div>
			{/each}
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

	.empty-text {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.burn-list {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.burn-item {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding: 0.6rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.burn-item:last-child {
		border-bottom: none;
	}

	.burn-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.burn-name {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--alt-charcoal);
	}

	.burn-value {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 700;
	}

	.burn-track {
		height: 2px;
		width: 100%;
		overflow: hidden;
		background: var(--surface-border);
	}

	.burn-fill {
		height: 100%;
		transition: width 0.3s ease;
	}
</style>
