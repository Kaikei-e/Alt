<script lang="ts">
let {
	funnel,
}: {
	funnel: { label: string; value: number }[];
} = $props();

const maxValue = $derived(
	funnel.length > 0 ? Math.max(...funnel.map((f) => f.value)) : 1,
);
</script>

<div class="panel" data-role="interaction-funnel">
	<h3 class="section-heading">Interaction Funnel</h3>
	<div class="heading-rule"></div>

	{#if funnel.length === 0}
		<p class="empty-text">No data available.</p>
	{:else}
		<div class="funnel-list">
			{#each funnel as step}
				<div class="funnel-row">
					<span class="funnel-label">{step.label}</span>
					<div class="funnel-bar-container">
						<div
							class="funnel-bar"
							style="width: {maxValue > 0 ? (step.value / maxValue) * 100 : 0}%; min-width: 20px;"
						></div>
					</div>
					<span class="funnel-value">{step.value.toLocaleString()}</span>
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

	.funnel-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.funnel-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.funnel-label {
		width: 5rem;
		text-align: right;
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
		flex-shrink: 0;
	}

	.funnel-bar-container {
		flex: 1;
		height: 16px;
	}

	.funnel-bar {
		height: 100%;
		background: var(--alt-primary);
		transition: width 0.3s ease;
	}

	.funnel-value {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		flex-shrink: 0;
		min-width: 3rem;
	}
</style>
