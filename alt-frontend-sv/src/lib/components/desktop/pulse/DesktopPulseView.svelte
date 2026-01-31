<script lang="ts">
import type { EveningPulse } from "$lib/schema/evening_pulse";
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

	// Handle number keys 1-3 for direct navigation
	if (e.key >= "1" && e.key <= "3") {
		const index = Number.parseInt(e.key) - 1;
		if (index < topicCount) {
			e.preventDefault();
			focusedIndex = index;
			focusCard(index);
		}
		return;
	}

	// Handle Tab navigation
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
	<header class="mb-8">
		<div class="flex items-end justify-between gap-4 mb-2">
			<div>
				<h1 class="text-3xl font-bold" style="color: var(--text-primary);">
					Evening Pulse
				</h1>
				<p class="text-sm mt-1" style="color: var(--text-secondary);">
					{#if pulse.generatedAt}
						{@const d = new Date(pulse.generatedAt)}
						{d.toLocaleDateString("en-US", {
							month: "long",
							day: "numeric",
							weekday: "long",
						})}
					{/if}
					{#if statusLabel}
						<span class="ml-2 px-2 py-0.5 text-xs" style="background: var(--surface-hover);">
							{statusLabel}
						</span>
					{/if}
				</p>
			</div>
			{#if onNavigateToRecap}
				<button
					type="button"
					class="text-sm px-4 py-2 border border-[var(--surface-border)] hover:bg-[var(--surface-hover)] transition-colors"
					onclick={onNavigateToRecap}
				>
					View 7-Day Recap
				</button>
			{/if}
		</div>
		<p class="text-xs" style="color: var(--text-muted);">
			Press 1, 2, or 3 to quickly navigate to topics
		</p>
	</header>

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
			class="p-8 border-2 border-[var(--surface-border)] text-center"
			style="background: var(--surface-bg);"
		>
			<p style="color: var(--text-secondary);">No topics available</p>
		</div>
	{/if}
</div>
