<script lang="ts">
import { BookOpen, SquareArrowOutUpRight } from "@lucide/svelte";
import { Spring } from "svelte/motion";
import { type SwipeDirection, swipe } from "#lib/actions/swipe.js";
import type { RenderFeed } from "#lib/schema/feed.js";
import { ogImageResolver } from "#lib/utils/ogImageResolver.js";
import { createProxyImage } from "#lib/utils/proxyImage.svelte.js";

interface Props {
	feed: RenderFeed;
	statusMessage?: string | null;
	onDismiss: (direction: number) => Promise<void> | void;
	onOpenArticle?: () => void;
	thumbnailProxyUrl: string | null;
	isBusy?: boolean;
	isLcp?: boolean;
}

const {
	feed,
	onDismiss,
	onOpenArticle,
	thumbnailProxyUrl,
	isBusy = false,
	isLcp = false,
}: Props = $props();

let thumbnailArea = $state<HTMLElement | null>(null);

const image = createProxyImage({
	url: () => thumbnailProxyUrl,
	container: () => thumbnailArea,
	resolve: () => ogImageResolver().resolve(feed.feedId ?? ""),
});

const SWIPE_THRESHOLD = 60;
const HORIZONTAL_DRAG_GUARD_PX = 10;
let x = new Spring(0, { stiffness: 0.18, damping: 0.85 });
let hasSwiped = $state(false);
let movedHorizontally = false;
let swipeElement: HTMLDivElement | null = $state(null);

const cardStyle = $derived.by(() => {
	const translate = x.current;
	const opacity = Math.max(0.4, 1 - Math.abs(translate) / 500);

	return [
		`transform: translate3d(${translate}px, 0, 0)`,
		`opacity: ${opacity}`,
	].join("; ");
});

const hasDescription = $derived(Boolean(feed.description));
const publishedLabel = $derived.by(() => {
	if (feed.created_at) {
		try {
			return new Date(feed.created_at).toLocaleString();
		} catch {
			// Fallback
		}
	}
	if (!feed.published) return null;
	try {
		return new Date(feed.published).toLocaleString();
	} catch {
		return feed.published;
	}
});

$effect(() => {
	if (!swipeElement) return;

	const swipeHandler = (event: Event) => {
		if (hasSwiped) return;
		handleSwipe(event as CustomEvent<{ direction: SwipeDirection }>);
	};

	const swipeMoveHandler = (event: Event) => {
		const moveEvent = event as CustomEvent<{
			deltaX: number;
			deltaY: number;
		}>;
		const { deltaX, deltaY } = moveEvent.detail;

		if (Math.abs(deltaX) > Math.abs(deltaY)) {
			if (Math.abs(deltaX) > HORIZONTAL_DRAG_GUARD_PX) {
				movedHorizontally = true;
			}
			x.set(deltaX, { instant: true });
		}
	};

	const swipeEndHandler = (_event: Event) => {
		x.target = 0;
		setTimeout(() => {
			movedHorizontally = false;
		}, 0);
	};

	swipeElement.addEventListener("swipe", swipeHandler);
	swipeElement.addEventListener("swipe:move", swipeMoveHandler);
	swipeElement.addEventListener("swipe:end", swipeEndHandler);

	return () => {
		swipeElement?.removeEventListener("swipe", swipeHandler);
		swipeElement?.removeEventListener("swipe:move", swipeMoveHandler);
		swipeElement?.removeEventListener("swipe:end", swipeEndHandler);
	};
});

async function handleSwipe(event: CustomEvent<{ direction: SwipeDirection }>) {
	const dir = event.detail.direction;
	if (dir !== "left" && dir !== "right") return;

	hasSwiped = true;

	const width = swipeElement?.clientWidth ?? window.innerWidth;
	const target = dir === "left" ? -width : width;

	await x.set(target, { preserveMomentum: 120 });
	await onDismiss(dir === "left" ? -1 : 1);

	hasSwiped = false;
	await x.set(0, { instant: true });
}

function handleCardClick(event: MouseEvent) {
	if (hasSwiped || movedHorizontally) return;
	const target = event.target as HTMLElement | null;
	if (target?.closest("a, button")) return;
	onOpenArticle?.();
}
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
  bind:this={swipeElement}
  class="swipe-card"
  use:swipe={{ threshold: SWIPE_THRESHOLD, restraint: 120, allowedTime: 500 }}
  aria-busy={isBusy}
  data-testid="visual-preview-card"
  onclick={handleCardClick}
  style="{cardStyle}; touch-action: none;"
>
  <div class="card-inner">
    <div class="thumbnail-area" bind:this={thumbnailArea}>
      {#if image.state === "absent"}
        <div class="thumbnail-fallback" data-testid="thumbnail-fallback">
          <span class="thumbnail-fallback-text">No preview</span>
        </div>
      {:else if image.state === "loaded" && image.src}
        <img
          src={image.src}
          alt=""
          loading={isLcp ? "eager" : "lazy"}
          fetchpriority={isLcp ? "high" : undefined}
          decoding="async"
          data-testid="thumbnail-image"
          class="thumbnail-img"
        />
      {:else}
        <div class="thumbnail-shimmer" data-testid="thumbnail-shimmer"></div>
      {/if}
      <div class="thumbnail-overlay"></div>
    </div>

    <div class="content-area">
      <div class="card-header-compact">
        <p class="card-label">Swipe to mark as read</p>
        <div class="flex items-center gap-2">
          <a
            href={feed.link}
            target="_blank"
            rel="noopener noreferrer"
            aria-label="Open article"
            class="card-title-link"
          >
            <div class="flex-shrink-0">
              <SquareArrowOutUpRight
                class="title-icon"
                size={16}
              />
            </div>
            <h2 class="card-title">
              {feed.title}
            </h2>
          </a>
        </div>
        {#if publishedLabel}
          <p class="card-dateline">{publishedLabel}</p>
        {/if}
      </div>

      <div
        style="touch-action: pan-y; overflow-x: hidden;"
        class="scroll-area"
        data-testid="unified-scroll-area"
      >
        {#if hasDescription}
          <div class="content-block">
            <div class="summary-prose summary-prose--clamp">
              {feed.description}
            </div>
          </div>
        {/if}
      </div>
    </div>

    <footer class="card-footer" data-testid="action-footer">
      <div class="flex gap-3 w-full">
        <button
          type="button"
          onclick={() => onOpenArticle?.()}
          class="action-btn"
        >
          <BookOpen size={14} />
          Article
        </button>
      </div>
    </footer>
  </div>
</div>

<style>
  .swipe-card {
    position: absolute;
    width: 100%;
    height: 100%;
    max-width: calc(100% - 1rem);
    background: var(--surface-bg);
    border: 1px solid var(--surface-border);
    user-select: none;
    overflow: hidden;
    cursor: pointer;
  }

  .card-inner {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .thumbnail-area {
    position: relative;
    width: 100%;
    height: 35%;
    min-height: 150px;
    overflow: hidden;
  }

  .thumbnail-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    animation: thumbnail-reveal 0.3s ease-out;
  }

  .thumbnail-shimmer {
    position: absolute;
    inset: 0;
    background: var(--surface-2);
    animation: shimmer-pulse 1.5s ease-in-out 3;
  }

  @keyframes thumbnail-reveal {
    from {
      opacity: 0;
    }
  }

  .thumbnail-fallback {
    position: absolute;
    inset: 0;
    background: var(--surface-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .thumbnail-fallback-text {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    color: var(--alt-ash);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .thumbnail-overlay {
    position: absolute;
    inset: 0;
    pointer-events: none;
    background: linear-gradient(to top, var(--surface-bg) 0%, transparent 50%);
  }

  .content-area {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    padding: 0 0.75rem;
    margin-top: -1rem;
    position: relative;
    z-index: 1;
  }

  .card-header-compact {
    margin-bottom: 0.5rem;
  }

  .card-label {
    font-family: var(--font-body);
    font-size: 0.65rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--alt-ash);
    margin: 0 0 0.35rem;
  }

  .card-title-link {
    display: flex;
    align-items: flex-start;
    gap: 0.4rem;
    text-decoration: none;
    min-width: 0;
  }

  .card-title-link :global(.title-icon) {
    color: var(--alt-primary);
    flex-shrink: 0;
    margin-top: 0.15rem;
  }

  .card-title {
    font-family: var(--font-display);
    font-size: 1rem;
    font-weight: 600;
    color: var(--alt-primary);
    line-height: 1.3;
    margin: 0;
    word-break: break-word;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .card-title-link:hover .card-title {
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .card-dateline {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    color: var(--alt-ash);
    margin: 0.25rem 0 0;
  }

  .scroll-area {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    background: transparent;
    scroll-behavior: smooth;
    overscroll-behavior: contain;
    user-select: none;
  }

  .scroll-area::-webkit-scrollbar { width: 3px; }
  .scroll-area::-webkit-scrollbar-track { background: transparent; }
  .scroll-area::-webkit-scrollbar-thumb { background: var(--surface-border); }

  .scroll-area,
  .scroll-area :global(*) {
    -webkit-user-select: none;
    -moz-user-select: none;
    -ms-user-select: none;
    user-select: none;
  }

  .content-block {
    margin-bottom: 0.75rem;
  }

  .summary-prose {
    font-family: var(--font-body);
    font-size: 0.85rem;
    line-height: 1.6;
    color: var(--alt-charcoal);
    word-break: break-word;
    overflow-wrap: anywhere;
  }

  .summary-prose--clamp {
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .card-footer {
    position: relative;
    z-index: 2;
    border-top: 1px solid var(--surface-border);
    padding: 0.75rem;
    padding-bottom: calc(0.75rem + env(safe-area-inset-bottom, 0px));
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
    padding: 0.5rem 0.75rem;
    min-height: 48px;
    flex: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }

  .action-btn:active:not(:disabled) {
    background: var(--alt-charcoal);
    color: var(--surface-bg);
  }

  @keyframes shimmer-pulse {
    0%, 100% { opacity: 0.5; }
    50% { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .thumbnail-shimmer {
      animation: none;
      opacity: 0.7;
    }
    .thumbnail-img {
      animation: none;
    }
  }
</style>
