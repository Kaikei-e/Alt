<script lang="ts">
	import { onMount } from "svelte";
	import { browser } from "$app/environment";
	import { get7DaysRecapClient } from "$lib/api/client";
	import type { RecapSummary } from "$lib/schema/recap";
	import EmptyFeedState from "$lib/components/mobile/EmptyFeedState.svelte";
	import RecapTimeline from "$lib/components/mobile/recap/RecapTimeline.svelte";
	import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
	import { Button } from "$lib/components/ui/button";

	let data = $state<RecapSummary | null>(null);
	let isInitialLoading = $state(true);
	let error = $state<Error | null>(null);
	let isRetrying = $state(false);

	const fetchData = async () => {
		try {
			isInitialLoading = true;
			error = null;
			const recap = await get7DaysRecapClient();
			data = recap;
		} catch (err) {
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
		<!-- Content -->
		<div
			class="p-5 max-w-2xl mx-auto overflow-y-auto overflow-x-hidden h-screen"
			data-testid="recap-scroll-container"
			style="background: var(--app-bg);"
		>
			<!-- ヘッダー -->
			<div class="mb-6">
				<h1
					class="text-2xl font-bold mb-2"
					style="color: var(--accent-primary); background: var(--accent-gradient); -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text;"
				>
					7 Days Recap
				</h1>
				<p
					class="text-xs mb-1"
					style="color: var(--text-secondary);"
				>
					Executed: {new Date(data.executedAt).toLocaleString("en-US")}
				</p>
				<p
					class="text-xs"
					style="color: var(--text-secondary);"
				>
					{data.totalArticles.toLocaleString()} articles analyzed
				</p>
			</div>

			<!-- タイムライン -->
			<RecapTimeline genres={data.genres} />

			<!-- フッター余白 -->
			<div class="h-20"></div>
		</div>
	{/if}

	<FloatingMenu />
</div>

