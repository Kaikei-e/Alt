import type { RenderFeed } from "$lib/schema/feed";

/**
 * Select the article IDs that still need an OG-image proxy URL fetched.
 *
 * Content-based (NOT count-based): returns every visible feed that has an
 * articleId, lacks an ogImageProxyUrl, and has not already been requested.
 * Idempotent — feeds whose articleId is in `requested` are skipped, so
 * concurrent grid mutations (mark-as-read remove + replacement append that
 * leaves the visible count unchanged) never drop a backfill.
 *
 * `backfilled` is the second place an answer can already live. A URL that
 * arrived after its card was rendered is held in the overlay keyed by article,
 * not written back onto the feed, so a feed object can be permanently missing
 * `ogImageProxyUrl` for an article we already have the picture for. Without
 * this the page would re-request every one of them each time it remounts.
 */
export function selectOgImagePrefetchIds(
	visibleFeeds: readonly RenderFeed[],
	requested: ReadonlySet<string>,
	backfilled: (articleId: string) => boolean = () => false,
): string[] {
	const ids: string[] = [];
	const seen = new Set<string>();
	for (const feed of visibleFeeds) {
		const id = feed.articleId;
		if (
			!id ||
			feed.ogImageProxyUrl ||
			requested.has(id) ||
			seen.has(id) ||
			backfilled(id)
		) {
			continue;
		}
		seen.add(id);
		ids.push(id);
	}
	return ids;
}
