import { stripHtmlToText } from "$lib/domain/feed/sanitize";
import type { FeedSearchResult, SearchFeedItem } from "$lib/schema/search";

/**
 * Meilisearch stores whatever the feed published, which for a great many
 * publishers is a raw `content:encoded` blob. The desktop search path already
 * runs its hits through `sanitizeFeed`; this is the same step for the mobile
 * path, which used to hand the blob to the card verbatim and render it as
 * literal markup.
 *
 * `description` is deliberately NOT truncated here: `SearchFeedItem` contracts
 * it as the full text so `SearchResultItem` can do its own 200-character cut
 * behind READ MORE.
 */
const toReadableText = (item: SearchFeedItem): SearchFeedItem => ({
	...item,
	title: stripHtmlToText(item.title),
	description: stripHtmlToText(item.description),
});

export const transformFeedSearchResult = (
	feedSearchResult: FeedSearchResult | SearchFeedItem[],
): SearchFeedItem[] => {
	// If the response is directly an array (backend returns array directly)
	if (Array.isArray(feedSearchResult)) {
		return feedSearchResult.map(toReadableText);
	}

	// If it's wrapped in FeedSearchResult structure
	if (
		feedSearchResult &&
		typeof feedSearchResult === "object" &&
		"results" in feedSearchResult
	) {
		if (Array.isArray(feedSearchResult.results)) {
			return feedSearchResult.results.map(toReadableText);
		}
	}

	// Fallback to empty array if structure is unexpected
	return [];
};
