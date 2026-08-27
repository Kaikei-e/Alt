/**
 * Proxy URLs that arrived after their cards were already on screen.
 *
 * The feed grid backfills og:image URLs for the articles it holds, and those
 * answers arrive one network round trip after the cards were rendered. Writing
 * them onto the feed objects themselves does not survive the trip:
 *
 *  - a feed that came from `+page.server.ts` is a plain SSR object, never a
 *    `$state` proxy, so assigning a field on it updates nothing;
 *  - a feed reached through a `$derived` that rebuilds its items (`.map(f =>
 *    ({...f}))`, `toRenderFeed(...)`) is a fresh object whose mutation is both
 *    invisible and discarded on the next recompute;
 *  - and the array a caller holds across an `await` is a snapshot — the item it
 *    finds afterwards may be one the grid has since replaced.
 *
 * Keying the answer by article sidesteps all three: a card looks its own answer
 * up while it renders, so the URL lands whichever object the card ended up
 * rendering.
 *
 * ## Why a plain `Map` of one-field cells rather than a `SvelteMap`
 *
 * `SvelteMap.get(k)` subscribes per key only when the key is already present.
 * For an absent key it subscribes the reader to the map's single version
 * signal, and the first `set()` for any key bumps that same signal — so a grid
 * of N cards that all asked about keys nobody has answered yet is re-rendered
 * once per arrival, N times over. On an infinite-scroll grid of a few hundred
 * cards that is quadratic, and it is quadratic in exactly the situation the
 * overlay exists for: a cold grid filling in.
 *
 * A cell per article inverts it. The registry itself never needs to be
 * reactive — its key set comes from the feed list, not from us — and each card
 * reads one `$state` field, so an arrival re-renders one card.
 *
 * Client-only by construction: written from `$effect`, which never runs on the
 * server, so this module-level state is never shared between requests.
 */

/** One article's backfilled URL. One `$state` field, read by one card. */
class OgImageCell {
	url = $state<string | null>(null);
}

export interface OgImageOverlay {
	/**
	 * The cell for this article, created on first ask.
	 *
	 * Call once during component initialisation and hold the reference: the
	 * cell outlives the card, so a tile that scrolls away and back gets the
	 * answer that arrived while it was gone.
	 */
	cell: (articleId: string | null | undefined) => OgImageCell | null;
	/**
	 * Whether a URL for this article has already arrived.
	 *
	 * Deliberately NOT reactive. Its caller is the same `$effect` that requests
	 * the backfill and then writes the results, and a reactive read here would
	 * make that effect re-run on its own writes.
	 */
	has: (articleId: string) => boolean;
	/** Records the URL a backfill produced. */
	resolve: (articleId: string, url: string) => void;
}

export function createOgImageOverlay(): OgImageOverlay {
	const cells = new Map<string, OgImageCell>();
	const resolved = new Set<string>();

	function cellFor(articleId: string): OgImageCell {
		let cell = cells.get(articleId);
		if (!cell) {
			cell = new OgImageCell();
			cells.set(articleId, cell);
		}
		return cell;
	}

	return {
		cell: (articleId) => (articleId ? cellFor(articleId) : null),
		has: (articleId) => Boolean(articleId) && resolved.has(articleId),
		resolve: (articleId, url) => {
			// An empty URL is not an image. Storing it would make the cell
			// truthy and start a load that can only fail.
			if (!articleId || !url) return;
			resolved.add(articleId);
			cellFor(articleId).url = url;
		},
	};
}

/**
 * The overlay the feed surfaces share.
 *
 * One per session on purpose: the grid's backfill effect and the cards that
 * render its results are different components, and a per-component overlay
 * would mean the answer never reaches the card that needed it.
 */
export const ogImageOverlay = createOgImageOverlay();
