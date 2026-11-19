<script lang="ts">
import { ArrowLeft, Check, ChevronLeft, ChevronRight } from "@lucide/svelte";
import { onDestroy } from "svelte";
import { MediaQuery } from "svelte/reactivity";
import { Button } from "#lib/components/ui/button/index.js";
import { ReadingSurface } from "#lib/reading/reading-events.js";
import { articlePrefetcher } from "#lib/utils/articlePrefetcher.js";
import ArticleEditorialRail from "./ArticleEditorialRail.svelte";
import ArticleReaderActions from "./ArticleReaderActions.svelte";
import ArticleReaderBody from "./ArticleReaderBody.svelte";
import { createArticleReader } from "./createArticleReader.svelte.js";
import {
	closeReaderOverlay,
	getReaderOverlaySession,
	navigateReaderOverlay,
	updateReaderOverlayArticleId,
} from "./reader-overlay.svelte.js";
import type { ReaderOverlayRouteState } from "./types.js";

interface Props {
	overlayState: ReaderOverlayRouteState;
}

const { overlayState }: Props = $props();

const session = $derived(getReaderOverlaySession(overlayState.sessionId));
const items = $derived(session?.items ?? [overlayState.article]);
const hasPrevious = $derived(overlayState.currentIndex > 0);
const hasNext = $derived(
	overlayState.currentIndex >= 0 &&
		overlayState.currentIndex < items.length - 1,
);

let scrollEl = $state<HTMLDivElement | null>(null);
let lastScrollUrl: string | null = null;
const railQuery = new MediaQuery("(min-width: 1024px)", false);
const hasRail = $derived(railQuery.current);

const reader = createArticleReader({
	getArticleId: () => overlayState.article.articleId,
	getUrl: () => overlayState.article.url,
	getTitle: () => overlayState.article.title,
	getSurface: () => overlayState.surface,
	getScrollContainer: () => scrollEl,
	onArticleIdResolved: (articleId, url) => {
		updateReaderOverlayArticleId(articleId, url);
	},
});

// Reset overlay scroll position when navigating to another article.
$effect(() => {
	const currentUrl = overlayState.article.url;
	if (currentUrl !== lastScrollUrl) {
		lastScrollUrl = currentUrl;
		if (scrollEl) {
			scrollEl.scrollTop = 0;
		}
	}
});

// Lock background page scroll while overlay is open.
$effect(() => {
	const originalOverflow = document.body.style.overflow;
	document.body.style.overflow = "hidden";
	return () => {
		document.body.style.overflow = originalOverflow;
	};
});

// Prefetch the next 2 articles from the surrounding list context.
$effect(() => {
	const currentIndex = overlayState.currentIndex;
	if (currentIndex < 0 || items.length <= 1) return;
	articlePrefetcher.triggerPrefetch(
		items.map((item) => ({ normalizedUrl: item.url })),
		currentIndex,
		2,
	);
});

onDestroy(() => {
	if (overlayState.surface !== ReadingSurface.SWIPE) {
		articlePrefetcher.triggerPrefetch([], -1, 0);
	}
});

function handlePrevious() {
	if (hasPrevious) {
		navigateReaderOverlay(overlayState.currentIndex - 1);
	}
}

function handleNext() {
	if (hasNext) {
		navigateReaderOverlay(overlayState.currentIndex + 1);
	}
}

function handleKeydown(event: KeyboardEvent) {
	const target = event.target as HTMLElement | null;
	if (
		target &&
		(target.tagName === "INPUT" ||
			target.tagName === "TEXTAREA" ||
			target.isContentEditable)
	) {
		return;
	}
	if (event.key === "Escape") {
		event.preventDefault();
		closeReaderOverlay();
	} else if (event.key === "ArrowLeft" && hasPrevious) {
		event.preventDefault();
		handlePrevious();
	} else if (event.key === "ArrowRight" && hasNext) {
		event.preventDefault();
		handleNext();
	}
}

async function handleMarkAsRead() {
	if (!session?.onMarkAsRead) return;
	await session.onMarkAsRead(overlayState.article);
}
</script>

<svelte:window onkeydown={handleKeydown} />

<div
	class="reader-overlay"
	role="dialog"
	aria-modal="true"
	aria-label={reader.mastheadTitle}
	data-testid="reader-overlay"
>
	<header class="overlay-bar" aria-label="Article actions">
		<Button
			variant="ghost"
			size="icon"
			aria-label="Close reader"
			data-testid="reader-overlay-close"
			onclick={closeReaderOverlay}
			class="action-btn action-btn--back"
		>
			<ArrowLeft class="h-4 w-4" />
			<span class="action-label">Back</span>
		</Button>

		{#if items.length > 1}
			<div class="nav-cluster">
				<Button
					variant="outline"
					size="icon"
					aria-label="Previous article"
					data-testid="reader-prev-button"
					onclick={handlePrevious}
					disabled={!hasPrevious}
					class="action-btn"
				>
					<ChevronLeft class="h-4 w-4" />
					<span class="action-label">Prev</span>
				</Button>
				<Button
					variant="outline"
					size="icon"
					aria-label="Next article"
					data-testid="reader-next-button"
					onclick={handleNext}
					disabled={!hasNext}
					class="action-btn"
				>
					<ChevronRight class="h-4 w-4" />
					<span class="action-label">Next</span>
				</Button>
			</div>
		{/if}

		{#if session?.onMarkAsRead}
			<Button
				variant="outline"
				size="icon"
				aria-label="Mark as Read"
				data-testid="reader-mark-read-button"
				onclick={() => void handleMarkAsRead()}
				disabled={session.isMarkingAsRead?.() ?? false}
				class="action-btn"
			>
				<Check class="h-4 w-4" />
				<span class="action-label">Mark as Read</span>
			</Button>
		{/if}

		{#if !hasRail}
			<ArticleReaderActions {reader} />
		{/if}
	</header>

	<div
		class="overlay-body"
		bind:this={scrollEl}
		data-testid="reader-overlay-body"
	>
		<div class="overlay-layout">
			<article class="overlay-article">
				<ArticleReaderBody
					{reader}
					showInlineSummary={!hasRail}
					fallbackDescription={overlayState.article.fallbackDescription}
				/>
			</article>

			{#if hasRail}
				<ArticleEditorialRail
					{reader}
					excerpt={overlayState.article.excerpt ??
						overlayState.article.fallbackDescription}
				/>
			{/if}
		</div>
	</div>
</div>

<style>
	.reader-overlay {
		position: fixed;
		inset: 0;
		z-index: 50;
		display: flex;
		flex-direction: column;
		background: var(--surface-bg, #faf9f7);
		color: var(--alt-charcoal, #1a1a1a);
		font-family: var(--font-body);
	}

	.overlay-bar {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-height: 56px;
		padding: calc(env(safe-area-inset-top, 0px) + 0.5rem) 1rem 0.5rem;
		background: var(--surface-bg, #faf9f7);
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
		flex-shrink: 0;
	}

	.nav-cluster {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
	}

	.overlay-body {
		--rail-sticky-top: 0px;
		--rail-max-height: calc(100vh - 5.5rem);
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 1.25rem 1rem calc(2rem + env(safe-area-inset-bottom, 0px));
	}

	.overlay-layout {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.overlay-article {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
		width: 100%;
		max-width: 72ch;
		margin: 0 auto;
		min-width: 0;
	}

	@media (min-width: 640px) {
		.overlay-bar {
			padding-inline: 1.5rem;
		}

		.overlay-body {
			padding: 1.5rem 2rem 3rem;
		}
	}

	@media (min-width: 1024px) {
		.overlay-layout {
			max-width: min(100%, 96rem);
			margin: 0 auto;
			display: grid;
			grid-template-columns: minmax(0, 1fr) 22rem;
			column-gap: 2rem;
			align-items: start;
		}

		.overlay-article {
			margin-inline: auto;
		}
	}
</style>
