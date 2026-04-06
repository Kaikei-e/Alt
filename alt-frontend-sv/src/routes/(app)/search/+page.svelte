<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { page } from "$app/state";
import { Search, AlertTriangle } from "@lucide/svelte";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import ArticleSearchSection from "$lib/components/search/ArticleSearchSection.svelte";
import RecapSearchSection from "$lib/components/search/RecapSearchSection.svelte";
import TagSearchSection from "$lib/components/search/TagSearchSection.svelte";
import SearchSectionSkeleton from "$lib/components/search/SearchSectionSkeleton.svelte";
import { useGlobalSearch } from "$lib/hooks/useGlobalSearch.svelte";
import { useViewport } from "$lib/stores/viewport.svelte";

const { isDesktop } = useViewport();
const gs = useGlobalSearch();

let inputQuery = $state("");

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

onMount(() => {
	if (browser) {
		const q = page.url.searchParams.get("q");
		if (q) {
			inputQuery = q;
			gs.search(q);
		}
	}
});
</script>

<svelte:head>
	<title>{gs.query ? `Search: ${gs.query}` : "Search"} - Alt</title>
</svelte:head>

{#if isDesktop}
	<PageHeader
		title="Search"
		description="Find articles, recaps, and tags across your knowledge base"
	/>
{/if}

<!-- Search Input -->
<div class="bg-[var(--surface-bg)] px-4 py-3" class:mb-6={isDesktop}>
	<div class="relative flex items-center gap-2">
		<div class="relative flex-1">
			<input
				type="text"
				bind:value={inputQuery}
				onkeydown={handleKeydown}
				placeholder="Search everything..."
				class="w-full rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] px-4 py-2.5 pl-9 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-secondary)] shadow-[var(--shadow-sm)] focus:border-[var(--accent-primary)] focus:ring-2 focus:ring-[var(--accent-primary,var(--interactive-text))]/20 focus:outline-none transition-colors"
			/>
			<Search
				class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--text-secondary)]"
			/>
		</div>
		{#if inputQuery.trim()}
			<button
				type="button"
				onclick={clearSearch}
				class="rounded-lg px-3 py-2 text-sm text-[var(--text-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--text-primary)] transition-colors"
			>
				Clear
			</button>
		{/if}
		<button
			type="button"
			onclick={handleSearch}
			class="rounded-lg border border-[var(--surface-border)] px-4 py-2 text-sm font-medium text-[var(--interactive-text)] hover:bg-[var(--surface-hover)] hover:text-[var(--interactive-text-hover)] transition-colors"
		>
			Search
		</button>
	</div>
</div>

<!-- Degraded Sections Banner -->
{#if gs.degradedSections.length > 0}
	<div
		class="mx-4 mb-4 flex items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-800"
	>
		<AlertTriangle class="h-4 w-4 flex-shrink-0" />
		<span>
			Some sections are unavailable: {gs.degradedSections.join(", ")}.
			Results may be incomplete.
		</span>
	</div>
{/if}

<!-- Error State -->
{#if gs.error}
	<div
		class="mx-4 mb-4 rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-800"
	>
		<p class="font-medium">Search failed</p>
		<p class="mt-1">{gs.error.message}</p>
	</div>
{/if}

<!-- Content -->
<div class="space-y-8" class:px-4={!isDesktop} class:pb-8={true}>
	{#if gs.loading}
		<!-- Loading Skeletons -->
		<SearchSectionSkeleton label="Loading articles" rows={3} />
		<SearchSectionSkeleton label="Loading recaps" rows={2} />
		<SearchSectionSkeleton label="Loading tags" rows={1} />
	{:else if gs.result}
		<!-- Article Section -->
		{#if gs.result.articleSection && !gs.degradedSections.includes("articles")}
			<ArticleSearchSection
				section={gs.result.articleSection}
				query={gs.query}
			/>
		{/if}

		<!-- Recap Section -->
		{#if gs.result.recapSection && !gs.degradedSections.includes("recaps")}
			<RecapSearchSection
				section={gs.result.recapSection}
				query={gs.query}
			/>
		{/if}

		<!-- Tag Section -->
		{#if gs.result.tagSection && !gs.degradedSections.includes("tags")}
			<TagSearchSection
				section={gs.result.tagSection}
				query={gs.query}
			/>
		{/if}

		<!-- Global Empty State -->
		{#if !gs.hasResults && gs.degradedSections.length === 0}
			<div class="flex flex-col items-center justify-center py-16 text-center">
				<Search class="h-12 w-12 text-[var(--text-secondary)] mb-4 opacity-40" />
				<p class="text-lg font-medium text-[var(--text-primary)]">
					No results found
				</p>
				<p class="mt-1 text-sm text-[var(--text-secondary)]">
					Try different keywords or broaden your search.
				</p>
			</div>
		{/if}
	{:else if !gs.query}
		<!-- Initial Empty State -->
		<div class="flex flex-col items-center justify-center py-16 text-center">
			<Search class="h-12 w-12 text-[var(--text-secondary)] mb-4 opacity-40" />
			<p class="text-lg font-medium text-[var(--text-primary)]">
				Search your knowledge base
			</p>
			<p class="mt-1 text-sm text-[var(--text-secondary)]">
				Find articles, recaps, and tags across all your content.
			</p>
		</div>
	{/if}
</div>
