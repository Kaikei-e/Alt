<script lang="ts">
let { distribution }: { distribution: { code: string; count: number }[] } =
	$props();

const maxCount = $derived(
	distribution.length > 0 ? Math.max(...distribution.map((d) => d.count)) : 1,
);
</script>

<div class="panel" data-role="reason-distribution">
	<h3 class="section-heading">Why Reason Distribution</h3>
	<div class="heading-rule"></div>

	{#if distribution.length === 0}
		<p class="empty-text">No data available.</p>
	{:else}
		<div class="dist-list">
			{#each distribution as item}
				<div class="dist-row">
					<span class="dist-code">{item.code}</span>
					<div class="dist-bar-container">
						<div
							class="dist-bar"
							style="width: {(item.count / maxCount) * 100}%; min-width: 4px;"
						></div>
					</div>
					<span class="dist-count">{item.count}</span>
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

	.dist-list {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.dist-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.dist-code {
		width: 8rem;
		text-align: right;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-slate);
		flex-shrink: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.dist-bar-container {
		flex: 1;
		height: 12px;
	}

	.dist-bar {
		height: 100%;
		background: var(--alt-primary);
		transition: width 0.3s ease;
	}

	.dist-count {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		flex-shrink: 0;
		min-width: 2.5rem;
	}
</style>
