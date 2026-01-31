<script lang="ts">
import type { PulseTopic } from "$lib/schema/evening_pulse";
import PulseRoleLabel from "$lib/components/pulse/PulseRoleLabel.svelte";
import PulseRationale from "$lib/components/pulse/PulseRationale.svelte";

interface Props {
	topic: PulseTopic;
	index: number;
	isFocused?: boolean;
	tabindex?: number;
	onclick?: () => void;
	onfocus?: () => void;
}

const {
	topic,
	index,
	isFocused = false,
	tabindex = 0,
	onclick,
	onfocus,
}: Props = $props();

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

// Format source names for display (e.g., "Reuters, BBC, +3 more")
const formattedSources = $derived.by(() => {
	const sources = topic.sourceNames ?? [];
	if (sources.length === 0) return "";
	if (sources.length <= 3) return sources.join(", ");
	return `${sources.slice(0, 2).join(", ")}, +${sources.length - 2} more`;
});
</script>

<button
	type="button"
	class="w-full text-left p-5 border-2 border-[var(--surface-border)] bg-white transition-all duration-200 cursor-pointer group focus:outline-2 focus:outline-[var(--alt-primary)]"
	class:shadow-md={isFocused}
	class:-translate-y-0.5={isFocused}
	style="border-left: 4px solid {cardBorderColor};"
	aria-label="Topic {index + 1}: {topic.title}"
	aria-describedby="rationale-{index}"
	{tabindex}
	onclick={onclick}
	onfocus={onfocus}
>
	<header class="flex items-start justify-between gap-2 mb-3">
		<div class="flex items-center gap-2">
			<span
				class="text-xs font-bold w-5 h-5 flex items-center justify-center rounded-full"
				style="background: {cardBorderColor}; color: white;"
				aria-hidden="true"
			>
				{index + 1}
			</span>
			<PulseRoleLabel role={topic.role} />
		</div>
		<span class="text-xs" style="color: var(--text-muted);">
			{topic.timeAgo}
		</span>
	</header>

	<h3
		id="topic-title-{index}"
		class="text-lg font-bold mb-2 leading-tight group-hover:text-[var(--alt-primary)] transition-colors"
		style="color: var(--text-primary);"
	>
		{topic.title}
	</h3>

	<!-- Top Entities -->
	{#if topic.topEntities && topic.topEntities.length > 0}
		<div class="flex flex-wrap gap-1.5 mb-3">
			{#each topic.topEntities.slice(0, 5) as entity}
				<span
					class="text-xs px-2 py-0.5 rounded-full"
					style="background: var(--surface-hover); color: var(--text-secondary);"
				>
					{entity}
				</span>
			{/each}
		</div>
	{/if}

	<!-- Representative Articles -->
	{#if topic.representativeArticles && topic.representativeArticles.length > 0}
		<ul class="space-y-1.5 mb-3 text-sm" style="color: var(--text-secondary);">
			{#each topic.representativeArticles.slice(0, 3) as article}
				<li class="flex items-start gap-2">
					<span class="text-[10px] mt-1" style="color: var(--text-muted);">-</span>
					<span class="flex-1 line-clamp-1">
						"{article.title}"
						<span class="text-xs" style="color: var(--text-muted);">
							- {article.sourceName}
						</span>
					</span>
				</li>
			{/each}
		</ul>
	{/if}

	<div id="rationale-{index}" class="mb-4">
		<PulseRationale rationale={topic.rationale} />
	</div>

	<footer class="flex items-center flex-wrap gap-3 text-xs" style="color: var(--text-muted);">
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
		{#if formattedSources}
			<span class="ml-auto" style="color: var(--text-tertiary);">
				{formattedSources}
			</span>
		{:else if topic.genre}
			<span class="ml-auto px-2 py-0.5" style="background: var(--surface-hover);">
				{topic.genre}
			</span>
		{/if}
	</footer>
</button>

<style>
	button:hover {
		transform: translateY(-2px);
		box-shadow: 0 4px 8px rgba(0, 0, 0, 0.12);
	}

	@media (prefers-reduced-motion: reduce) {
		button {
			transition: none;
		}
		button:hover {
			transform: none;
		}
	}
</style>
