<script lang="ts">
import type { EveningPulse } from "$lib/schema/evening_pulse";
import MobilePulseCard from "./MobilePulseCard.svelte";

interface Props {
	pulse: EveningPulse;
	onTopicClick?: (clusterId: number) => void;
}

const { pulse, onTopicClick }: Props = $props();

const formattedDate = $derived.by(() => {
	const date = new Date(pulse.generatedAt);
	return date.toLocaleDateString("en-US", {
		month: "short",
		day: "numeric",
		weekday: "short",
	});
});

const statusLabel = $derived.by(() => {
	switch (pulse.status) {
		case "normal":
			return "3 topics";
		case "partial":
			return `${pulse.topics.length} topic${pulse.topics.length > 1 ? "s" : ""}`;
		default:
			return "";
	}
});
</script>

<div class="p-4 max-w-2xl mx-auto pb-24">
	<header class="mb-6">
		<h1
			class="text-2xl font-bold mb-1"
			style="color: var(--text-primary);"
		>
			Evening Pulse
		</h1>
		<p class="text-sm" style="color: var(--text-secondary);">
			{formattedDate}
			{#if statusLabel}
				<span class="ml-2 px-2 py-0.5 rounded-full text-xs" style="background: var(--surface-hover);">
					{statusLabel}
				</span>
			{/if}
		</p>
	</header>

	<div class="flex flex-col gap-4">
		{#each pulse.topics as topic, index (topic.clusterId)}
			<MobilePulseCard
				{topic}
				{index}
				onclick={() => onTopicClick?.(topic.clusterId)}
			/>
		{/each}
	</div>

	{#if pulse.topics.length === 0 && pulse.status !== "quiet_day"}
		<div
			class="p-6 rounded-2xl text-center"
			style="background: var(--surface-bg);"
		>
			<p style="color: var(--text-secondary);">No topics available</p>
		</div>
	{/if}
</div>
