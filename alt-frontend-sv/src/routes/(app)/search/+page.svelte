<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { page } from "$app/state";
import ArticleSearchSection from "$lib/components/search/ArticleSearchSection.svelte";
import RecapSearchSection from "$lib/components/search/RecapSearchSection.svelte";
import TagSearchSection from "$lib/components/search/TagSearchSection.svelte";
import SearchSectionSkeleton from "$lib/components/search/SearchSectionSkeleton.svelte";
import { useGlobalSearch } from "$lib/hooks/useGlobalSearch.svelte";
import { useViewport } from "$lib/stores/viewport.svelte";

const { isDesktop } = useViewport();
const gs = useGlobalSearch();

let inputQuery = $state("");
let revealed = $state(false);

const dateStr = $derived(
	new Date().toLocaleDateString("en-US", {
		weekday: "long",
		year: "numeric",
		month: "long",
		day: "numeric",
	}),
);

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
	if (browser) {
		const q = page.url.searchParams.get("q");
		if (q) {
			inputQuery = q;
			gs.search(q);
		}
	}
});

function handleKeydown(e: KeyboardEvent) {
	if (e.key === "Enter") {
		handleSearch();
	}
}

function handleSearch() {
	const trimmed = inputQuery.trim();
	if (!trimmed) return;
	goto(`/search?q=${encodeURIComponent(trimmed)}`, {
		replaceState: true,
		noScroll: true,
		keepFocus: true,
	});
	gs.search(trimmed);
}

function clearSearch() {
	inputQuery = "";
	gs.clear();
	goto("/search", { replaceState: true, noScroll: true, keepFocus: true });
}
</script>

<svelte:head>
	<title>{gs.query ? `Search: ${gs.query}` : "Search"} - Alt</title>
</svelte:head>

<div class="ref-page" class:revealed data-role="reference-desk-page">
	<header class="ref-header">
		<span class="ref-date">{dateStr}</span>
		{#if isDesktop}
			<h1 class="ref-title">Reference Desk</h1>
		{:else}
			<h1 class="ref-title-mobile">Reference Desk</h1>
		{/if}
		<div class="ref-rule" aria-hidden="true"></div>
	</header>

	<div class="ref-search-bar">
		<input
			type="text"
			bind:value={inputQuery}
			onkeydown={handleKeydown}
			placeholder="Search everything..."
			class="ref-input"
			data-role="ref-search-input"
		/>
		{#if inputQuery.trim()}
			<button
				type="button"
				onclick={clearSearch}
				class="ref-clear-btn"
			>
				CLEAR
			</button>
		{/if}
		<button
			type="button"
			onclick={handleSearch}
			class="ref-search-btn"
		>
			SEARCH
		</button>
	</div>

	{#if gs.degradedSections.length > 0}
		<div class="ref-degraded-banner" role="status">
			Some sections are unavailable: {gs.degradedSections.join(", ")}.
			Results may be incomplete.
		</div>
	{/if}

	{#if gs.error}
		<div class="error-stripe" role="alert">
			<p class="error-stripe-title">Search failed</p>
			<p>{gs.error.message}</p>
		</div>
	{/if}

	<div class="ref-content" class:px-4={!isDesktop}>
		{#if gs.loading}
			<SearchSectionSkeleton label="Loading articles" rows={3} />
			<SearchSectionSkeleton label="Loading recaps" rows={2} />
			<SearchSectionSkeleton label="Loading tags" rows={1} />
		{:else if gs.result}
			{#if gs.result.articleSection && !gs.degradedSections.includes("articles")}
				<ArticleSearchSection
					section={gs.result.articleSection}
					query={gs.query}
				/>
			{/if}

			{#if gs.result.recapSection && !gs.degradedSections.includes("recaps")}
				<RecapSearchSection
					section={gs.result.recapSection}
					query={gs.query}
				/>
			{/if}

			{#if gs.result.tagSection && !gs.degradedSections.includes("tags")}
				<TagSearchSection
					section={gs.result.tagSection}
					query={gs.query}
				/>
			{/if}

			{#if !gs.hasResults && gs.degradedSections.length === 0}
				<div class="ref-empty">
					<p class="ref-empty-title">No results found</p>
					<p class="ref-empty-subtitle">
						Try different keywords or broaden your search.
					</p>
				</div>
			{/if}
		{:else if !gs.query}
			<div class="ref-empty">
				<p class="ref-empty-title">Search your knowledge base</p>
				<p class="ref-empty-subtitle">
					Find articles, recaps, and tags across all your content.
				</p>
			</div>
		{/if}
	</div>
</div>

<style>
	.ref-page {
		opacity: 0;
		transform: translateY(6px);
		transition: opacity 0.4s ease, transform 0.4s ease;
	}

	.ref-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	.ref-header {
		padding: 1.5rem 0 0;
	}

	.ref-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.ref-title {
		font-family: var(--font-display);
		font-size: 1.6rem;
		font-weight: 800;
		color: var(--alt-charcoal);
		letter-spacing: -0.01em;
		margin: 0.15rem 0 0;
		line-height: 1.2;
	}

	.ref-title-mobile {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0.1rem 0 0;
		line-height: 1.2;
	}

	.ref-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	.ref-search-bar {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		margin-top: 1rem;
		margin-bottom: 1.25rem;
	}

	.ref-input {
		flex: 1;
		padding: 0.625rem 0.75rem;
		font-family: var(--font-body);
		font-size: 1rem;
		color: var(--alt-charcoal);
		background: var(--surface-bg);
		border: 1px solid var(--surface-border);
		border-radius: 0;
		outline: none;
		transition: border-color 0.15s;
	}

	.ref-input:focus {
		border-color: var(--alt-charcoal);
	}

	.ref-input::placeholder {
		color: var(--alt-ash);
	}

	.ref-search-btn {
		min-height: 44px;
		padding: 0 1.25rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.ref-search-btn:hover {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.ref-clear-btn {
		padding: 0 0.75rem;
		min-height: 44px;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-ash);
		background: transparent;
		border: 1px solid var(--surface-border);
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}

	.ref-clear-btn:hover {
		border-color: var(--alt-charcoal);
		color: var(--alt-charcoal);
	}

	.ref-degraded-banner {
		padding: 0.5rem 0.75rem;
		border-left: 3px solid var(--alt-terracotta);
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-terracotta);
		margin-bottom: 1rem;
	}

	.ref-content {
		display: flex;
		flex-direction: column;
		gap: 2rem;
		padding-bottom: 2rem;
	}

	.ref-empty {
		padding: 3rem 0;
		text-align: center;
	}

	.ref-empty-title {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.ref-empty-subtitle {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		font-style: italic;
		margin: 0.3rem 0 0;
	}

	.error-stripe {
		padding: 0.75rem 1rem;
		border-left: 3px solid var(--alt-terracotta);
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-terracotta);
		margin-bottom: 1rem;
	}

	.error-stripe-title {
		font-weight: 600;
		margin: 0 0 0.25rem;
	}

	.error-stripe p {
		margin: 0;
	}

	@media (prefers-reduced-motion: reduce) {
		.ref-page {
			opacity: 1;
			transform: none;
			transition: none;
		}
	}
</style>
