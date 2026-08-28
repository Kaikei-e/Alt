<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import type { SearchQuery } from "$lib/schema/search";
import SearchResults from "./SearchResults.svelte";
import SearchWindow from "./SearchWindow.svelte";
import type { MobileSearchSession } from "./search-session";

interface Props {
	initialQuery?: string;
	/** The query text, owned by the route so both viewports share one. */
	query: string;
	setQuery: (query: string) => void;
	/** The rest of the search session, owned by the route for the same reason. */
	session: MobileSearchSession;
}
const { initialQuery = "", query, setQuery, session }: Props = $props();

/**
 * True only the first time this search is drawn.
 *
 * Read here, before `onMount` flips it, because both things it gates are
 * arrival behaviour: auto-running a `?q=` search, and scrolling to the top.
 * This component is remounted by every rotation now that the branch is
 * reactive, and repeating either would cost a request nobody asked for and the
 * reader's place in the results.
 */
// svelte-ignore state_referenced_locally
const firstVisit = !session.visited;

const searchQuery = $derived<SearchQuery>({ query });

onMount(() => {
	session.visited = true;
	if (firstVisit && browser && window.scrollTo) {
		window.scrollTo(0, 0);
	}
});
</script>

<div class="archive-mobile-page" data-role="archive-desk-mobile">
	<div class="flex flex-col gap-6 max-w-[600px] mx-auto p-4">
		<div class="archive-header-mobile">
			<h1 class="archive-title-mobile">Search Feeds</h1>
			<p class="archive-subtitle-mobile">
				Search across your RSS feeds
			</p>
		</div>

		<div class="archive-search-container">
			<SearchWindow
				{searchQuery}
				autoSearch={firstVisit && !!initialQuery.trim()}
				setSearchQuery={(next) => {
					setQuery(next.query ?? "");
				}}
				setFeedResults={(newResults) => {
					session.results = newResults;
				}}
				setCursor={(newCursor) => {
					session.cursor = newCursor;
				}}
				setHasMore={(newHasMore) => {
					session.hasMore = newHasMore;
				}}
				isLoading={session.isLoading}
				setIsLoading={(loading) => {
					session.isLoading = loading;
				}}
				setSearchTime={(time) => {
					session.searchTime = time;
				}}
			/>
		</div>

		<SearchResults
			results={session.results}
			isLoading={session.isLoading}
			searchQuery={query}
			searchTime={session.searchTime}
			cursor={session.cursor}
			hasMore={session.hasMore}
			setResults={(newResults) => {
				session.results = newResults;
			}}
			setCursor={(newCursor) => {
				session.cursor = newCursor;
			}}
			setHasMore={(newHasMore) => {
				session.hasMore = newHasMore;
			}}
			setIsLoading={(loading) => {
				session.isLoading = loading;
			}}
		/>

		{#if !query && !session.isLoading && session.results.length === 0}
			<div class="archive-tip">
				<p class="archive-tip-text">
					Try searching for topics like "AI", "technology", or "news"
				</p>
			</div>
		{/if}
	</div>
</div>

<style>
	.archive-mobile-page {
		min-height: 100dvh;
		background: var(--surface-bg);
		color: var(--alt-charcoal);
	}

	.archive-header-mobile {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		margin-top: 0.5rem;
		text-align: center;
	}

	.archive-title-mobile {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.archive-subtitle-mobile {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		margin: 0;
	}

	.archive-search-container {
		border: 1px solid var(--surface-border);
		padding: 0.75rem;
		background: var(--surface-bg);
	}

	.archive-tip {
		border: 1px solid var(--surface-border);
		padding: 0.75rem;
		background: var(--surface-bg);
	}

	.archive-tip-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		font-style: italic;
		text-align: center;
		margin: 0;
	}
</style>
