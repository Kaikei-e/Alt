<script lang="ts">
import type { FeatureFlagsConfigData } from "$lib/connect/knowledge_home_admin";

let { flags }: { flags: FeatureFlagsConfigData | null } = $props();

const flagItems = $derived(
	flags
		? [
				{ label: "Home Page", enabled: flags.enableHomePage },
				{ label: "Tracking", enabled: flags.enableTracking },
				{ label: "Projection V2", enabled: flags.enableProjectionV2 },
				{ label: "Recall Rail", enabled: flags.enableRecallRail },
				{ label: "Lens", enabled: flags.enableLens },
				{ label: "Stream Updates", enabled: flags.enableStreamUpdates },
				{ label: "Supersede UX", enabled: flags.enableSupersedeUx },
			]
		: [],
);
</script>

<div class="panel" data-role="feature-flags">
	<h3 class="section-heading">Feature Flags</h3>
	<div class="heading-rule"></div>

	{#if !flags}
		<div class="loading-state">
			<span class="loading-pulse"></span>
			<span class="loading-text">Loading flags</span>
		</div>
	{:else}
		<div class="flag-list">
			{#each flagItems as item}
				<div class="flag-row">
					<span class="flag-label">{item.label}</span>
					<span class="flag-dot" class:on={item.enabled}></span>
				</div>
			{/each}
			<div class="flag-row">
				<span class="flag-label">Rollout %</span>
				<span class="flag-value">{flags.rolloutPercentage}%</span>
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

	.flag-list {
		display: flex;
		flex-direction: column;
		gap: 0;
	}

	.flag-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.flag-row:last-child {
		border-bottom: none;
	}

	.flag-label {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-charcoal);
	}

	.flag-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--surface-border);
	}

	.flag-dot.on {
		background: var(--alt-sage);
	}

	.flag-value {
		font-family: var(--font-mono);
		font-size: 0.8rem;
		font-weight: 600;
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
