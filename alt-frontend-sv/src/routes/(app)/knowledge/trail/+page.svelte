<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import TrailBranches from "$lib/components/knowledge-trail/TrailBranches.svelte";
import TrailSearch from "$lib/components/knowledge-trail/TrailSearch.svelte";
import TrailSpine from "$lib/components/knowledge-trail/TrailSpine.svelte";
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

	<!-- The spine (the path the user has worn) is the hero. System-proposed
	     branches are secondary and rendered, capped, below it. While a search
	     is active, the spine shows only the matching episodes. -->
	<TrailSpine
		episodes={trail.searchActive ? trail.searchEpisodes : trail.episodes}
		loading={trail.searchActive ? trail.searching : trail.loading}
		hasMore={trail.searchActive ? false : trail.hasMore}
		hasEverLoaded={trail.searchActive ? true : trail.hasEverLoaded}
		matchedItemKeys={trail.searchActive ? trail.matchedItemKeys : []}
		searchActive={trail.searchActive}
		onLoadMore={() => trail.loadMore()}
	/>

	<TrailBranches
		branches={trail.branches}
		onResolve={trail.resolveBranch}
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
