<script lang="ts">
import PulseRoleLabel from "#lib/components/pulse/PulseRoleLabel.svelte";
import type { PulseTopic } from "#lib/schema/evening_pulse.js";

interface Props {
	topic: PulseTopic;
	index: number;
	onclick?: () => void;
}

const { topic, index, onclick }: Props = $props();

const cardBorderColor = $derived.by(() => {
	switch (topic.role) {
		case "need_to_know":
			return "var(--accent-emphasis-text)";
		case "trend":
			return "var(--accent-muted-text)";
		case "serendipity":
			return "var(--accent-info-text)";
		default:
			return "var(--surface-border)";
	}
});

const leadArticle = $derived(topic.representativeArticles?.[0] ?? null);

const primaryTitle = $derived(leadArticle?.title?.trim() || topic.title);
</script>

<button
	type="button"
	class="w-full text-left p-4 border transition-colors duration-150 cursor-pointer"
	style="background: var(--surface-bg); border-color: var(--surface-border); border-left: 3px solid {cardBorderColor};"
	aria-label="Topic {index + 1}: {primaryTitle} — {topic.title}"
	{onclick}
>
	<header class="flex items-start justify-between gap-2 mb-2">
		<div
			class="flex flex-wrap items-center gap-2 min-w-0"
			data-testid="pulse-card-kicker"
		>
			<PulseRoleLabel role={topic.role} />
			<span
				class="text-xs uppercase tracking-wider font-mono truncate"
				style="color: var(--text-muted);"
				data-testid="pulse-cluster-kicker"
			>
				{topic.title}
			</span>
		</div>
		<span class="text-xs font-mono shrink-0" style="color: var(--text-muted);">
			{topic.timeAgo}
		</span>
	</header>

	<h3
		id="topic-title-{index}"
		class="text-lg font-bold mb-1.5 leading-snug"
		style="color: var(--text-primary); font-family: var(--font-display);"
	>
		{primaryTitle}
	</h3>

	{#if leadArticle?.sourceName}
		<p class="text-xs font-mono mb-2" style="color: var(--text-muted);">
			{leadArticle.sourceName}
		</p>
	{/if}

	{#if topic.topEntities && topic.topEntities.length > 0}
		<div class="flex flex-wrap gap-1 mb-2">
			{#each topic.topEntities.slice(0, 3) as entity}
				<span
					class="text-[11px] px-1.5 py-0.5 border font-mono"
					style="background: var(--chip-bg); border-color: var(--chip-border); color: var(--chip-text);"
				>
					{entity}
				</span>
			{/each}
		</div>
	{/if}

	<p
		class="text-sm mb-3 leading-relaxed line-clamp-2"
		style="color: var(--text-secondary);"
	>
		{topic.rationale.text}
	</p>

	<footer
		class="flex items-center flex-wrap gap-3 pt-2 border-t text-xs font-mono"
		style="color: var(--text-muted); border-color: var(--surface-border);"
	>
		<span>{topic.articleCount} articles</span>
		<span>{topic.sourceCount} sources</span>
		{#if topic.tier1Count}
			<span class="font-semibold" style="color: var(--accent-emphasis-text);">
				Tier1: {topic.tier1Count}
			</span>
		{/if}
		{#if topic.trendMultiplier}
			<span class="font-semibold" style="color: var(--accent-muted-text);">
				{topic.trendMultiplier.toFixed(1)}x
			</span>
		{/if}
		{#if topic.genre}
			<span
				class="ml-auto px-2 py-0.5 border"
				style="background: var(--surface-2); border-color: var(--surface-border); color: var(--text-secondary);"
			>
				{topic.genre}
			</span>
		{/if}
	</footer>
</button>
