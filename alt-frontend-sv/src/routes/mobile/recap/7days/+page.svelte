<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { ConnectError, Code } from "@connectrpc/connect";
import { createClientTransport, getSevenDayRecap } from "$lib/connect";
import EmptyFeedState from "$lib/components/mobile/EmptyFeedState.svelte";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import SwipeRecapScreen from "$lib/components/mobile/recap/SwipeRecapScreen.svelte";
import { Button } from "$lib/components/ui/button";
import type { RecapSummary } from "$lib/schema/recap";

let data = $state<RecapSummary | null>(null);
let isInitialLoading = $state(true);
let error = $state<Error | null>(null);
let isRetrying = $state(false);

const fetchData = async () => {
	try {
		isInitialLoading = true;
		error = null;
		const transport = createClientTransport();
		const recap = await getSevenDayRecap(transport);
		data = recap;
	} catch (err) {
		// Handle authentication error
		if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
			goto("/login");
			return;
		}
		error = err instanceof Error ? err : new Error("Unknown error");
		data = null;
	} finally {
		isInitialLoading = false;
	}
};

const retry = async () => {
	isRetrying = true;
	try {
		await fetchData();
	} catch (err) {
		console.error("Retry failed:", err);
	} finally {
		isRetrying = false;
	}
};

onMount(() => {
	if (browser) {
		void fetchData();
	}
});
</script>

<svelte:head>
	<title>7-Day Recap - Alt</title>
</svelte:head>

<div class="min-h-[100dvh] relative" style="background: var(--app-bg);">
	{#if isInitialLoading}
		<!-- Initial loading skeleton -->
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
		<!-- Error state -->
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
	{:else if !data || data.genres.length === 0}
		<!-- Empty state -->
		<EmptyFeedState />
	{:else}
		<!-- Content with Swipe UI -->
		<SwipeRecapScreen genres={data.genres} summaryData={data} />
	{/if}

	<FloatingMenu />
</div>

