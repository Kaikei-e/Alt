import type { RenderFeed } from "$lib/schema/feed";

/**
 * Select the article IDs that still need an OG-image proxy URL fetched.
 *
 * Content-based (NOT count-based): returns every visible feed that has an
 * articleId, lacks an ogImageProxyUrl, and has not already been requested.
 * Idempotent — feeds whose articleId is in `requested` are skipped, so
 * concurrent grid mutations (mark-as-read remove + replacement append that
 * leaves the visible count unchanged) never drop a backfill.
 */
export function selectOgImagePrefetchIds(
	visibleFeeds: readonly RenderFeed[],
	requested: ReadonlySet<string>,
): string[] {
	const ids: string[] = [];
	const seen = new Set<string>();
	for (const feed of visibleFeeds) {
		const id = feed.articleId;
		if (!id || feed.ogImageProxyUrl || requested.has(id) || seen.has(id)) {
			continue;
		}
		seen.add(id);
		ids.push(id);
	}
	return ids;
}
