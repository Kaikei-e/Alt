<script lang="ts">
import type { FeedHealthStatus } from "$lib/schema/feedLink";
import { getHealthColor, getHealthLabel } from "$lib/utils/feedHealth";

interface Props {
	status: FeedHealthStatus;
	lastFailureReason?: string;
	consecutiveFailures?: number;
	compact?: boolean;
}

const {
	status,
	lastFailureReason = "",
	consecutiveFailures = 0,
	compact = false,
}: Props = $props();

const color = $derived(getHealthColor(status));
const label = $derived(getHealthLabel(status));

const tooltipText = $derived.by(() => {
	if (status === "healthy") return "Feed is operating normally";
	if (status === "warning")
		return `${consecutiveFailures} consecutive failure(s). ${lastFailureReason}`.trim();
	if (status === "error")
		return `${consecutiveFailures} consecutive failures. ${lastFailureReason}`.trim();
	if (status === "inactive")
		return "Feed has been disabled due to persistent errors";
	return "No health data available";
});
</script>

<span
	class="feed-health-badge"
	title={tooltipText}
	aria-label="{label} status"
>
	<svg
		width="12"
		height="12"
		viewBox="0 0 12 12"
		aria-hidden="true"
		style="color: {color};"
	>
		{#if status === "healthy"}
			<circle cx="6" cy="6" r="5" fill="currentColor" />
		{:else if status === "warning"}
			<polygon points="6,1 11,11 1,11" fill="currentColor" />
		{:else if status === "error"}
			<rect x="1" y="1" width="10" height="10" fill="currentColor" />
		{:else if status === "inactive"}
			<circle
				cx="6"
				cy="6"
				r="4.5"
				fill="none"
				stroke="currentColor"
				stroke-width="1.5"
			/>
		{:else}
			<circle
				cx="6"
				cy="6"
				r="4.5"
				fill="none"
				stroke="currentColor"
				stroke-width="1.5"
				stroke-dasharray="3,2"
			/>
		{/if}
	</svg>
	{#if !compact}
		<span class="feed-health-label" style="color: {color};">
			{label}
		</span>
	{/if}
</span>

<style>
	.feed-health-badge {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		white-space: nowrap;
	}

	.feed-health-label {
		font-size: 0.6875rem;
		font-weight: 500;
		line-height: 1;
	}
</style>
