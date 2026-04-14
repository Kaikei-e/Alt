<script lang="ts">
import { goto } from "$app/navigation";
import { onMount } from "svelte";
import { useViewport } from "$lib/stores/viewport.svelte";
import { useFeedStats } from "$lib/hooks/useFeedStats.svelte";
import { getFeedsWithCursorClient } from "$lib/api/client/feeds";
import { ConnectError, Code } from "@connectrpc/connect";
import { createClientTransport, getThreeDayRecap } from "$lib/connect";
import type { RenderFeed } from "$lib/schema/feed";
import type { RecapSummary } from "$lib/schema/recap";

import StatsBarWidget from "$lib/components/desktop/dashboard/StatsBarWidget.svelte";
import UnreadFeedsWidget from "$lib/components/desktop/dashboard/UnreadFeedsWidget.svelte";
import RecapSummaryWidget from "$lib/components/desktop/dashboard/RecapSummaryWidget.svelte";

const { isDesktop } = useViewport();
const stats = useFeedStats();

// Feed state
let feeds = $state<RenderFeed[]>([]);
let feedsLoading = $state(true);
let feedsError = $state<Error | null>(null);

// Recap state
let recapData = $state<RecapSummary | null>(null);
let recapLoading = $state(true);
let recapError = $state<Error | null>(null);

// Page reveal
let revealed = $state(false);

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

onMount(() => {
	if (!isDesktop) {
		goto("/home", { replaceState: true });
		return;
	}

	// Stagger page reveal
	requestAnimationFrame(() => {
		revealed = true;
	});

	// Fetch feeds
	getFeedsWithCursorClient(undefined, 5)
		.then((result) => {
			feeds = result.data ?? [];
		})
		.catch((err) => {
			feedsError = err as Error;
		})
		.finally(() => {
			feedsLoading = false;
		});

	// Fetch recap
	const transport = createClientTransport();
	getThreeDayRecap(transport)
		.then((data) => {
			recapData = data;
		})
		.catch((err) => {
			if (err instanceof ConnectError && err.code === Code.NotFound) {
				recapData = null;
				return;
			}
			recapError = err as Error;
		})
		.finally(() => {
			recapLoading = false;
		});
});
</script>

<svelte:head>
	<title>Dashboard - Alt</title>
</svelte:head>

{#if isDesktop}
	<div class="brief-page" class:revealed>
		<!-- Editorial Header -->
		<header class="brief-header">
			<span class="brief-date">{dateStr}</span>
			<h1 class="brief-title">Editorial Brief</h1>
			<div class="brief-rule" aria-hidden="true"></div>
		</header>

		<!-- Figures Bar -->
		<div class="section-reveal" style="--delay: 1;">
			<StatsBarWidget
				feedAmount={stats.feedAmount}
				totalArticlesAmount={stats.totalArticlesAmount}
				unsummarizedArticlesAmount={stats.unsummarizedArticlesAmount}
				isConnected={stats.isConnected}
			/>
			<div class="brief-rule" aria-hidden="true"></div>
		</div>

		<!-- Two-column content -->
		<div class="content-columns">
			<div class="section-reveal" style="--delay: 2;">
				<UnreadFeedsWidget
					{feeds}
					isLoading={feedsLoading}
					error={feedsError}
				/>
			</div>
			<div class="section-reveal" style="--delay: 3;">
				<RecapSummaryWidget
					{recapData}
					isLoading={recapLoading}
					error={recapError}
				/>
			</div>
		</div>
	</div>
{:else}
	<div class="redirect-state">
		<p class="redirect-text">Redirecting&hellip;</p>
	</div>
{/if}

<style>
	.brief-page {
		max-width: 1400px;
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.4s ease,
			transform 0.4s ease;
	}

	.brief-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	.brief-header {
		padding: 1.5rem 0 0;
	}

	.brief-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.brief-title {
		font-family: var(--font-display);
		font-size: 1.6rem;
		font-weight: 800;
		color: var(--alt-charcoal);
		letter-spacing: -0.01em;
		margin: 0.15rem 0 0;
		line-height: 1.2;
	}

	.brief-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	.content-columns {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 2rem;
		margin-top: 1.5rem;
	}

	.section-reveal {
		opacity: 0;
		transform: translateY(6px);
		animation: reveal 0.4s ease forwards;
		animation-delay: calc(var(--delay) * 100ms);
	}

	.redirect-state {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		min-height: 100dvh;
		background: var(--surface-bg);
	}

	.redirect-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		font-style: italic;
	}

	@keyframes reveal {
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.brief-page {
			opacity: 1;
			transform: none;
			transition: none;
		}
		.section-reveal {
			animation: none;
			opacity: 1;
			transform: none;
		}
	}
</style>
