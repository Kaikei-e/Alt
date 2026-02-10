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
		selectedWindow = getWindowFromUrl();
		void fetchRecap(selectedWindow);
	}
});
</script>

<svelte:head>
	<title>{selectedWindow}-Day Recap - Alt</title>
</svelte:head>

{#if isDesktop}
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
			<div class="col-span-2">
				<RecapDetail genre={selectedGenre} />
			</div>
		</div>
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
