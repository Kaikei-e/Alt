<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import TrailSearch from "$lib/components/knowledge-trail/TrailSearch.svelte";
import TrailSpine from "$lib/components/knowledge-trail/TrailSpine.svelte";
import type { BranchData } from "$lib/connect/knowledge_trail";
import { useKnowledgeTrail } from "$lib/hooks/useKnowledgeTrail.svelte";

const trail = useKnowledgeTrail();

const dateStr = new Date().toLocaleDateString([], {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

onMount(() => {
	if (browser) {
		void trail.fetchData(true);
	}
});

const activeEpisodes = $derived(
	trail.searchActive ? trail.searchEpisodes : trail.episodes,
);

// Wave 10 (D26/D28): the top-of-trail branch inbox is removed. At most one
// branch surfaces per episode, subordinate to its header, matched by anchor
// membership — first match only, one per episode. A branch whose anchor
// isn't a member of any currently-shown episode is not rendered anywhere;
// its real stage is the article read-end (ArticleEndBranches), not the trail.
const branchByEpisodeKey = $derived.by(() => {
	const map = new Map<string, BranchData>();
	for (const episode of activeEpisodes) {
		const memberKeys = new Set(episode.footprints.map((fp) => fp.itemKey));
		const match = trail.branches.find((b) => memberKeys.has(b.anchorItemKey));
		if (match) map.set(episode.episodeKey, match);
	}
	return map;
});
</script>

<svelte:head>
	<title>Your Trail — Alt</title>
</svelte:head>

<div class="content">
	<header class="desk">
		<span class="desk-date">{dateStr} — Field Notes</span>
		<h1 class="desk-title">Your Trail</h1>
		<p class="desk-subtitle">
			The path you have worn through what you read.
		</p>
		<div class="desk-rule" aria-hidden="true"></div>
	</header>

	<div class="trailhead-actions">
		<button
			class="action-link"
			data-testid="trail-refresh"
			onclick={() => trail.refresh()}
			disabled={trail.loading}
		>
			Refresh trail
		</button>
		<a class="action-link" href="/home">Back to Home</a>
	</div>

	{#if trail.error && !trail.hasEverLoaded}
		<p class="trail-error" data-testid="trail-error">
			The trail could not be loaded. Try refreshing.
		</p>
	{/if}

	<!-- Trail search (D25/Wave 9): the sole rediscovery instrument, pull-only —
	     fetches only on explicit submit, never a keystroke or an $effect. -->
	<TrailSearch
		active={trail.searchActive}
		searching={trail.searching}
		onSearch={(q) => trail.search(q)}
		onClear={() => trail.clearSearch()}
	/>

	<!-- The spine (the path the user has worn) is the hero. There is no
	     top-of-trail branch inbox (D26/D28): at most one branch surfaces per
	     episode, subordinate to its header. While a search is active, the
	     spine shows only the matching episodes. -->
	<TrailSpine
		episodes={activeEpisodes}
		loading={trail.searchActive ? trail.searching : trail.loading}
		hasMore={trail.searchActive ? false : trail.hasMore}
		hasEverLoaded={trail.searchActive ? true : trail.hasEverLoaded}
		matchedItemKeys={trail.searchActive ? trail.matchedItemKeys : []}
		searchActive={trail.searchActive}
		onLoadMore={() => trail.loadMore()}
		{branchByEpisodeKey}
		onResolveBranch={trail.resolveBranch}
	/>
</div>

<style>
	.content {
		padding: 1.75rem 2.25rem 3rem;
		max-width: 1100px;
	}
	.desk-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash, #999);
		letter-spacing: 0.06em;
		text-transform: uppercase;
	}
	.desk-title {
		font-family: var(--font-display);
		font-size: 1.85rem;
		font-weight: 800;
		letter-spacing: -0.01em;
		line-height: 1.15;
		margin-top: 0.2rem;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.desk-subtitle {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash, #999);
		margin-top: 0.25rem;
		font-style: italic;
	}
	.desk-rule {
		height: 1px;
		background: var(--surface-border, #c8c8c8);
		margin: 0.9rem 0 0;
	}
	.trailhead-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 1.1rem;
	}
	.action-link {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		border: 1px solid var(--chip-border, #d0c8bb);
		background: var(--action-surface, #ebe8e1);
		padding: 0.45rem 0.85rem;
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-weight: 500;
		color: var(--interactive-text, #2f4f4f);
		cursor: pointer;
		text-decoration: none;
	}
	.action-link:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.trail-error {
		margin-top: 1rem;
		font-family: var(--font-body);
		font-size: 0.9rem;
		color: var(--accent-emphasis-text, #8c1d1d);
	}
</style>
