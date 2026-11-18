<script lang="ts">
import { onMount } from "svelte";
import {
	batchPrefetchImagesClient,
	getFeedContentOnTheFlyClient,
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "#lib/api/client/index.js";
import type { ConnectFeedSource } from "#lib/connect/feeds.js";
import { QueueArticleOutcome } from "#lib/connect/reading.js";
import { renderFeedToReaderItem } from "#lib/reader/mappers.js";
import { openReaderOverlay } from "#lib/reader/reader-overlay.svelte.js";
import { readTodayQueue } from "#lib/reading/read-today-queue.svelte.js";
import { ReadingSurface } from "#lib/reading/reading-events.js";
import type { RenderFeed, SanitizedFeed } from "#lib/schema/feed.js";
import { toRenderFeed } from "#lib/schema/feed.js";
import { articlePrefetcher } from "#lib/utils/articlePrefetcher.js";
import { canonicalize } from "#lib/utils/feed.js";
import { browser } from "$app/env";
import DispatchHeader from "./DispatchHeader.svelte";
import {
	createDispatchSession,
	type DismissKind,
} from "./dispatch-session.svelte";
import SwipeFilterSortSheet from "./SwipeFilterSortSheet.svelte";
import SwipeLoadingOverlay from "./SwipeLoadingOverlay.svelte";
import VisualPreviewCard from "./VisualPreviewCard.svelte";

interface Props {
	initialFeeds?: RenderFeed[];
	initialNextCursor?: string | null;
	initialArticleContent?: string | null;
	initialArticleId?: string | null;
	/**
	 * The SSR-resolved og:image for the first card, as a signed
	 * `/v1/images/proxy/...` path. Never a publisher URL — see
	 * `+page.server.ts`, which drops the raw fallback it used to end with.
	 */
	initialOgImageProxyUrl?: string | null;
	mode?: "default" | "visual-preview";
}

const {
	initialFeeds = [],
	initialNextCursor,
	initialArticleContent: _initialArticleContent,
	initialArticleId: _initialArticleId,
	initialOgImageProxyUrl,
	mode = "visual-preview",
}: Props = $props();

const isVisualPreview = $derived(mode === "visual-preview");
// ADR-000884: visual-preview was 10; reduced to 4 to stay within the
// backend's 5 rps per-host budget under the per-host serialization gate.
const prefetchAhead = $derived(isVisualPreview ? 4 : 2);

const PAGE_SIZE = 20;

// State
// svelte-ignore state_referenced_locally
let feeds = $state<RenderFeed[]>([...(initialFeeds ?? [])]);
// svelte-ignore state_referenced_locally
let cursor = $state<string | null>(initialNextCursor ?? null);
// svelte-ignore state_referenced_locally
let hasMore = $state(initialFeeds.length > 0 ? !!initialNextCursor : true);
let isLoading = $state(false);
let error = $state<string | null>(null);
let readFeeds = $state<Set<string>>(new Set());
let dismissedLinks = new Set<string>();
let activeIndex = $state(0);
// svelte-ignore state_referenced_locally
let isInitialLoading = $state((initialFeeds ?? []).length === 0);
let liveRegionMessage = $state("");
let noticeMessage = $state<string | null>(null);
let inFlightQueueCount = 0;

// Filter & sort state
let excludedFeedLinkIds = $state<string[]>([]);
let feedSources = $state<ConnectFeedSource[]>([]);
let sortOrder = $state<"newest" | "oldest">("newest");
let isFilterSheetOpen = $state(false);

function syncPendingQueueCount(hasPendingQueueSlot: boolean) {
	readTodayQueue.setPendingQueueCount(
		(hasPendingQueueSlot ? 1 : 0) + inFlightQueueCount,
	);
}

// Session bookkeeping: deferred pass/queue flush + one-card undo window
const session = createDispatchSession({
	onPass: (link) => updateFeedReadStatusClient(link),
	onQueue: (link) => commitQueue(link),
	onPendingChange: (pending) => {
		syncPendingQueueCount(pending?.kind === "queue");
	},
});

// Reactive OGP image update version counter
let ogImageVersion = $state(0);

// Derived
const activeFeed = $derived(feeds[activeIndex]);
const nextFeed = $derived(feeds[activeIndex + 1]);
const canQueue = $derived(readTodayQueue.remainingSlots > 0);

function isFeedQueued(feed: RenderFeed): boolean {
	return (
		readTodayQueue.isQueued(feed.articleId) ||
		readTodayQueue.isQueued(feed.normalizedUrl || feed.link)
	);
}

function fullQueueNotice(cap: number): string {
	return `Read today is full (${cap}). Read or remove one first.`;
}

function restoreFailedQueueCard(link: string, message: string) {
	dismissedLinks.delete(link);
	const idx = feeds.findIndex((f) => canonicalize(f.link) === link);
	if (idx !== -1) {
		if (idx === activeIndex - 1) {
			activeIndex = Math.max(0, activeIndex - 1);
		} else if (idx < activeIndex - 1) {
			const [card] = feeds.splice(idx, 1);
			if (card) {
				const insertAt = Math.max(0, activeIndex - 1);
				feeds.splice(insertAt, 0, card);
				feeds = [...feeds];
				activeIndex = insertAt;
			}
		}
	}
	session.revertCount();
	noticeMessage = message;
	liveRegionMessage = message;
}

async function commitQueue(link: string): Promise<void> {
	inFlightQueueCount++;
	syncPendingQueueCount(session.pending?.kind === "queue");

	try {
		const feed = feeds.find(
			(f) =>
				canonicalize(f.link) === link || canonicalize(f.normalizedUrl) === link,
		);
		const targetUrl = feed?.normalizedUrl || link;
		let articleId =
			feed?.articleId?.trim() ||
			articlePrefetcher.getCachedArticleId(targetUrl)?.trim() ||
			"";

		if (!articleId) {
			try {
				const res = await getFeedContentOnTheFlyClient(targetUrl);
				articleId = res.article_id?.trim() || "";
				if (articleId && feed) {
					handleArticleIdResolved(feed.link, articleId);
				}
			} catch {
				articleId = "";
			}
		}

		if (!articleId) {
			restoreFailedQueueCard(
				link,
				"Could not add to Read today. Card returned to the deck.",
			);
			return;
		}

		const result = await readTodayQueue.queue({
			articleId,
			surface: ReadingSurface.SWIPE,
			title: feed?.title,
			url: targetUrl,
			sourceName: feed?.author,
			imageUrl:
				feed?.ogImageProxyUrl ||
				articlePrefetcher.getCachedOgImage(targetUrl) ||
				"",
		});

		if (result.outcome === QueueArticleOutcome.QUEUE_FULL) {
			const cap = result.cap > 0 ? result.cap : readTodayQueue.cap;
			restoreFailedQueueCard(link, fullQueueNotice(cap));
		}
	} catch {
		restoreFailedQueueCard(
			link,
			"Could not add to Read today. Card returned to the deck.",
		);
	} finally {
		inFlightQueueCount = Math.max(0, inFlightQueueCount - 1);
		syncPendingQueueCount(session.pending?.kind === "queue");
	}
}

// Initialize read feeds
onMount(() => {
	if (!browser) return;

	const initializeReadFeeds = async () => {
		try {
			const res = await getReadFeedsWithCursorClient(undefined, 32);
			const links = new Set<string>();
			if (res?.data) {
				res.data.forEach((feed: SanitizedFeed) => {
					links.add(canonicalize(feed.link));
				});
			}
			readFeeds = links;
		} catch (err) {
			console.warn("Failed to initialize read feeds:", err);
		}
	};

	if ("requestIdleCallback" in window) {
		window.requestIdleCallback(() => void initializeReadFeeds());
	} else {
		setTimeout(() => void initializeReadFeeds(), 100);
	}
});

// Hydrate Read today queue so remainingSlots and unread filtering are accurate
onMount(() => {
	void readTodayQueue.refresh().catch(() => {
		// Keep default cap when queue refresh fails on mount
	});
});

// Initial load if needed
onMount(() => {
	if (feeds.length === 0 && hasMore) {
		void loadMore();
	} else {
		isInitialLoading = false;
	}
});

// Load feed sources for filter
onMount(() => {
	void (async () => {
		try {
			feedSources = await listSubscriptionsClient();
		} catch (e) {
			console.warn("Failed to load feed sources:", e);
		}
	})();
});

// Wire OGP image callback for reactivity
onMount(() => {
	articlePrefetcher.setOnOgImageFetched(() => {
		ogImageVersion++;
	});
	return () => articlePrefetcher.setOnOgImageFetched(null);
});

// Commit the pending action when the surface is left or backgrounded
onMount(() => {
	const flush = () => session.flush();
	window.addEventListener("pagehide", flush);
	return () => {
		window.removeEventListener("pagehide", flush);
		session.flush();
		readTodayQueue.setPendingQueueCount(0);
	};
});

// Wire articleId callback to trigger batch image prefetch in visual-preview mode
onMount(() => {
	if (!isVisualPreview) return;

	articlePrefetcher.setOnArticleIdCached((feedUrl, articleId) => {
		batchPrefetchImagesClient([articleId])
			.then((results) => {
				for (const info of results) {
					if (info.articleId === articleId && info.proxyUrl) {
						articlePrefetcher.seedCache(
							feedUrl,
							articlePrefetcher.getCachedContent(feedUrl) || "",
							articleId,
							info.proxyUrl,
						);
					}
				}
			})
			.catch((err) => {
				console.warn(
					"[SwipeFeedScreen] Batch image prefetch for new articleId failed:",
					err,
				);
			});
	});
	return () => articlePrefetcher.setOnArticleIdCached(null);
});

// Filter remaining (not-yet-swiped) deck items whenever queue membership changes
// without shifting cards before activeIndex.
$effect(() => {
	void readTodayQueue.items;
	if (activeIndex >= feeds.length) return;
	const tail = feeds.slice(activeIndex);
	if (!tail.some((f) => isFeedQueued(f))) return;
	feeds = [
		...feeds.slice(0, activeIndex),
		...tail.filter((f) => !isFeedQueued(f)),
	];
});

// The card's og:image, as a signed proxy path or null.
const currentOgImage = $derived.by(() => {
	void ogImageVersion;
	if (!activeFeed) return null;
	const cached = articlePrefetcher.getCachedOgImage(activeFeed.normalizedUrl);
	if (cached) return cached;
	if (activeFeed.ogImageProxyUrl) return activeFeed.ogImageProxyUrl;
	if (activeIndex === 0 && initialOgImageProxyUrl) {
		return initialOgImageProxyUrl;
	}
	return null;
});

// Prefetch next feeds.
$effect(() => {
	if (feeds.length > 0) {
		articlePrefetcher.triggerPrefetch(feeds, activeIndex, prefetchAhead);
	}
});

// Load more when running low
$effect(() => {
	if (
		!isLoading &&
		hasMore &&
		feeds.length - activeIndex < 5 &&
		feeds.length > 0
	) {
		void loadMore();
	}
});

function filterIncomingFeeds(newFeeds: RenderFeed[]): RenderFeed[] {
	return newFeeds.filter((f) => {
		const link = canonicalize(f.link);
		return (
			!readFeeds.has(link) && !dismissedLinks.has(link) && !isFeedQueued(f)
		);
	});
}

async function loadMore() {
	if (isLoading || !hasMore) return;
	isLoading = true;
	error = null;

	const isFirstLoad = feeds.length === 0;

	try {
		if (isFirstLoad) {
			const res = await getFeedsWithCursorClient(
				cursor ?? undefined,
				PAGE_SIZE,
				excludedFeedLinkIds.length > 0 ? excludedFeedLinkIds : undefined,
			);
			const newFeeds = res.data.map((f: SanitizedFeed) => toRenderFeed(f));
			const filtered = filterIncomingFeeds(newFeeds);

			feeds = [...feeds, ...filtered];
			cursor = res.next_cursor;
			hasMore = res.next_cursor !== null;
			isInitialLoading = false;

			const firstFeed = filtered[0];
			if (firstFeed) {
				const cacheKey = firstFeed.normalizedUrl;

				if (!articlePrefetcher.getCachedContent(cacheKey)) {
					getFeedContentOnTheFlyClient(cacheKey)
						.then((articleRes) => {
							if (articleRes.content) {
								articlePrefetcher.seedCache(
									cacheKey,
									articleRes.content,
									articleRes.article_id || "",
									articleRes.og_image_proxy_url || null,
								);
							}
						})
						.catch((err) => {
							console.warn(
								`[SwipeFeedScreen] Failed to fetch article content for first feed: ${cacheKey}`,
								err,
							);
						});
				}
			}
		} else {
			const res = await getFeedsWithCursorClient(
				cursor ?? undefined,
				PAGE_SIZE,
				excludedFeedLinkIds.length > 0 ? excludedFeedLinkIds : undefined,
			);
			const newFeeds = res.data.map((f: SanitizedFeed) => toRenderFeed(f));
			const filtered = filterIncomingFeeds(newFeeds);

			feeds = [...feeds, ...filtered];
			cursor = res.next_cursor;
			hasMore = res.next_cursor !== null;

			if (isVisualPreview && filtered.length > 0) {
				triggerBatchImagePrefetch(filtered);
			}
		}
	} catch (err) {
		console.warn("Error loading feeds:", err);
		error = "Failed to load feeds";
	} finally {
		isLoading = false;
		isInitialLoading = false;
	}
}

function triggerBatchImagePrefetch(newFeeds: RenderFeed[]) {
	const articleIds: string[] = [];
	for (const feed of newFeeds.slice(0, 10)) {
		if (feed.articleId) {
			articleIds.push(feed.articleId);
		}
	}
	if (articleIds.length === 0) return;

	batchPrefetchImagesClient(articleIds)
		.then((results) => {
			for (const info of results) {
				for (const feed of newFeeds) {
					if (feed.articleId === info.articleId && info.proxyUrl) {
						articlePrefetcher.seedCache(
							feed.normalizedUrl,
							articlePrefetcher.getCachedContent(feed.normalizedUrl) || "",
							info.articleId,
							info.proxyUrl,
						);
						break;
					}
				}
			}
		})
		.catch((err) => {
			console.warn("[SwipeFeedScreen] Batch image prefetch failed:", err);
		});
}

function handleExclude(ids: string[]) {
	excludedFeedLinkIds = ids;
	resetAndReload();
}

function handleClearExclusion() {
	excludedFeedLinkIds = [];
	resetAndReload();
}

function resetAndReload() {
	feeds = [];
	activeIndex = 0;
	cursor = null;
	hasMore = true;
	isInitialLoading = true;
	void loadMore();
}

async function handleDismiss(direction: number): Promise<boolean> {
	if (!activeFeed) return false;

	noticeMessage = null;
	const kind: DismissKind = direction > 0 ? "queue" : "pass";

	if (kind === "queue" && readTodayQueue.remainingSlots <= 0) {
		const msg = fullQueueNotice(readTodayQueue.cap);
		noticeMessage = msg;
		liveRegionMessage = msg;
		return false;
	}

	const currentLink = canonicalize(activeFeed.link);
	dismissedLinks.add(currentLink);

	if (kind === "pass") {
		readFeeds.add(currentLink);
		readFeeds = new Set(readFeeds);
		articlePrefetcher.markAsDismissed(currentLink);
		liveRegionMessage = "Passed";
	} else {
		liveRegionMessage = "Added to Read today";
	}

	setTimeout(() => {
		liveRegionMessage = "";
	}, 1000);

	activeIndex++;
	session.dismiss(currentLink, kind);

	// With no next card on screen there may be no next dismissal — commit
	// now rather than gambling on the pagehide flush surviving unload.
	if (!feeds[activeIndex]) {
		session.flush();
	}

	return true;
}

function handleUndo() {
	noticeMessage = null;
	const restored = session.undo();
	if (!restored) return;

	dismissedLinks.delete(restored.link);
	if (restored.kind === "pass") {
		readFeeds.delete(restored.link);
		readFeeds = new Set(readFeeds);
	}
	activeIndex = Math.max(0, activeIndex - 1);

	liveRegionMessage = "Returned card to the pile";
	setTimeout(() => {
		liveRegionMessage = "";
	}, 1000);
}

function handleArticleIdResolved(feedLink: string, articleId: string) {
	// Only reassign when something actually changed: `feeds` drives the prefetch
	// $effect, and every card called this on mount, so an unconditional rebuild
	// re-ran the effect (and rescheduled the prefetch ladder) for nothing.
	const index = feeds.findIndex((f) => f.link === feedLink);
	if (index === -1 || feeds[index]?.articleId === articleId) return;
	feeds = feeds.map((f, i) => (i === index ? { ...f, articleId } : f));
}

function removeArticlesByIdsFromDeck(articleIds: string[]) {
	if (articleIds.length === 0) return;
	const idSet = new Set(articleIds);
	for (const f of feeds) {
		if (f.articleId && idSet.has(f.articleId)) {
			readFeeds.add(canonicalize(f.link));
		}
	}
	readFeeds = new Set(readFeeds);
	if (activeIndex >= feeds.length) return;
	const tail = feeds.slice(activeIndex);
	if (!tail.some((f) => f.articleId && idSet.has(f.articleId))) return;
	feeds = [
		...feeds.slice(0, activeIndex),
		...tail.filter((f) => !f.articleId || !idSet.has(f.articleId)),
	];
}

function handleOpenArticle() {
	if (!activeFeed) return;
	noticeMessage = null;
	openReaderOverlay({
		surface: ReadingSurface.SWIPE,
		items: feeds.map(renderFeedToReaderItem),
		currentIndex: activeIndex,
		onArticleIdResolved: (url, articleId) => {
			const match = feeds.find(
				(f) => f.normalizedUrl === url || f.link === url,
			);
			if (match) {
				handleArticleIdResolved(match.link, articleId);
			}
		},
		onArticlesRead: removeArticlesByIdsFromDeck,
	});
}
</script>

<div class="swipe-screen">
	<!-- Live Region -->
	<div
		aria-live="polite"
		aria-atomic="true"
		class="sr-only"
  >
    {liveRegionMessage}
  </div>

	<DispatchHeader
		mode={mode}
		readCount={session.readCount}
		canUndo={session.canUndo}
		onUndo={handleUndo}
		onOpenFilter={() => {
			isFilterSheetOpen = true;
		}}
		filterActive={excludedFeedLinkIds.length > 0}
	/>

	{#if noticeMessage}
		<p class="swipe-notice" role="alert" data-testid="swipe-notice">
			{noticeMessage}
		</p>
	{/if}

	{#if isInitialLoading}
		<div class="initial-loading">
			<div class="loading-dot" aria-hidden="true"></div>
			<p class="loading-text">Loading feeds...</p>
		</div>
	{:else if error && feeds.length === 0}
		<div class="empty-state">
			<p class="error-title">Error loading feeds</p>
			<p class="error-message">{error}</p>
			<button
				type="button"
				class="action-btn"
				onclick={() => void loadMore()}
      >
        Retry
      </button>
		</div>
	{:else if activeFeed}
		<div class="card-container">
			<!-- Next card (background) -->
			{#if nextFeed}
        <div
          class="background-card"
          aria-hidden="true"
        ></div>
			{/if}

			<!-- Active card -->
			{#key activeFeed.id}
				<VisualPreviewCard
					feed={activeFeed}
					statusMessage={liveRegionMessage}
					onDismiss={handleDismiss}
					onOpenArticle={handleOpenArticle}
					thumbnailProxyUrl={currentOgImage}
					isBusy={isLoading}
					isLcp={activeIndex === 0}
					{canQueue}
				/>
			{/key}
		</div>

		<div class="swipe-actions" data-testid="swipe-actions">
			<button
				type="button"
				class="swipe-action-btn swipe-action-btn--pass"
				data-testid="swipe-pass-button"
				onclick={() => void handleDismiss(-1)}
			>
				Pass
			</button>
			<button
				type="button"
				class="swipe-action-btn swipe-action-btn--queue"
				data-testid="swipe-queue-button"
				onclick={() => void handleDismiss(1)}
			>
				Read today
			</button>
		</div>
	{:else}
		<div class="empty-state">
			<p class="empty-text">No more feeds</p>
			<button
				type="button"
				class="action-btn"
				onclick={() => resetAndReload()}
      >
        Refresh
      </button>
		</div>
	{/if}

	<SwipeLoadingOverlay isVisible={isLoading} />
	<SwipeFilterSortSheet
		bind:open={isFilterSheetOpen}
		sources={feedSources}
		excludedFeedLinkIds={excludedFeedLinkIds}
		sortOrder={sortOrder}
		onExclude={handleExclude}
		onClearExclusion={handleClearExclusion}
    onSortChange={(order) => { sortOrder = order; }}
	/>
</div>

<style>
  .swipe-screen {
    height: 100dvh;
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    overflow: hidden;
    background: var(--surface-bg);
  }

  .sr-only {
    position: absolute;
    left: -10000px;
    width: 1px;
    height: 1px;
    overflow: hidden;
  }

  .swipe-notice {
    width: 100%;
    max-width: 30rem;
    margin: 0.5rem 0 0;
    padding: 0.5rem 0.75rem;
    font-family: var(--font-body);
    font-size: 0.8rem;
    line-height: 1.4;
    color: var(--alt-error, #8c1d1d);
    background: var(--surface-2, #f5f4f1);
    border-top: 1px solid var(--surface-border);
    border-bottom: 1px solid var(--surface-border);
    flex-shrink: 0;
  }

  .card-container {
    position: relative;
    width: 100%;
    max-width: 30rem;
    flex: 1;
    min-height: 0;
    padding: 0 0.5rem;
    overflow: hidden;
  }

  .background-card {
    position: absolute;
    width: 100%;
    height: 100%;
    max-width: calc(100% - 1rem);
    background: var(--surface-bg);
    border: 1px solid var(--surface-border);
    opacity: 0.35;
    pointer-events: none;
  }

  .swipe-actions {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.5rem;
    width: 100%;
    max-width: 30rem;
    padding: 0.5rem 0.5rem calc(0.75rem + env(safe-area-inset-bottom, 0px));
    background: var(--surface-bg);
    flex-shrink: 0;
  }

  .swipe-action-btn {
    min-height: 48px;
    padding: 0.5rem 0.75rem;
    font-family: var(--font-body);
    font-size: 0.85rem;
    font-weight: 600;
    border: 1.5px solid var(--alt-charcoal);
    background: transparent;
    color: var(--alt-charcoal);
    cursor: pointer;
    touch-action: manipulation;
    transition: background 0.15s, color 0.15s;
  }

  .swipe-action-btn--queue {
    border-color: var(--alt-primary);
    background: var(--alt-primary);
    color: var(--surface-bg);
  }

  .swipe-action-btn--pass:active {
    background: var(--alt-charcoal);
    color: var(--surface-bg);
  }

  .swipe-action-btn--queue:active {
    background: var(--interactive-text-hover, #223b3b);
  }

	/* ── Loading ── */
	.initial-loading {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
		margin: auto 0;
	}

	.loading-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-slate);
		margin: 0;
	}

	/* ── Empty / Error ── */
	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 1.5rem;
		text-align: center;
		gap: 0.5rem;
		margin: auto 0;
	}

	.empty-text {
		font-family: var(--font-body);
		font-size: 0.9rem;
		color: var(--alt-slate);
		margin: 0 0 0.75rem;
	}

	.error-title {
		font-family: var(--font-body);
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--alt-terracotta);
		margin: 0;
	}

	.error-message {
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-slate);
		margin: 0 0 0.75rem;
	}

	.action-btn {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		padding: 0.5rem 1.5rem;
		min-height: 44px;
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.action-btn:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-dot {
			animation: none;
			opacity: 0.6;
		}
	}
</style>
