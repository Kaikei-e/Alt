<script lang="ts">
	import { onMount } from "svelte";
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import RecapGenreList from "$lib/components/desktop/recap/RecapGenreList.svelte";
	import RecapDetail from "$lib/components/desktop/recap/RecapDetail.svelte";
	import type { RecapGenre } from "$lib/schema/recap";
	import { get7DaysRecapClient } from "$lib/api/client/recap";
	import { loadingStore } from "$lib/stores/loading.svelte";

	let selectedGenre = $state<RecapGenre | null>(null);

	// Simple state for recap
	let genres = $state<RecapGenre[]>([]);
	let isLoading = $state(true);
	let error = $state<Error | null>(null);

	// Fetch 7-day recap on mount
	onMount(async () => {
		try {
			isLoading = true;
			loadingStore.startLoading();
			const result = await get7DaysRecapClient();
			genres = result.genres ?? [];
			// Auto-select first genre
			if (genres.length > 0) {
				selectedGenre = genres[0];
			}
		} catch (err) {
			error = err as Error;
		} finally {
			isLoading = false;
			loadingStore.stopLoading();
		}
	});

	function handleSelectGenre(genre: RecapGenre) {
		selectedGenre = genre;
	}
</script>

<PageHeader title="7-Day Recap" description="Weekly news summary by genre" />

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
