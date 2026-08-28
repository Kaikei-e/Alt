<script lang="ts">
import { onMount } from "svelte";
import { getReadFeedsWithCursorClient } from "$lib/api/client/feeds";
import DesktopFeedCard from "$lib/components/desktop/feeds/DesktopFeedCard.svelte";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedGrid from "$lib/components/desktop/feeds/FeedGrid.svelte";
import type { FeedGridApi } from "$lib/components/desktop/feeds/feed-grid-types";
import EmptyViewedFeedsState from "$lib/components/mobile/EmptyViewedFeedsState.svelte";
import MorgueClipping from "$lib/components/mobile/MorgueClipping.svelte";
import type { RenderFeed } from "$lib/schema/feed";
import { isDesktop } from "$lib/stores/viewport.svelte";

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

let revealed = $state(false);

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
});

let selectedFeedUrl = $state<string | null>(null);
let isModalOpen = $state(false);
let feedGridApi = $state<FeedGridApi | null>(null);

const selectedFeed = $derived.by(() => {
	if (!selectedFeedUrl || !feedGridApi) return null;
	return feedGridApi.getFeedByUrl(selectedFeedUrl);
});

const currentIndex = $derived.by(() => {
	if (!selectedFeedUrl || !feedGridApi) return -1;
	const feeds = feedGridApi.getVisibleFeeds();
	return feeds.findIndex(
		(f: RenderFeed) => f.normalizedUrl === selectedFeedUrl,
	);
});

const totalCount = $derived(feedGridApi?.getVisibleFeeds().length ?? 0);
const hasPrevious = $derived(currentIndex > 0);
const hasNext = $derived(currentIndex >= 0 && currentIndex < totalCount - 1);

function handlePrevious() {
	if (!feedGridApi || currentIndex <= 0) return;
	const previous = feedGridApi.getVisibleFeeds()[currentIndex - 1];
	if (previous) selectedFeedUrl = previous.normalizedUrl;
}

function handleNext() {
	if (!feedGridApi || currentIndex >= totalCount - 1) return;
	const next = feedGridApi.getVisibleFeeds()[currentIndex + 1];
	if (next) selectedFeedUrl = next.normalizedUrl;
}

function handleSelectFeed(feed: RenderFeed, _index: number, _total: number) {
	selectedFeedUrl = feed.normalizedUrl;
	isModalOpen = true;
}

function handleFeedGridReady(api: FeedGridApi) {
	feedGridApi = api;
}

/**
 * A column of clippings on a phone, three across from `md` up.
 *
 * Two class strings before this: the grid default (`grid-cols-2
 * md:grid-cols-3 lg:grid-cols-3 gap-4`, whose two-column narrow half was dead
 * code — that arm only ever rendered at 768px and up) and the phone list's
 * `flex flex-col`. A single-column grid with no gap is that flex column, so one
 * breakpoint holds both, and `lg:grid-cols-3` stays spelled out because "three
 * across on a wide screen, not four" is a decision someone made on purpose.
 */
const MORGUE_LIST_CLASS =
	"grid grid-cols-1 gap-0 md:grid-cols-3 lg:grid-cols-3 md:gap-4";
</script>

<svelte:head>
	<title>The Morgue Desk - Alt</title>
</svelte:head>

<!--
	One header, one list.

	This page used to be an `{#if isDesktop()}` with a `<FeedGrid>` on one side
	and a 356-line `ViewedFeedsClient` on the other — two implementations of "the
	filed clippings, paginated by cursor", each holding its own `feeds`, cursor
	and `hasMore`. Rotating the phone destroyed the live one and mounted the
	other from nothing, so the reader lost every page they had scrolled to and
	paid for the first page again.

	Only the card differs now, and it differs for a real reason: the desk card is
	a click target that opens the detail modal, and the phone clipping is a
	self-contained article with its own Open and Details actions.
-->
<div class="morgue-page" class:revealed data-role="morgue-desk-page">
	<header class="morgue-header">
		<span class="morgue-date">{dateStr}</span>
		<h1 class="morgue-title">The Morgue Desk</h1>
		<div class="morgue-rule" aria-hidden="true"></div>
	</header>

	<div class="morgue-body" data-role="morgue-feed-list">
		<FeedGrid
			onSelectFeed={handleSelectFeed}
			onReady={handleFeedGridReady}
			fetchFn={getReadFeedsWithCursorClient}
			gridClass={MORGUE_LIST_CLASS}
			emptyText="Nothing filed yet"
			loadingText="Retrieving filed clippings"
		>
			{#snippet cardRenderer({ feed, isRead, onSelect }: { feed: RenderFeed; index: number; isRead: boolean; onSelect: (feed: RenderFeed) => void })}
				{#if isDesktop()}
					<DesktopFeedCard {feed} {isRead} {onSelect} />
				{:else}
					<MorgueClipping {feed} />
				{/if}
			{/snippet}

			<!--
				The phone list shipped a labelled region with a heading and a line
				saying what will show up here; the desktop grid had one grey
				sentence. Folding the two together keeps the better of the two at
				both widths rather than whichever the desktop happened to own.
			-->
			{#snippet emptyContent()}
				<EmptyViewedFeedsState />
			{/snippet}
		</FeedGrid>
	</div>
</div>

<FeedDetailModal
	bind:open={isModalOpen}
	feed={selectedFeed}
	onOpenChange={(open: boolean) => (isModalOpen = open)}
	{hasPrevious}
	{hasNext}
	onPrevious={handlePrevious}
	onNext={handleNext}
	feeds={feedGridApi?.getVisibleFeeds() ?? []}
	{currentIndex}
/>

<style>
	.morgue-page {
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.4s ease,
			transform 0.4s ease;
	}

	.morgue-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	/*
		The two headers differed only in type size and padding — presentation, so
		it belongs in a media query rather than in a branch that rebuilds the DOM.
	*/
	.morgue-header {
		padding: 1rem 1.25rem 0;
	}

	.morgue-body {
		/* The phone list held its clippings to a readable measure and centred
		   them; the desk grid runs the full column width. */
		max-width: 42rem;
		margin: 0 auto;
		padding: 0 1.25rem 1.25rem;
		width: 100%;
	}

	@media (width >= 48rem) {
		.morgue-header {
			padding: 1.5rem 0 0;
		}

		.morgue-body {
			max-width: none;
			margin: 0;
			padding: 0;
		}
	}

	.morgue-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.morgue-title {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0.1rem 0 0;
		line-height: 1.2;
	}

	@media (width >= 48rem) {
		.morgue-title {
			font-size: 1.6rem;
			font-weight: 800;
			letter-spacing: -0.01em;
			margin: 0.15rem 0 0;
		}
	}

	.morgue-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	@media (prefers-reduced-motion: reduce) {
		.morgue-page {
			opacity: 1;
			transform: none;
			transition: none;
		}
	}
</style>
