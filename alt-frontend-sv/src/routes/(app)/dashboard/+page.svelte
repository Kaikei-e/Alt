<script lang="ts">
import { Code, ConnectError } from "@connectrpc/connect";
import { onMount } from "svelte";
import { goto } from "$app/navigation";
import { getFeedsWithCursorClient } from "$lib/api/client/feeds";
import RecapSummaryWidget from "$lib/components/desktop/dashboard/RecapSummaryWidget.svelte";
import StatsBarWidget from "$lib/components/desktop/dashboard/StatsBarWidget.svelte";
import UnreadFeedsWidget from "$lib/components/desktop/dashboard/UnreadFeedsWidget.svelte";
import { createClientTransport, getThreeDayRecap } from "$lib/connect";
import { useFeedStats } from "$lib/hooks/useFeedStats.svelte";
import type { RenderFeed } from "$lib/schema/feed";
import type { RecapSummary } from "$lib/schema/recap";
import { isDesktop, isMobile } from "$lib/stores/viewport.svelte";

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

// Deliberately `onMount` and deliberately not an `$effect`: this is an
// arrival check, not a viewport binding. The brief is typeset for a wide
// screen, so a phone-sized *arrival* is handed to Knowledge Home. Turning a
// phone is not an arrival — a `goto` in an `$effect` would throw away a brief
// the reader is part-way through on every rotation, and on every drag of a
// desktop window across 768px.
//
// Because `onMount` will not run again, the narrow branch below has to stand
// on its own. It used to read "Redirecting…" and rely on this redirect to make
// that true; once the viewport check became reactive, rotating upright flipped
// to that branch with no redirect behind it and the sentence became a
// permanent lie with nothing to act on.
onMount(() => {
	if (isMobile()) {
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

{#if isDesktop()}
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
	<div class="narrow-state">
		<p class="narrow-title">Editorial Brief</p>
		<p class="narrow-text">
			The brief is typeset for a wide screen. Turn the phone back to
			landscape to read it here.
		</p>
		<a class="narrow-link" href="/home">Go to Knowledge Home</a>
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

	.narrow-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.6rem;
		padding: 0 1.5rem;
		text-align: center;
		min-height: 100vh;
		min-height: 100dvh;
		background: var(--surface-bg);
	}

	.narrow-title {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.narrow-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		max-width: 24rem;
		margin: 0;
	}

	.narrow-link {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-charcoal);
		text-decoration: underline;
		text-underline-offset: 0.25em;
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
