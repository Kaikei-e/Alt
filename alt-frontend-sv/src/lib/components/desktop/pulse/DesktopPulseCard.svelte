<script lang="ts">
import PulseRationale from "#lib/components/pulse/PulseRationale.svelte";
import PulseRoleLabel from "#lib/components/pulse/PulseRoleLabel.svelte";
import type { PulseTopic } from "#lib/schema/evening_pulse.js";

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

const secondaryArticles = $derived(
	(topic.representativeArticles ?? []).slice(1, 3),
);

const formattedSources = $derived.by(() => {
	const sources = topic.sourceNames ?? [];
	if (sources.length === 0) return "";
	if (sources.length <= 3) return sources.join(", ");
	return `${sources.slice(0, 2).join(", ")}, +${sources.length - 2} more`;
});
</script>

<button
	type="button"
	class="w-full text-left p-5 border transition-colors duration-150 cursor-pointer group bg-[var(--surface-bg)] hover:bg-[var(--surface-hover)] data-[focused=true]:bg-[var(--surface-hover)] focus:outline-2 focus:outline-[var(--alt-primary)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--alt-primary)]"
	class:bg-[var(--surface-hover)]={isFocused}
	data-focused={isFocused ? "true" : undefined}
	style="border-color: var(--surface-border); border-left: 3px solid {cardBorderColor};"
	aria-label="Topic {index + 1}: {primaryTitle} — {topic.title}"
	aria-describedby="rationale-{index}"
	{tabindex}
	{onclick}
	{onfocus}
>
	<header class="flex items-start justify-between gap-2 mb-3">
		<div
			class="flex flex-wrap items-center gap-2 min-w-0"
			data-testid="pulse-card-kicker"
		>
			<span
				class="text-xs font-bold px-1.5 py-0.5 border font-mono"
				style="background: var(--surface-2); border-color: var(--surface-border); color: var(--text-primary);"
				aria-hidden="true"
			>
				#{index + 1}
			</span>
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
		class="text-lg font-bold mb-2 leading-snug text-[var(--text-primary)] group-hover:text-[var(--interactive-text)] transition-colors"
		style="font-family: var(--font-display);"
	>
		{primaryTitle}
	</h3>

	{#if leadArticle?.sourceName}
		<p class="text-xs font-mono mb-3" style="color: var(--text-muted);">
			{leadArticle.sourceName}
		</p>
	{/if}

	{#if topic.topEntities && topic.topEntities.length > 0}
		<div class="flex flex-wrap gap-1.5 mb-3">
			{#each topic.topEntities.slice(0, 5) as entity}
				<span
					class="text-xs px-2 py-0.5 border font-mono"
					style="background: var(--chip-bg); border-color: var(--chip-border); color: var(--chip-text);"
				>
					{entity}
				</span>
			{/each}
		</div>
	{/if}

	{#if secondaryArticles.length > 0}
		<ul
			class="space-y-1.5 mb-3 pt-2 border-t text-sm"
			style="color: var(--text-secondary); border-color: var(--surface-border);"
		>
			{#each secondaryArticles as article}
				<li class="flex items-start gap-2">
					<span class="text-xs font-mono mt-0.5" style="color: var(--text-muted);">—</span>
					<span class="flex-1 line-clamp-1">
						{article.title}
						<span class="text-xs font-mono" style="color: var(--text-muted);">
							({article.sourceName})
						</span>
					</span>
				</li>
			{/each}
		</ul>
	{/if}

	<div id="rationale-{index}" class="mb-4">
		<PulseRationale rationale={topic.rationale} />
	</div>

	<footer
		class="flex items-center flex-wrap gap-3 pt-3 border-t text-xs font-mono"
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
		{#if formattedSources}
			<span class="ml-auto" style="color: var(--text-muted);">
				{formattedSources}
			</span>
		{:else if topic.genre}
			<span
				class="ml-auto px-2 py-0.5 border"
				style="background: var(--surface-2); border-color: var(--surface-border); color: var(--text-secondary);"
			>
				{topic.genre}
			</span>
		{/if}
	</footer>
</button>
