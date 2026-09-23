<script lang="ts">
import { Code, ConnectError } from "@connectrpc/connect";
import { AlertTriangle, ArrowLeft, Calendar, RefreshCw } from "@lucide/svelte";
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import { RecapTopicCard } from "$lib/components/recap";
import { Button } from "$lib/components/ui/button";
import {
	createClientTransport,
	getThreeDayRecapCards,
	type ThreeDayRecapCardsResponse,
} from "$lib/connect";
import { getLoadingStore } from "$lib/stores/loading.svelte";
import { isDesktop } from "$lib/stores/viewport.svelte";
import { determineCardsPageState, formatJobWindow } from "./cards-state";

const loadingStore = getLoadingStore();

let recapCardsData = $state<ThreeDayRecapCardsResponse | null>(null);
let isLoading = $state(true);
let error = $state<Error | null>(null);
let isRetrying = $state(false);

const pageState = $derived(
	determineCardsPageState({
		isLoading,
		error,
		data: recapCardsData,
	}),
);

const sortedCards = $derived(
	recapCardsData?.cards
		? [...recapCardsData.cards].sort((a, b) => a.rank - b.rank)
		: [],
);

async function fetchCards() {
	try {
		isLoading = true;
		error = null;

		if (isDesktop()) {
			loadingStore.startLoading();
		}

		const transport = createClientTransport();
		recapCardsData = await getThreeDayRecapCards(transport);
	} catch (err) {
		if (err instanceof ConnectError) {
			if (err.code === Code.Unauthenticated) {
				goto("/login");
				return;
			}
			if (err.code === Code.NotFound) {
				recapCardsData = null;
				error = null;
				return;
			}
		}
		error =
			err instanceof Error ? err : new Error("Failed to load topic cards");
		recapCardsData = null;
	} finally {
		isLoading = false;
		if (isDesktop()) {
			loadingStore.stopLoading();
		}
	}
}

async function handleRetry() {
	isRetrying = true;
	try {
		await fetchCards();
	} finally {
		isRetrying = false;
	}
}

onMount(() => {
	if (browser) {
		void fetchCards();
	}
});
</script>

<svelte:head>
	<title>Topic Cards (3-Day) - Alt</title>
</svelte:head>

<div class="cards-page-container min-h-[calc(100dvh-5rem)] pb-12">
	{#if isDesktop()}
		<!-- Desktop Header -->
		<PageHeader
			title="Topic Cards"
			description="Ranked three-day topic summaries and evidence sources"
		>
			{#snippet actions()}
				<div class="flex items-center gap-2">
					<a
						href="/recap"
						class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-lg text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] border border-[var(--surface-border)] transition-colors"
					>
						<ArrowLeft class="h-4 w-4" />
						Genre recap
					</a>
				</div>
			{/snippet}
		</PageHeader>
	{:else}
		<!-- Mobile Header -->
		<header class="mb-6 pb-4 border-b border-[var(--surface-border)]">
			<div class="flex items-center justify-between gap-3">
				<div>
					<h1 class="text-xl font-bold text-[var(--text-primary)]">
						Topic Cards
					</h1>
					<p class="text-xs text-[var(--text-secondary)] mt-0.5">
						3-day window recap
					</p>
				</div>
				<a
					href="/recap"
					class="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] bg-[var(--surface-bg)] border border-[var(--surface-border)]"
				>
					<ArrowLeft class="h-3.5 w-3.5" />
					Genre view
				</a>
			</div>
		</header>
	{/if}

	<!-- Content State Machine -->
	{#if pageState === "loading"}
		<!-- Loading Skeleton -->
		<div
			data-testid="recap-cards-skeleton"
			class="space-y-4 max-w-3xl"
			aria-busy="true"
			aria-label="Loading topic cards"
		>
			<div class="flex items-center gap-2 text-sm text-[var(--text-muted)] animate-pulse">
				<div class="h-4 w-48 bg-[var(--surface-hover)] rounded"></div>
			</div>
			{#each Array(3) as _}
				<div class="rounded-xl border border-[var(--surface-border)] bg-[var(--surface-bg)] p-5 space-y-3 animate-pulse">
					<div class="flex items-center gap-2">
						<div class="h-5 w-8 bg-[var(--surface-hover)] rounded"></div>
						<div class="h-5 w-20 bg-[var(--surface-hover)] rounded-full"></div>
					</div>
					<div class="h-6 w-3/4 bg-[var(--surface-hover)] rounded"></div>
					<div class="h-4 w-full bg-[var(--surface-hover)] rounded"></div>
					<div class="h-4 w-5/6 bg-[var(--surface-hover)] rounded"></div>
				</div>
			{/each}
		</div>
	{:else if pageState === "error"}
		<!-- Error State -->
		<div
			data-testid="recap-cards-error"
			class="flex flex-col items-center justify-center py-16 px-4 text-center max-w-md mx-auto"
		>
			<div class="rounded-full bg-red-500/10 p-3 mb-4 text-red-500">
				<AlertTriangle class="h-8 w-8" />
			</div>
			<h2 class="text-lg font-bold text-[var(--text-primary)] mb-1">
				Failed to load topic cards
			</h2>
			<p class="text-sm text-[var(--text-secondary)] mb-6">
				Topic cards could not be loaded. Try again.
			</p>
			<Button
				onclick={handleRetry}
				disabled={isRetrying}
				class="inline-flex items-center gap-2 px-4 py-2"
			>
				<RefreshCw class="h-4 w-4 {isRetrying ? 'animate-spin' : ''}" />
				{isRetrying ? "Retrying..." : "Retry"}
			</Button>
		</div>
	{:else if pageState === "empty"}
		<!-- Empty State -->
		<div
			data-testid="recap-cards-empty"
			class="flex flex-col items-center justify-center py-20 px-4 text-center max-w-lg mx-auto"
		>
			<div class="rounded-full bg-[var(--surface-hover)] p-4 mb-4 border border-[var(--surface-border)] text-[var(--text-muted)]">
				<Calendar class="h-8 w-8" />
			</div>
			<h2 class="text-xl font-bold text-[var(--text-primary)] mb-2">
				No topic cards yet
			</h2>
			<p class="text-sm text-[var(--text-secondary)] leading-relaxed">
				Three-day topic recap cards will appear here once generated.
			</p>
		</div>
	{:else}
		<!-- Populated / Degraded States -->
		<div class="space-y-6 max-w-3xl">
			<!-- Window metadata header -->
			{#if recapCardsData?.job}
				<div class="flex items-center justify-between gap-3 flex-wrap">
					<div
						data-testid="recap-cards-window"
						class="text-xs sm:text-sm font-medium text-[var(--text-secondary)] flex items-center gap-2"
					>
						<span class="inline-block w-2 h-2 rounded-full bg-[var(--interactive-text)]"></span>
						<span>{formatJobWindow(recapCardsData.job.from, recapCardsData.job.to)}</span>
						<span class="text-[var(--text-muted)]">&middot;</span>
						<span>{sortedCards.length} topic{sortedCards.length !== 1 ? 's' : ''}</span>
					</div>
				</div>

				<!-- Degraded notice -->
				{#if recapCardsData.job.degraded}
					<div
						data-testid="recap-cards-degraded"
						class="flex items-start gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3.5 text-xs sm:text-sm text-amber-700 dark:text-amber-300"
						role="alert"
					>
						<AlertTriangle class="h-4 w-4 shrink-0 mt-0.5 text-amber-600 dark:text-amber-400" />
						<div class="space-y-0.5">
							<p class="font-semibold">Degraded generation</p>
							<p class="text-amber-800/80 dark:text-amber-200/80">
								Fewer than the minimum expected topic cards were produced for this window.
							</p>
						</div>
					</div>
				{/if}
			{/if}

			<!-- Cards list -->
			<div data-testid="recap-cards-list" class="space-y-4">
				{#each sortedCards as card (card.id)}
					<RecapTopicCard {card} />
				{/each}
			</div>
		</div>
	{/if}
</div>
