<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { page } from "$app/state";
import { ConnectError, Code } from "@connectrpc/connect";
import { useViewport } from "$lib/stores/viewport.svelte";
import {
	createClientTransport,
	getSevenDayRecap,
	getThreeDayRecap,
	searchRecaps,
	type RecapSearchResultItem,
} from "$lib/connect";
import { getLoadingStore } from "$lib/stores/loading.svelte";
import type { RecapGenre, RecapSummary } from "$lib/schema/recap";

// Desktop components
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import RecapGenreList from "$lib/components/desktop/recap/RecapGenreList.svelte";
import RecapDetail from "$lib/components/desktop/recap/RecapDetail.svelte";

// Mobile components
import RecapEmptyState from "$lib/components/mobile/recap/RecapEmptyState.svelte";
import SwipeRecapScreen from "$lib/components/mobile/recap/SwipeRecapScreen.svelte";
import { Button } from "$lib/components/ui/button";
import { Search, ArrowLeft, BookOpen } from "@lucide/svelte";

const { isDesktop } = useViewport();
const loadingStore = getLoadingStore();

// Window selection: 3 or 7 days, driven by URL query param
type RecapWindow = 3 | 7;

function getWindowFromUrl(): RecapWindow {
	const w = page.url.searchParams.get("window");
	return w === "7" ? 7 : 3;
}

let selectedWindow = $state<RecapWindow>(3);
let selectedGenre = $state<RecapGenre | null>(null);
let recapData = $state<RecapSummary | null>(null);
let isLoading = $state(true);
let error = $state<Error | null>(null);
let isRetrying = $state(false);

let genres = $derived(recapData?.genres ?? []);

// Search mode state
let searchQuery = $state<string>("");
let searchResults = $state<RecapSearchResultItem[]>([]);
let isSearching = $state(false);
let searchError = $state<Error | null>(null);
let isSearchMode = $derived(searchQuery.length > 0);

function getQueryFromUrl(): string {
	return page.url.searchParams.get("q") ?? "";
}

async function executeSearch(query: string) {
	if (!query.trim()) return;
	try {
		isSearching = true;
		searchError = null;
		searchResults = [];

		const transport = createClientTransport();
		searchResults = await searchRecaps(transport, query, 50);
	} catch (err) {
		if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
			goto("/login");
			return;
		}
		searchError = err instanceof Error ? err : new Error("Unknown error");
	} finally {
		isSearching = false;
	}
}

function handleSearchSubmit(e: Event) {
	e.preventDefault();
	const trimmed = searchQuery.trim();
	if (!trimmed) return;
	const url = new URL(page.url);
	url.searchParams.set("q", trimmed);
	goto(url.toString(), { replaceState: true });
	void executeSearch(trimmed);
}

function navigateToSearchResult(item: RecapSearchResultItem) {
	searchQuery = "";
	const url = new URL(page.url);
	url.searchParams.delete("q");
	url.searchParams.set("window", String(item.windowDays));
	url.searchParams.set("genre", item.genre);
	goto(url.toString());
	selectedWindow = item.windowDays === 7 ? 7 : 3;
	void fetchRecap(selectedWindow);
}

function clearSearch() {
	searchQuery = "";
	searchResults = [];
	searchError = null;
	const url = new URL(page.url);
	url.searchParams.delete("q");
	goto(url.toString(), { replaceState: true });
	selectedWindow = getWindowFromUrl();
	void fetchRecap(selectedWindow);
}

function formatSearchDate(dateStr: string): string {
	return new Date(dateStr).toLocaleDateString("en-US", {
		month: "short",
		day: "numeric",
		year: "numeric",
	});
}

async function fetchRecap(window: RecapWindow) {
	try {
		isLoading = true;
		error = null;
		recapData = null;
		selectedGenre = null;

		if (isDesktop) {
			loadingStore.startLoading();
		}

		const transport = createClientTransport();
		recapData =
			window === 3
				? await getThreeDayRecap(transport)
				: await getSevenDayRecap(transport);

		// Desktop: auto-select genre from URL param or first genre
		if (isDesktop && recapData?.genres && recapData.genres.length > 0) {
			const genreParam = page.url.searchParams.get("genre");
			if (genreParam) {
				const matchingGenre = recapData.genres.find(
					(g) => g.genre === genreParam,
				);
				selectedGenre = matchingGenre ?? recapData.genres[0];
			} else {
				selectedGenre = recapData.genres[0];
			}
		}
	} catch (err) {
		if (err instanceof ConnectError) {
			if (err.code === Code.Unauthenticated) {
				goto("/login");
				return;
			}
			// NOT_FOUND means no recap job completed yet
			if (err.code === Code.NotFound) {
				recapData = null;
				error = null;
				return;
			}
		}
		error = err instanceof Error ? err : new Error("Unknown error");
		recapData = null;
	} finally {
		isLoading = false;
		if (isDesktop) {
			loadingStore.stopLoading();
		}
	}
}

function switchWindow(window: RecapWindow) {
	if (window !== selectedWindow) {
		selectedWindow = window;
		fetchRecap(window);
	}
}

const retry = async () => {
	isRetrying = true;
	try {
		await fetchRecap(selectedWindow);
	} catch (err) {
		console.error("Retry failed:", err);
	} finally {
		isRetrying = false;
	}
};

function handleSelectGenre(genre: RecapGenre) {
	selectedGenre = genre;
}

function formatExecutedAt(dateStr: string): string {
	return new Date(dateStr).toLocaleString("ja-JP", {
		month: "numeric",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}

function formatArticleCount(count: number): string {
	return count.toLocaleString("ja-JP");
}

onMount(() => {
	if (browser) {
		const q = getQueryFromUrl();
		if (q) {
			searchQuery = q;
			void executeSearch(q);
		} else {
			selectedWindow = getWindowFromUrl();
			void fetchRecap(selectedWindow);
		}
	}
});
</script>

<svelte:head>
	<title>{isSearchMode ? `Search: ${searchQuery}` : `${selectedWindow}-Day Recap`} - Alt</title>
</svelte:head>

{#if isDesktop}
	{#if isSearchMode}
		<!-- Search mode -->
		<PageHeader title="Recap Search" description="Search across all recap genres">
			{#snippet actions()}
				<button
					type="button"
					onclick={clearSearch}
					class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm text-[var(--interactive-text)] hover:text-[var(--interactive-text-hover)] transition-colors"
				>
					<ArrowLeft class="h-4 w-4" />
					Back to latest
				</button>
			{/snippet}
		</PageHeader>

		<form onsubmit={handleSearchSubmit} class="mb-6">
			<div class="relative">
				<Search class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--text-secondary)]" />
				<input
					type="text"
					bind:value={searchQuery}
					placeholder="Search recaps..."
					class="w-full rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] py-2.5 pl-10 pr-4 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-secondary)] focus:border-[var(--interactive-text)] focus:outline-none transition-colors"
				/>
			</div>
		</form>

		{#if isSearching}
			<div class="space-y-3">
				{#each Array(4) as _}
					<div class="rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] p-4 animate-pulse">
						<div class="h-4 bg-[var(--surface-hover)] rounded w-1/3 mb-3"></div>
						<div class="h-3 bg-[var(--surface-hover)] rounded w-full mb-2"></div>
						<div class="h-3 bg-[var(--surface-hover)] rounded w-4/5"></div>
					</div>
				{/each}
			</div>
		{:else if searchError}
			<div class="text-center py-12">
				<p class="text-[var(--text-secondary)] text-sm">Failed to search recaps.</p>
				<p class="text-[var(--text-muted)] text-xs mt-1">{searchError.message}</p>
			</div>
		{:else if searchResults.length === 0}
			<div class="text-center py-12">
				<p class="text-[var(--text-secondary)] text-sm">No matching recaps found.</p>
			</div>
		{:else}
			<p class="text-xs text-[var(--text-secondary)] mb-4">
				{searchResults.length} result{searchResults.length !== 1 ? 's' : ''}
			</p>
			<div class="space-y-3">
				{#each searchResults as item (item.jobId + item.genre)}
					<button
						type="button"
						onclick={() => navigateToSearchResult(item)}
						class="w-full text-left rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] p-4 space-y-2 cursor-pointer hover:border-[var(--interactive-text)] transition-colors"
					>
						<div class="flex items-start gap-3">
							<BookOpen class="mt-0.5 h-4 w-4 flex-shrink-0 text-[var(--text-secondary)]" />
							<div class="min-w-0 flex-1 space-y-1.5">
								<div class="flex items-center gap-2">
									<h3 class="text-sm font-medium text-[var(--text-primary)]">
										{item.genre}
									</h3>
									<span class="inline-block rounded border border-[var(--surface-border)] px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-[var(--text-secondary)]">
										{item.windowDays}-day
									</span>
									<span class="text-[10px] text-[var(--text-secondary)]">
										{formatSearchDate(item.executedAt)}
									</span>
								</div>
								{#if item.summary}
									<p class="text-xs text-[var(--text-secondary)] leading-relaxed line-clamp-3">
										{item.summary}
									</p>
								{/if}
								{#if item.topTerms.length > 0}
									<div class="flex flex-wrap gap-1.5">
										{#each item.topTerms.slice(0, 5) as term}
											<span class="inline-block rounded-full bg-[var(--surface-hover)] px-2 py-0.5 text-[10px] font-medium text-[var(--text-secondary)]">
												{term}
											</span>
										{/each}
									</div>
								{/if}
							</div>
						</div>
					</button>
				{/each}
			</div>
		{/if}
	{:else}
		<!-- Normal recap view -->
		<PageHeader title="Recap" description="News summary by genre">
			{#snippet actions()}
				<div class="flex items-center gap-1 bg-[var(--surface-bg)] rounded-lg p-1 border border-[var(--border-color)]">
					<button
						class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {selectedWindow === 3
							? 'bg-gray-800 text-white shadow-sm'
							: 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)]'}"
						onclick={() => switchWindow(3)}
						disabled={isLoading}
					>
						3 Days
					</button>
					<button
						class="px-3 py-1.5 text-sm font-medium rounded-md transition-colors {selectedWindow === 7
							? 'bg-gray-800 text-white shadow-sm'
							: 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)]'}"
						onclick={() => switchWindow(7)}
						disabled={isLoading}
					>
						7 Days
					</button>
				</div>
			{/snippet}
		</PageHeader>

		{#if recapData}
			<div class="flex items-center gap-2 text-sm text-[var(--text-secondary)] mb-4 -mt-2">
				<span class="font-medium">{selectedWindow}-day window</span>
				<span class="text-[var(--text-muted)]">&middot;</span>
				<span>Generated: {formatExecutedAt(recapData.executedAt)}</span>
				<span class="text-[var(--text-muted)]">&middot;</span>
				<span>{formatArticleCount(recapData.totalArticles)} articles</span>
			</div>
		{/if}

		{#if isLoading}
			<!-- Loading state handled by SystemLoader via loadingStore -->
		{:else if error}
			<div class="text-center py-12">
				<div class="inline-flex flex-col items-center gap-3">
					<p class="text-[var(--text-secondary)] text-sm">
						No recap data available yet.
					</p>
					<p class="text-[var(--text-muted)] text-xs">
						Run a recap job first to see the summary here.
					</p>
				</div>
			</div>
		{:else if genres.length === 0}
			<div class="text-center py-12">
				<p class="text-[var(--text-secondary)] text-sm">No recap data available</p>
			</div>
		{:else}
			<div class="grid grid-cols-3 gap-6 h-[calc(100vh-12rem)]">
				<div class="col-span-1 h-full overflow-y-auto">
					<RecapGenreList {genres} {selectedGenre} onSelectGenre={handleSelectGenre} />
				</div>
				<div class="col-span-2 h-full overflow-y-auto">
					<RecapDetail genre={selectedGenre} />
				</div>
			</div>
		{/if}
	{/if}
{:else}
	<!-- Mobile -->
	<div class="min-h-[100dvh] relative" style="background: var(--app-bg);">
		{#if isLoading}
			<div
				class="p-5 max-w-2xl mx-auto h-[100dvh]"
				data-testid="recap-skeleton-container"
			>
				<div class="flex flex-col gap-4">
					{#each Array(5) as _}
						<div
							class="p-4 rounded-2xl border-2 border-border animate-pulse"
							style="background: var(--surface-bg);"
						>
							<div class="h-4 bg-muted rounded w-3/4 mb-2"></div>
							<div class="h-3 bg-muted rounded w-full mb-1"></div>
							<div class="h-3 bg-muted rounded w-5/6"></div>
						</div>
					{/each}
				</div>
			</div>
		{:else if error}
			<div class="flex flex-col items-center justify-center min-h-[50vh] p-6">
				<div
					class="p-6 rounded-lg border text-center"
					style="background: var(--surface-bg); border-color: hsl(var(--destructive));"
				>
					<p
						class="font-semibold mb-2"
						style="color: hsl(var(--destructive));"
					>
						Error loading recap
					</p>
					<p
						class="text-sm mb-4"
						style="color: var(--text-secondary);"
					>
						{error.message}
					</p>
					<Button
						onclick={() => void retry()}
						disabled={isRetrying}
						class="px-4 py-2 rounded disabled:opacity-50"
						style="background: var(--alt-primary); color: var(--text-primary);"
					>
						{isRetrying ? "Retrying..." : "Retry"}
					</Button>
				</div>
			</div>
		{:else if !recapData || recapData.genres.length === 0}
			<RecapEmptyState />
		{:else}
			<SwipeRecapScreen genres={recapData.genres} summaryData={recapData} />
		{/if}
	</div>
{/if}
