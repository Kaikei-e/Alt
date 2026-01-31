<script lang="ts">
import type { PulseTopic } from "$lib/schema/evening_pulse";
import PulseRoleLabel from "$lib/components/pulse/PulseRoleLabel.svelte";

interface Props {
	topic: PulseTopic;
	index: number;
	onclick?: () => void;
}

const { topic, index, onclick }: Props = $props();

const cardBorderColor = $derived.by(() => {
	switch (topic.role) {
		case "need_to_know":
			return "var(--pulse-need-to-know-text, hsl(0 84% 60%))";
		case "trend":
			return "var(--pulse-trend-text, hsl(45 93% 47%))";
		case "serendipity":
			return "var(--pulse-serendipity-text, hsl(262 83% 58%))";
		default:
			return "var(--border)";
	}
});
</script>

<button
	type="button"
	class="w-full text-left p-4 rounded-2xl border-2 transition-all duration-200 hover:shadow-md active:scale-[0.99]"
	style="background: var(--surface-bg); border-color: {cardBorderColor};"
	aria-labelledby="topic-title-{index}"
	{onclick}
>
	<header class="flex items-start justify-between gap-2 mb-3">
		<PulseRoleLabel role={topic.role} />
		<span class="text-xs" style="color: var(--text-tertiary);">
			{topic.timeAgo}
		</span>
	</header>

	<h3
		id="topic-title-{index}"
		class="text-lg font-bold mb-2 leading-tight"
		style="color: var(--text-primary);"
	>
		{topic.title}
	</h3>

	<p
		class="text-sm mb-3 leading-relaxed"
		style="color: var(--text-secondary);"
	>
		{topic.rationale.text}
	</p>

	<footer class="flex items-center gap-3 text-xs" style="color: var(--text-tertiary);">
		<span>{topic.articleCount} articles</span>
		<span>{topic.sourceCount} sources</span>
		{#if topic.tier1Count}
			<span class="font-medium" style="color: var(--pulse-need-to-know-text, hsl(0 84% 60%));">
				Tier1: {topic.tier1Count}
			</span>
		{/if}
		{#if topic.trendMultiplier}
			<span class="font-medium" style="color: var(--pulse-trend-text, hsl(45 93% 47%));">
				{topic.trendMultiplier.toFixed(1)}x
			</span>
		{/if}
		{#if topic.genre}
			<span class="ml-auto px-2 py-0.5 rounded" style="background: var(--surface-hover);">
				{topic.genre}
			</span>
		{/if}
	</footer>
</button>
