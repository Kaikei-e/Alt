<script lang="ts">
/**
 * A single tile in the mobile visual-preview gallery.
 *
 * Layout follows the mobile-list research rather than the desktop card: at half
 * a phone's width, a list-style thumbnail is too small to be recognisable, so
 * the image runs the full tile width at 16:9 — the ratio OG images are actually
 * authored at, which keeps the crop close to nothing — and the title is the only
 * prose under it. No excerpt, no author line, no tag row: each of those costs
 * image height, and the image is what the reader is scanning here.
 */
import type { RenderFeed } from "$lib/schema/feed";
import { ogImageOverlay } from "$lib/stores/ogImageOverlay.svelte";
import { formatCompactDate } from "$lib/utils/feed";
import { ogImageResolver } from "$lib/utils/ogImageResolver";
import { createProxyImage } from "$lib/utils/proxyImage.svelte";

interface Props {
	feed: RenderFeed;
	onSelect: (feed: RenderFeed) => void;
	isRead?: boolean;
}

const { feed, onSelect, isRead = false }: Props = $props();

let imageContainer = $state<HTMLElement | null>(null);

// See VisualFeedCard: a URL the grid backfilled after this tile rendered
// arrives through the overlay, keyed by article, not by mutating `feed`.
const overlayCell = $derived(ogImageOverlay.cell(feed.articleId));

const image = createProxyImage({
	url: () => feed.ogImageProxyUrl || overlayCell?.url,
	container: () => imageContainer,
	// Feeds whose RSS carried no image resolve when the reader reaches them,
	// keyed on the feed: most of these have no article row to key on.
	//
	// `feedId`, never `id`. `id` is articles.id or a per-response UUID, and
	// ResolveOgImages matches feeds.id — sending `id` matched nothing and read
	// back as "no feed has an image". Absent on surfaces built without a
	// feeds.id (search results come from Meilisearch hits), and the resolver
	// settles an empty key on `absent` without a request.
	resolve: () => ogImageResolver().resolve(feed.feedId ?? ""),
});

// A tile with no URL yet is resolving, not empty — it keeps the shimmer until
// resolution settles, and only a settled "absent" reaches the fallback.
const showFallback = $derived(image.state === "absent");

// The shared `publishedAtFormatted` carries a time of day, which wraps to a
// second line at half a phone's width and pulls weight away from the image.
const dateline = $derived(
	formatCompactDate(feed.created_at || feed.published) ||
		feed.publishedAtFormatted,
);
</script>

<button
  type="button"
  class="gallery-tile"
  data-testid="gallery-tile"
  data-read={isRead}
  aria-label="Open {feed.title}"
  onclick={() => onSelect(feed)}
>
  <div
    class="tile-image-area"
    bind:this={imageContainer}
    aria-busy={image.state === "idle" || image.state === "loading"}
  >
    {#if showFallback}
      <div class="tile-fallback" data-testid="tile-image-fallback">
        <span class="tile-fallback-text">No preview</span>
      </div>
    {:else if image.state === "loaded" && image.src}
      <img
        data-testid="tile-image"
        src={image.src}
        alt=""
        decoding="async"
        class="tile-image"
      />
    {:else}
      <div class="tile-shimmer" data-testid="tile-image-loading"></div>
    {/if}

    {#if !isRead}
      <span class="unread-dot" data-testid="tile-unread-dot"></span>
    {/if}
  </div>

  <div class="tile-text">
    <h3 class="tile-title" data-testid="tile-title">{feed.title}</h3>
    {#if dateline}
      <p class="tile-meta">{dateline}</p>
    {/if}
  </div>
</button>

<style>
  /* height:100% + a two-line title reservation keeps both tiles in a row the
     same height. Without it a one-word headline next to a wrapping one leaves a
     ragged edge down the middle of the gallery, and the ragged edge is what the
     eye tracks instead of the images. */
  .gallery-tile {
    display: flex;
    flex-direction: column;
    height: 100%;
    width: 100%;
    text-align: left;
    padding: 0;
    background: var(--surface-bg);
    border: 1px solid var(--surface-border);
    cursor: pointer;
    touch-action: manipulation;
    transition: opacity 0.15s, border-color 0.15s;
  }

  /* Read tiles recede rather than disappear: the gallery is also how a reader
     confirms they have already been somewhere. */
  .gallery-tile[data-read="true"] {
    opacity: 0.55;
  }

  .gallery-tile:active {
    border-color: var(--alt-primary);
  }

  /* ── Image ──
     16:9 matches the ratio OG images are authored at (1.91:1 is close enough
     that `cover` crops only a sliver), so the thumbnail shows what the
     publisher framed rather than a centre crop of it. */
  .tile-image-area {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    overflow: hidden;
    background: var(--surface-hover);
  }

  .tile-image {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    animation: reveal 200ms ease-out;
  }

  @keyframes reveal {
    from {
      opacity: 0;
    }
  }

  .tile-fallback {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(
      135deg,
      var(--surface-hover) 0%,
      var(--surface-bg) 100%
    );
  }

  .tile-fallback-text {
    font-family: var(--font-mono);
    font-size: 0.55rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--alt-ash);
  }

  .tile-shimmer {
    position: absolute;
    inset: 0;
    background: linear-gradient(
      90deg,
      rgba(0, 0, 0, 0.03) 25%,
      rgba(0, 0, 0, 0.08) 50%,
      rgba(0, 0, 0, 0.03) 75%
    );
    background-size: 200% 100%;
    /* Bounded rather than `infinite`: a resolution can outlive several asks,
       and an animation running past five seconds with no pause control fails
       WCAG 2.2.2. The flat tint that remains still reads as "not here yet". */
    animation: shimmer 1.5s linear 3 forwards;
  }

  /* Unread marker sits on the image, not in the text block, so the read/unread
     split is legible while scanning images at speed. */
  .unread-dot {
    position: absolute;
    top: 0.4rem;
    left: 0.4rem;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--alt-primary);
    box-shadow: 0 0 0 2px var(--surface-bg);
  }

  /* ── Text ── */
  .tile-text {
    flex: 1;
    padding: 0.5rem 0.5rem 0.6rem;
  }

  .tile-title {
    font-family: var(--font-body);
    font-size: 0.8rem;
    font-weight: 600;
    line-height: 1.35;
    /* Two lines are always reserved, so a short headline does not pull the
       dateline up and misalign it against its neighbour. */
    min-height: calc(2 * 1.35 * 0.8rem);
    color: var(--text-primary);
    margin: 0;
    word-break: break-word;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .gallery-tile[data-read="true"] .tile-title {
    font-weight: 400;
    color: var(--text-muted);
  }

  .tile-meta {
    font-family: var(--font-mono);
    font-size: 0.6rem;
    color: var(--text-muted);
    margin: 0.3rem 0 0;
  }

  @keyframes shimmer {
    0% {
      background-position: 200% 0;
    }
    100% {
      background-position: -200% 0;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .tile-shimmer {
      animation: none;
      opacity: 0.7;
    }
    .tile-image {
      animation: none;
    }
  }
</style>
