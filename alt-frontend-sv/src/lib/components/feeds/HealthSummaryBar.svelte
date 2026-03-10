<script lang="ts">
import type { FeedLink, FeedHealthStatus } from "$lib/schema/feedLink";
import {
	summarizeHealth,
	getHealthColor,
	getHealthLabel,
} from "$lib/utils/feedHealth";

interface Props {
	feeds: FeedLink[];
}

const { feeds }: Props = $props();

const counts = $derived(summarizeHealth(feeds));

const visibleStatuses = $derived(
	(
		["healthy", "warning", "error", "inactive", "unknown"] as FeedHealthStatus[]
	).filter((s) => counts[s] > 0),
);
</script>

{#if feeds.length > 0}
	<div
		class="health-summary-bar"
		style="
			background: var(--surface-bg);
			border-color: var(--surface-border);
		"
		aria-label="Feed health summary"
	>
		<span class="summary-total" style="color: var(--text-secondary);">
			{feeds.length} feeds
		</span>
		<span class="summary-separator" style="color: var(--surface-border);">—</span>
		<span class="summary-counts">
			{#each visibleStatuses as status}
				<span class="summary-item">
					<svg
						width="8"
						height="8"
						viewBox="0 0 8 8"
						aria-hidden="true"
						style="color: {getHealthColor(status)};"
					>
						<circle cx="4" cy="4" r="3.5" fill="currentColor" />
					</svg>
					<span style="color: var(--text-secondary);">
						{counts[status]} {getHealthLabel(status).toLowerCase()}
					</span>
				</span>
			{/each}
		</span>
	</div>
{/if}

<style>
	.health-summary-bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0.75rem;
		border-radius: 0.5rem;
		border: 1px solid;
		font-size: 0.75rem;
		flex-wrap: wrap;
	}

	.summary-total {
		font-weight: 600;
	}

	.summary-counts {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.summary-item {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
	}
</style>
