<script lang="ts">
import PulseHeader from "#lib/components/pulse/PulseHeader.svelte";
import type { EveningPulse } from "#lib/schema/evening_pulse.js";
import DesktopPulseCard from "./DesktopPulseCard.svelte";

interface Props {
	pulse: EveningPulse;
	onTopicClick?: (clusterId: number) => void;
	onNavigateToRecap?: () => void;
}

const { pulse, onTopicClick, onNavigateToRecap }: Props = $props();

let focusedIndex = $state<number | null>(null);
let containerRef = $state<HTMLElement | null>(null);

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

const handleKeydown = (e: KeyboardEvent) => {
	const topicCount = pulse.topics.length;
	if (topicCount === 0) return;

	if (e.key >= "1" && e.key <= "3") {
		const index = Number.parseInt(e.key, 10) - 1;
		if (index < topicCount) {
			e.preventDefault();
			focusedIndex = index;
			focusCard(index);
		}
		return;
	}

	if (e.key === "Tab") {
		if (focusedIndex === null) {
			focusedIndex = e.shiftKey ? topicCount - 1 : 0;
		}
		return;
	}
};

const focusCard = (index: number) => {
	if (!containerRef) return;
	const cards = containerRef.querySelectorAll("button[aria-label^='Topic']");
	const card = cards[index] as HTMLElement | undefined;
	card?.focus();
};

const handleCardClick = (clusterId: number, index: number) => {
	focusedIndex = index;
	onTopicClick?.(clusterId);
};

const handleCardFocus = (index: number) => {
	focusedIndex = index;
};
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
	class="p-6 max-w-7xl mx-auto"
	bind:this={containerRef}
	onkeydown={handleKeydown}
	role="region"
	aria-label="Evening Pulse topics"
>
	<PulseHeader
		date={pulse.generatedAt}
		{statusLabel}
		showKeyboardHint={pulse.topics.length > 0}
		{onNavigateToRecap}
	/>

	{#if pulse.topics.length > 0}
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
			{#each pulse.topics as topic, index (topic.clusterId)}
				<DesktopPulseCard
					{topic}
					{index}
					isFocused={focusedIndex === index}
					tabindex={focusedIndex === null ? (index === 0 ? 0 : -1) : (focusedIndex === index ? 0 : -1)}
					onclick={() => handleCardClick(topic.clusterId, index)}
					onfocus={() => handleCardFocus(index)}
				/>
			{/each}
		</div>
	{:else}
		<div
			class="p-8 border text-center"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<p style="color: var(--text-secondary);">No topics available</p>
		</div>
	{/if}
</div>
