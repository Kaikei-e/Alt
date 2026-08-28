<script lang="ts">
import { onMount } from "svelte";
import { removeFavoriteFeedClient } from "$lib/api/client";
import { getFavoriteFeedsWithCursorClient } from "$lib/api/client/feeds";
import DesktopFeedCard from "$lib/components/desktop/feeds/DesktopFeedCard.svelte";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedGrid from "$lib/components/desktop/feeds/FeedGrid.svelte";
import type { FeedGridApi } from "$lib/components/desktop/feeds/feed-grid-types";
import ClippingsEntry from "$lib/components/mobile/ClippingsEntry.svelte";
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
 * Un-starring a clipping.
 *
 * The removal now goes through the grid's own `removedUrls` rather than through
 * a page-local array the phone arm used to keep. That is what makes it survive
 * a rotation: the page no longer owns a second copy of the list to disagree
 * with.
 *
 * The card is taken off screen first and the server is told afterwards; if the
 * server refuses, `restoreFeedByUrl` puts it back exactly where it was. The
 * alternative rollback — re-reading the whole file — would throw away every
 * page the reader had scrolled to in order to undo one tap.
 */
async function handleRemoveFavorite(feedUrl: string) {
	if (!feedGridApi) return;
	feedGridApi.removeFeedByUrl(feedUrl);
	try {
		await removeFavoriteFeedClient(feedUrl);
	} catch (err) {
		console.error("Failed to remove favorite:", err);
		feedGridApi.restoreFeedByUrl(feedUrl);
	}
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
const CLIPPINGS_LIST_CLASS =
	"grid grid-cols-1 gap-0 md:grid-cols-3 lg:grid-cols-3 md:gap-4";
</script>

<svelte:head>
	<title>The Clippings File - Alt</title>
</svelte:head>

<!--
	One header, one list.

	The desktop arm rendered a `<FeedGrid>`; the phone arm rendered a second,
	hand-rolled copy of the same thing directly in this page — `mobileFeeds`,
	`mobileNextCursor`, `mobileHasMore` and its own `infiniteScroll`. Rotating
	the phone flipped the `{#if}`, destroyed whichever list was live and built
	the other from nothing: page one fetched again, every scrolled-to page gone,
	and a clipping the reader had just removed back on screen.

	Only the card differs now, and for a real reason: the desk card is a click
	target that opens the detail modal, and the phone entry is a self-contained
	article carrying its own Details / Remove / Open actions.
-->
<div class="clippings-page" class:revealed data-role="clippings-file-page">
	<header class="clippings-header">
		<span class="clippings-date">{dateStr}</span>
		<h1 class="clippings-title">The Clippings File</h1>
		<p class="clippings-subtitle">Your curated collection</p>
		<div class="clippings-rule" aria-hidden="true"></div>
	</header>

	<div class="clippings-body" data-role="clippings-feed-list">
		<FeedGrid
			onSelectFeed={handleSelectFeed}
			onReady={handleFeedGridReady}
			fetchFn={getFavoriteFeedsWithCursorClient}
			gridClass={CLIPPINGS_LIST_CLASS}
			emptyText="No clippings yet"
			loadingText="Retrieving your clippings"
		>
			{#snippet cardRenderer({ feed, isRead, onSelect }: { feed: RenderFeed; index: number; isRead: boolean; onSelect: (feed: RenderFeed) => void })}
				{#if isDesktop()}
					<DesktopFeedCard {feed} {isRead} {onSelect} />
				{:else}
					<ClippingsEntry {feed} onRemove={handleRemoveFavorite} />
				{/if}
			{/snippet}

			<!--
				The phone arm shipped a labelled empty state that says what starring
				an article does; the desktop grid had the single line "No clippings
				yet". Keeping the better of the two at both widths.
			-->
			{#snippet emptyContent()}
				<div class="empty-state" role="region" aria-label="Empty clippings state">
					<div class="empty-ornament" aria-hidden="true">&#9670;</div>
					<h2 class="empty-heading">No Clippings Yet</h2>
					<p class="empty-body">
						Star articles from the wire to add them to your clippings file.
					</p>
				</div>
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
	.clippings-page {
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.4s ease,
			transform 0.4s ease;
	}

	.clippings-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	/*
		The two headers differed only in type size and padding — presentation, so
		it belongs in a media query rather than in a branch that rebuilds the DOM.
	*/
	.clippings-header {
		padding: 1rem 1.25rem 0;
	}

	.clippings-body {
		/* The phone list held its clippings to a readable measure and centred
		   them; the desk grid runs the full column width. */
		max-width: 42rem;
		margin: 0 auto;
		padding: 0 1.25rem 1.25rem;
		width: 100%;
	}

	@media (width >= 48rem) {
		.clippings-header {
			padding: 1.5rem 0 0;
		}

		.clippings-body {
			max-width: none;
			margin: 0;
			padding: 0;
		}
	}

	.clippings-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.clippings-title {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0.1rem 0 0;
		line-height: 1.2;
	}

	.clippings-subtitle {
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-slate);
		margin: 0.1rem 0 0;
	}

	@media (width >= 48rem) {
		.clippings-title {
			font-size: 1.6rem;
			font-weight: 800;
			letter-spacing: -0.01em;
			margin: 0.15rem 0 0;
		}

		.clippings-subtitle {
			font-size: 0.85rem;
			margin: 0.2rem 0 0;
		}
	}

	.clippings-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 70dvh;
		padding: 1.5rem;
		text-align: center;
	}

	.empty-ornament {
		font-size: 1.5rem;
		color: var(--surface-border);
		margin-bottom: 1rem;
	}

	.empty-heading {
		font-family: var(--font-display);
		font-size: 1.4rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0 0 0.5rem;
	}

	.empty-body {
		font-family: var(--font-body);
		font-size: 0.9rem;
		line-height: 1.6;
		color: var(--alt-slate);
		max-width: 320px;
		margin: 0;
	}

	@media (prefers-reduced-motion: reduce) {
		.clippings-page {
			opacity: 1;
			transform: none;
			transition: none;
		}
	}
</style>
