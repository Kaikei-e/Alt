<script lang="ts">
	import { ArrowRight, Loader2, Tag } from "@lucide/svelte";
	import { get7DaysRecapClient } from "$lib/api/client/recap";
	import type { RecapGenre, RecapSummary } from "$lib/schema/recap";
	import { onMount } from "svelte";

	const svBasePath = "/sv";

	// Simple state without TanStack Query
	let recapData = $state<RecapSummary | null>(null);
	let isLoading = $state(true);
	let error = $state<Error | null>(null);

	// Computed top genres
	let topGenres = $derived(recapData?.genres?.slice(0, 3) ?? []);

	// Fetch 7-day recap on mount
	onMount(async () => {
		try {
			isLoading = true;
			recapData = await get7DaysRecapClient();
		} catch (err) {
			error = err as Error;
		} finally {
			isLoading = false;
		}
	});
</script>

<div class="border border-[var(--surface-border)] bg-white p-6 flex flex-col h-full">
	<!-- Header -->
	<div class="flex items-center justify-between mb-4">
		<h3 class="text-lg font-semibold text-[var(--text-primary)]">7-Day Recap</h3>
		<a
			href="{svBasePath}/desktop/recap"
			class="flex items-center gap-1 text-sm text-[var(--accent-primary)] hover:underline"
		>
			View Details
			<ArrowRight class="h-3.5 w-3.5" />
		</a>
	</div>

	<!-- Content -->
	<div class="flex-1 overflow-y-auto">
		{#if isLoading}
			<div class="flex items-center justify-center py-12">
				<Loader2 class="h-6 w-6 animate-spin text-[var(--accent-primary)]" />
			</div>
		{:else if error}
			<div class="text-sm text-[var(--alt-error)] text-center py-8">
				Error: {error.message}
			</div>
		{:else if topGenres.length === 0}
			<div class="text-sm text-[var(--text-secondary)] text-center py-8">
				No recap data available
			</div>
		{:else}
			<div class="space-y-4">
				{#each topGenres as genre}
					<div
						class="border border-[var(--surface-border)] p-4 hover:bg-[var(--surface-hover)] transition-colors duration-200"
					>
						<div class="flex items-start justify-between mb-2">
							<h4 class="text-sm font-semibold text-[var(--text-primary)]">
								{genre.genre}
							</h4>
							<span class="text-xs text-[var(--text-secondary)]">
								{genre.articleCount} articles
							</span>
						</div>
						<p class="text-xs text-[var(--text-secondary)] line-clamp-2 mb-2">
							{genre.summary}
						</p>
						{#if genre.topTerms && genre.topTerms.length > 0}
							<div class="flex flex-wrap gap-1">
								{#each genre.topTerms.slice(0, 3) as term}
									<span
										class="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-[var(--surface-bg)] border border-[var(--surface-border)] text-[var(--text-secondary)]"
									>
										<Tag class="h-2.5 w-2.5" />
										{term}
									</span>
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>

			{#if recapData?.executedAt}
				<p class="text-xs text-[var(--text-muted)] text-center mt-4">
					Updated: {new Date(recapData.executedAt).toLocaleDateString('en-US')}
				</p>
			{/if}
		{/if}
	</div>
</div>
