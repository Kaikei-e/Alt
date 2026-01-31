<script lang="ts">
import { onMount } from "svelte";
import { goto } from "$app/navigation";
import { page } from "$app/state";
import { ConnectError, Code } from "@connectrpc/connect";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import RecapGenreList from "$lib/components/desktop/recap/RecapGenreList.svelte";
import RecapDetail from "$lib/components/desktop/recap/RecapDetail.svelte";
import type { RecapGenre, RecapSummary } from "$lib/schema/recap";
import { createClientTransport, getSevenDayRecap } from "$lib/connect";
import { loadingStore } from "$lib/stores/loading.svelte";

let selectedGenre = $state<RecapGenre | null>(null);

// Simple state for recap
let recapData = $state<RecapSummary | null>(null);
let isLoading = $state(true);
let error = $state<Error | null>(null);

// Derived genres from recapData
let genres = $derived(recapData?.genres ?? []);

// Fetch 7-day recap on mount
onMount(async () => {
	try {
		isLoading = true;
		loadingStore.startLoading();
		const transport = createClientTransport();
		recapData = await getSevenDayRecap(transport);
		// Auto-select genre from URL param or first genre
		if (recapData?.genres && recapData.genres.length > 0) {
			const genreParam = page.url.searchParams.get('genre');
			if (genreParam) {
				const matchingGenre = recapData.genres.find(g => g.genre === genreParam);
				selectedGenre = matchingGenre ?? recapData.genres[0];
			} else {
				selectedGenre = recapData.genres[0];
			}
		}
	} catch (err) {
		// Handle authentication error
		if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
			goto("/login");
			return;
		}
		error = err as Error;
	} finally {
		isLoading = false;
		loadingStore.stopLoading();
	}
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
	<title>7-Day Recap - Alt</title>
</svelte:head>

<PageHeader title="7-Day Recap" description="Weekly news summary by genre" />

{#if recapData}
	<div class="flex items-center gap-2 text-sm text-[var(--text-secondary)] mb-4 -mt-2">
		<span>Generated: {formatExecutedAt(recapData.executedAt)}</span>
		<span class="text-[var(--text-muted)]">·</span>
		<span>{formatArticleCount(recapData.totalArticles)} articles</span>
	</div>
{/if}

{#if isLoading}
	<!-- Loading state handled by SystemLoader via loadingStore -->
{:else if error}
	<div class="text-center py-12">
		<p class="text-[var(--alt-error)] text-sm">
			Error loading recap: {error.message}
		</p>
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
