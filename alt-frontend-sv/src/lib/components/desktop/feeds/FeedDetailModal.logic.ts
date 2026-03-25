import type { FeedContentOnTheFlyResponse } from "$lib/api/client/articles";

/**
 * Process article fetch response, treating empty content as an error.
 *
 * Without this check, empty content causes the auto-fetch $effect to loop
 * infinitely: articleContent stays null, contentError stays null,
 * isFetchingContent resets → all conditions for re-fetch remain true.
 */
export function processArticleFetchResponse(response: FeedContentOnTheFlyResponse): {
	articleContent: string | null;
	articleID: string | null;
	contentError: string | null;
} {
	if (response.content?.trim()) {
		return {
			articleContent: response.content,
			articleID: response.article_id || null,
			contentError: null,
		};
	}
	return {
		articleContent: null,
		articleID: null,
		contentError: "Article content could not be retrieved",
	};
}
