<script lang="ts">
import { onMount } from "svelte";
import { goto } from "$app/navigation";
import { page } from "$app/state";
import { ConnectError, Code } from "@connectrpc/connect";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import RecapGenreList from "$lib/components/desktop/recap/RecapGenreList.svelte";
import RecapDetail from "$lib/components/desktop/recap/RecapDetail.svelte";
import type { RecapGenre, RecapSummary } from "$lib/schema/recap";
import {
	createClientTransport,
	getSevenDayRecap,
	getThreeDayRecap,
} from "$lib/connect";
import { getLoadingStore } from "$lib/stores/loading.svelte";

const loadingStore = getLoadingStore();

let selectedGenre = $state<RecapGenre | null>(null);

// Window selection: 3 or 7 days
type RecapWindow = 3 | 7;
let selectedWindow = $state<RecapWindow>(3);

// Simple state for recap
let recapData = $state<RecapSummary | null>(null);
let isLoading = $state(true);
let error = $state<Error | null>(null);

// Derived genres from recapData
let genres = $derived(recapData?.genres ?? []);

// Fetch recap data based on selected window
async function fetchRecap(window: RecapWindow) {
	try {
		isLoading = true;
		error = null;
		recapData = null; // Clear previous data before fetching
		selectedGenre = null;
		loadingStore.startLoading();
		const transport = createClientTransport();
		recapData =
			window === 3
				? await getThreeDayRecap(transport)
				: await getSevenDayRecap(transport);
		// Auto-select genre from URL param or first genre
		if (recapData?.genres && recapData.genres.length > 0) {
			const genreParam = page.url.searchParams.get("genre");
			if (genreParam) {
				const matchingGenre = recapData.genres.find(
					(g) => g.genre === genreParam,
				);
				selectedGenre = matchingGenre ?? recapData.genres[0];
			} else {
				selectedGenre = recapData.genres[0];
			}
		} else {
			selectedGenre = null;
		}
	} catch (err) {
		// Handle authentication error
		if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
			goto("/login");
			return;
		}
		error = err as Error;
		recapData = null; // Clear data on error
	} finally {
		isLoading = false;
		loadingStore.stopLoading();
	}
}

// Switch window and refetch
function switchWindow(window: RecapWindow) {
	if (window !== selectedWindow) {
		selectedWindow = window;
		fetchRecap(window);
	}
}

// Fetch 3-day recap on mount (default)
onMount(() => {
	fetchRecap(selectedWindow);
});

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
</script>

<svelte:head>
	<title>{selectedWindow}-Day Recap - Alt</title>
</svelte:head>

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
		<span class="text-[var(--text-muted)]">·</span>
		<span>Generated: {formatExecutedAt(recapData.executedAt)}</span>
		<span class="text-[var(--text-muted)]">·</span>
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
		<!-- Genre list (left column, 1/3 width) -->
		<div class="col-span-1 h-full overflow-y-auto">
			<RecapGenreList {genres} {selectedGenre} onSelectGenre={handleSelectGenre} />
		</div>

		<!-- Detail view (right columns, 2/3 width) -->
		<div class="col-span-2">
			<RecapDetail genre={selectedGenre} />
		</div>
	</div>
{/if}
