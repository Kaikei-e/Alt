<script lang="ts">
import PulseHeader from "#lib/components/pulse/PulseHeader.svelte";
import type { EveningPulse } from "#lib/schema/evening_pulse.js";
import MobilePulseCard from "./MobilePulseCard.svelte";

interface Props {
	pulse: EveningPulse;
	onTopicClick?: (clusterId: number) => void;
}

const { pulse, onTopicClick }: Props = $props();

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
	<PulseHeader date={pulse.generatedAt} {statusLabel} />

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
			class="p-6 border text-center"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<p style="color: var(--text-secondary);">No topics available</p>
		</div>
	{/if}
</div>
