<script lang="ts">
import { getContext, onMount } from "svelte";
import {
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "$lib/api/client/feeds";
import DesktopFeedCard from "$lib/components/desktop/feeds/DesktopFeedCard.svelte";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedFilters from "$lib/components/desktop/feeds/FeedFilters.svelte";
import FeedGrid from "$lib/components/desktop/feeds/FeedGrid.svelte";
import type { FeedGridApi } from "$lib/components/desktop/feeds/feed-grid-types";
import Toast from "$lib/components/knowledge-home/Toast.svelte";
import EmptyFeedState from "$lib/components/mobile/EmptyFeedState.svelte";
import FeedCard from "$lib/components/mobile/FeedCard.svelte";
import MobileFeedExcludeFilter from "$lib/components/mobile/feeds/MobileFeedExcludeFilter.svelte";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import { MARK_AS_READ_FAILED_MESSAGE } from "$lib/feeds/mark-as-read-feedback";
import type { RenderFeed } from "$lib/schema/feed";
import {
	CONNECTION_RECOVERY_KEY,
	type ConnectionRecoveryStore,
} from "$lib/stores/connection-recovery.svelte";
import { useToastStore } from "$lib/stores/toast.svelte";
import { isDesktop, isMobile } from "$lib/stores/viewport.svelte";

interface PageData {
	initialFeeds?: RenderFeed[];
	error?: string;
}

const { data }: { data: PageData } = $props();
const toast = useToastStore();
const connectionRecovery = getContext<ConnectionRecoveryStore | undefined>(
	CONNECTION_RECOVERY_KEY,
);

/**
 * Which wire the reader arrived at, decided once.
 *
 * The phone list only ever showed unread dispatches; the desk grid showed
 * everything, with "unread only" as a filter the reader could tick. Those are
 * two different questions asked of two different RPCs, and now that both
 * viewports share one grid, one of them has to be the starting point.
 *
 * It is read here, outside any reactive context, on purpose: a rotation must
 * not change which wire is being read. If `unreadOnly` followed the viewport,
 * turning the phone would change the data source, and the grid would answer by
 * throwing the list away and fetching page one again — the exact failure this
 * whole change exists to remove. The desk's filter is still live; it just has
 * to be a decision the reader makes, not one the accelerometer makes.
 */
const arrivedOnPhone = isMobile();

let selectedFeedUrl = $state<string | null>(null);
let isModalOpen = $state(false);
let filters = $state({
	unreadOnly: arrivedOnPhone,
	sortBy: "date_desc",
	excludedFeedLinkIds: [] as string[],
});
let feedGridApi = $state<FeedGridApi | null>(null);
let feedSources = $state<ConnectFeedSource[]>([]);
let isProcessingMarkAsRead = $state(false);
let isMarkingAsRead = $state(false);
let revealed = $state(false);

/**
 * What a screen reader hears when a dispatch leaves the wire.
 *
 * Marking read removes a card and changes nothing else on the page, so without
 * this the only feedback is visual. The phone list carried it; the desk grid
 * did not, and the desk grid has the same silent removal in its detail modal.
 * One region now serves both.
 */
let liveRegionMessage = $state("");
let liveRegionTimer: ReturnType<typeof setTimeout> | null = null;

function announce(message: string, holdMs: number) {
	if (liveRegionTimer) clearTimeout(liveRegionTimer);
	liveRegionMessage = message;
	liveRegionTimer = setTimeout(() => {
		liveRegionMessage = "";
		liveRegionTimer = null;
	}, holdMs);
}

/**
 * The server-rendered first screen, handed to the grid so the reader sees
 * dispatches before any client request goes out.
 *
 * `+page.server.ts` loads it from the *unread* RPC, so it only belongs under a
 * grid that is asking the same question. A desk arrival starts on the full
 * wire and gets nothing here rather than a screenful of rows that quietly
 * exclude everything already read.
 */
const seededFeeds = $derived(arrivedOnPhone ? (data.initialFeeds ?? []) : []);

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

async function loadFeedSources() {
	try {
		feedSources = await listSubscriptionsClient();
	} catch (e) {
		console.error("Failed to load feed sources:", e);
	}
}

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
	loadFeedSources();
});

// Safari drops connections after a long spell in the background. One recovery
// handler for both viewports now — the phone list used to keep its own.
$effect(() => {
	if (!connectionRecovery) return;
	const unsubscribe = connectionRecovery.subscribe((info) => {
		console.info("[Feeds] Connection recovery triggered:", info.reason);
		loadFeedSources();
		feedGridApi?.refresh();
	});
	return unsubscribe;
});

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

function handleFilterChange(newFilters: {
	unreadOnly: boolean;
	sortBy: string;
	excludedFeedLinkIds: string[];
}) {
	filters = newFilters;
}

async function handleMarkAsReadInModal() {
	const feed = selectedFeed;
	if (!feed || isMarkingAsRead) return;

	try {
		isMarkingAsRead = true;
		await updateFeedReadStatusClient(feed.normalizedUrl);
		handleMarkAsRead(feed.normalizedUrl);
		announce("Feed marked as read", 1000);
	} catch (error) {
		console.error("Failed to mark feed as read:", error);
		announce(MARK_AS_READ_FAILED_MESSAGE, 4000);
		toast.push(MARK_AS_READ_FAILED_MESSAGE, "error", 4000);
	} finally {
		isMarkingAsRead = false;
	}
}

/**
 * The card's own "Mark as Read".
 *
 * Optimistic, unlike the modal's: a tap on a card in a list has to answer
 * instantly, and there is no open modal whose contents would have to be walked
 * back. `restoreFeedByUrl` is the undo — before this the phone list rolled back
 * by deleting from its own read-set, and the grid had no equivalent, so the
 * only rollback available would have been a full refresh.
 */
async function handleMarkAsReadFromCard(feedUrl: string) {
	if (!feedGridApi) return;

	handleMarkAsRead(feedUrl);
	announce("Feed marked as read", 1000);

	try {
		await updateFeedReadStatusClient(feedUrl);
	} catch (e) {
		console.error("Failed to mark feed as read:", e);
		feedGridApi.restoreFeedByUrl(feedUrl);
		announce(MARK_AS_READ_FAILED_MESSAGE, 4000);
		toast.push(MARK_AS_READ_FAILED_MESSAGE, "error", 4000);
	}
}

function handleMarkAsRead(feedUrl: string) {
	if (isProcessingMarkAsRead || !feedGridApi) return;

	isProcessingMarkAsRead = true;

	// Whether the modal is looking at the feed being removed. It always was
	// when this was reachable only from the modal's own footer; the card's
	// inline button reaches it too now, and moving the modal's selection to
	// some unrelated "next" feed because a card three rows down was tapped is
	// not what that reader asked for.
	const wasOnScreen = selectedFeedUrl === feedUrl;

	const currentFeeds = feedGridApi.getVisibleFeeds();
	const currentIdx = currentFeeds.findIndex(
		(f: RenderFeed) => f.normalizedUrl === feedUrl,
	);
	const isLastFeed = currentIdx === currentFeeds.length - 1;

	const { nextFeedUrl, totalCount } = feedGridApi.removeFeedByUrl(feedUrl);

	if (wasOnScreen) {
		if (totalCount === 0 || isLastFeed || nextFeedUrl === null) {
			isModalOpen = false;
			selectedFeedUrl = null;
		} else {
			selectedFeedUrl = nextFeedUrl;
		}
	}

	feedGridApi.fetchReplacementFeed();
	isProcessingMarkAsRead = false;
}

function handleFeedGridReady(api: FeedGridApi) {
	feedGridApi = api;
}

/**
 * A column of dispatches on a phone, a grid from `md` up.
 *
 * Two class strings before this: the grid default (`grid-cols-2
 * md:grid-cols-3 lg:grid-cols-3 gap-4`, whose two-column narrow half was dead
 * code — that arm only ever rendered at 768px and up) and the phone list's
 * `flex flex-col`. A single-column grid with no gap is that flex column, so one
 * breakpoint holds both, and `lg:grid-cols-3` stays spelled out because "three
 * across on a wide screen, not four" is a decision someone made on purpose.
 */
const WIRE_LIST_CLASS =
	"grid grid-cols-1 gap-0 md:grid-cols-3 lg:grid-cols-3 md:gap-4";
</script>

<svelte:head>
	<title>Feeds - Alt</title>
</svelte:head>

<Toast items={toast.items} onDismiss={toast.remove} />

<div
	aria-live="assertive"
	aria-atomic="true"
	class="sr-only"
>
	{liveRegionMessage}
</div>

<!--
	One header, one list.

	This page used to be an `{#if isDesktop()}` with a `<FeedGrid>` on one side
	and a 562-line `FeedsClient` on the other — two implementations of "the wire,
	paginated by cursor", each holding its own feeds, cursor, read-set and
	infinite-scroll sentinel. Rotating the phone flipped the branch, destroyed
	the live one and mounted the other from nothing: page one fetched again,
	every scrolled-to page gone, and articles the reader had marked read back on
	the wire.

	What is left inside a branch is what genuinely differs — the filter control,
	and the card.
-->
<div class="wire-page" class:revealed>
	<header class="wire-header">
		<span class="wire-date">{dateStr}</span>
		<h1 class="wire-title">Feeds</h1>
		<div class="wire-rule" aria-hidden="true"></div>
	</header>

	{#if isDesktop()}
		<FeedFilters
			unreadOnly={filters.unreadOnly}
			sortBy={filters.sortBy}
			excludedFeedLinkIds={filters.excludedFeedLinkIds}
			{feedSources}
			onFilterChange={handleFilterChange}
		/>
	{:else}
		<!--
			The phone gets the source exclusion only: sort order and read/unread
			are desk controls, and the phone arrives on the unread wire already.
			Both write the same `filters.excludedFeedLinkIds` the grid reads, so
			an exclusion set on one side is still set after a rotation — it used to
			be two separate arrays that never met.
		-->
		<MobileFeedExcludeFilter
			sources={feedSources}
			excludedFeedLinkIds={filters.excludedFeedLinkIds}
			onExclude={(ids: string[]) =>
				(filters = { ...filters, excludedFeedLinkIds: ids })}
			onClearExclusion={() =>
				(filters = { ...filters, excludedFeedLinkIds: [] })}
		/>
	{/if}

	<div class="wire-body">
		<FeedGrid
			onSelectFeed={handleSelectFeed}
			unreadOnly={filters.unreadOnly}
			sortBy={filters.sortBy}
			excludedFeedLinkIds={filters.excludedFeedLinkIds}
			onReady={handleFeedGridReady}
			initialFeeds={seededFeeds}
			gridClass={WIRE_LIST_CLASS}
			gridTestId="virtual-feed-list"
			emptyText="No dispatches on the wire"
			loadingText="Retrieving dispatches"
		>
			{#snippet cardRenderer({ feed, isRead, onSelect }: { feed: RenderFeed; index: number; isRead: boolean; onSelect: (feed: RenderFeed) => void })}
				{#if isDesktop()}
					<DesktopFeedCard {feed} {isRead} {onSelect} />
				{:else}
					<FeedCard
						{feed}
						isReadStatus={false}
						setIsReadStatus={(feedLink: string) =>
							void handleMarkAsReadFromCard(feedLink)}
					/>
				{/if}
			{/snippet}

			<!--
				The phone arm shipped a labelled empty region that says what to do
				next; the desk grid had the sentence "No dispatches on the wire"
				and no way forward. Keeping the better of the two at both widths.
			-->
			{#snippet emptyContent()}
				<EmptyFeedState />
			{/snippet}

			<!--
				And the phone arm's skeleton, for the same reason: a shimmer that
				is the shape of the answer reads better than a line of italics,
				at either width.
			-->
			{#snippet loadingContent()}
				<div class={WIRE_LIST_CLASS} aria-hidden="true">
					{#each Array(5) as _, i}
						<div class="skeleton-entry" style="animation-delay: {i * 80}ms;">
							<div class="skeleton-line skeleton-line--title animate-shimmer-warm"></div>
							<div class="skeleton-line skeleton-line--full animate-shimmer-warm"></div>
							<div class="skeleton-line skeleton-line--short animate-shimmer-warm"></div>
						</div>
					{/each}
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
	onMarkAsRead={handleMarkAsReadInModal}
	isMarkingAsRead={isMarkingAsRead || isProcessingMarkAsRead}
/>

<style>
	.sr-only {
		position: absolute;
		left: -10000px;
		width: 1px;
		height: 1px;
		overflow: hidden;
	}

	.wire-page {
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.4s ease,
			transform 0.4s ease;
	}

	.wire-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	/*
		The two headers differed only in type size and padding — presentation, so
		it belongs in a media query rather than in a branch that rebuilds the DOM.
	*/
	.wire-header {
		padding: 1rem 1.25rem 0;
	}

	.wire-body {
		/* The phone list held its dispatches to a readable measure and centred
		   them; the desk grid runs the full column width. */
		max-width: 42rem;
		margin: 0 auto;
		padding: 0 1.25rem 1.25rem;
		width: 100%;
	}

	@media (width >= 48rem) {
		.wire-header {
			padding: 1.5rem 0 0;
		}

		.wire-body {
			max-width: none;
			margin: 0;
			padding: 0;
		}
	}

	.wire-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.wire-title {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0.1rem 0 0;
		line-height: 1.2;
	}

	@media (width >= 48rem) {
		.wire-title {
			font-size: 1.6rem;
			font-weight: 800;
			letter-spacing: -0.01em;
			margin: 0.15rem 0 0;
		}
	}

	.wire-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	.skeleton-entry {
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--surface-border);
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.skeleton-line {
		height: 0.75rem;
	}

	.skeleton-line--title {
		width: 75%;
		height: 1rem;
	}

	.skeleton-line--full {
		width: 100%;
	}

	.skeleton-line--short {
		width: 60%;
	}

	@media (prefers-reduced-motion: reduce) {
		.wire-page {
			opacity: 1;
			transform: none;
			transition: none;
		}
	}
</style>
